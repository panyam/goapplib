package github

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
)

// WithLazySnapshots makes Prepare pin a snapshot's commit and list its tree without downloading
// any file (agni issue 1877). The mount gets no Manifest. Open and HostPath then fetch a file the
// first time something reads it, through FetchAt, so a server keeps only the files it serves. The
// Limits apply to each read rather than to the whole commit: one file for Open, and the files
// under a folder for HostPath. A repo whose boards together pass the cap can then be mounted, as
// long as no one folder that is read passes it.
//
// A snapshot an eager driver made keeps its Manifest and is served from it as before, so a server
// can turn this on over mounts it already has.
func WithLazySnapshots() Option { return func(d *Driver) { d.lazy = true } }

// lazy reports whether m is a snapshot whose files are fetched on first use.
func lazy(m mounts.Mount) bool { return m.Mode == mounts.ModeSnapshot && m.Manifest == "" }

// prepareLazy checks that the tree at m.Version can be listed whole, which also fills ListAt's
// cache for the first reads.
func (d *Driver) prepareLazy(ctx context.Context, m mounts.Mount, creds mounts.Credentials) error {
	if !commitSHA.MatchString(m.Version) {
		return fmt.Errorf("%w: snapshot version %q isn't a commit sha", mounts.ErrInvalidArgument, m.Version)
	}
	_, err := d.treeAt(ctx, m, creds, m.Version)
	return err
}

// lazyTree is a lazy snapshot's files and folders, from the tree listing alone.
type lazyTree struct {
	files map[string]mounts.File
	dirs  map[string][]fs.DirEntry
}

// lazyTree builds the tree once per listing and keeps it beside treeAt's, since the registry's FS
// opens the mount again for every file it reads.
func (d *Driver) lazyTree(ctx context.Context, m mounts.Mount, creds mounts.Credentials) (*lazyTree, error) {
	rp, err := repo(m.Source)
	if err != nil {
		return nil, err
	}
	all, err := d.treeAt(ctx, m, creds, m.Version)
	if err != nil {
		return nil, err
	}
	key, err := treeKey(rp, m.Version, creds)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	t := d.lazyTrees[key]
	d.mu.Unlock()
	if t != nil {
		return t, nil
	}
	t = buildLazyTree(all)
	d.mu.Lock()
	if d.lazyTrees == nil || len(d.lazyTrees) >= treeCacheSize {
		d.lazyTrees = map[string]*lazyTree{}
	}
	d.lazyTrees[key] = t
	d.mu.Unlock()
	return t, nil
}

func buildLazyTree(all []mounts.File) *lazyTree {
	t := &lazyTree{files: make(map[string]mounts.File, len(all)), dirs: map[string][]fs.DirEntry{".": nil}}
	for _, f := range all {
		t.files[f.Path] = f
		child := f.Path
		ent := fs.DirEntry(info{name: path.Base(f.Path), size: f.Size})
		for {
			parent := path.Dir(child)
			_, seen := t.dirs[parent]
			t.dirs[parent] = append(t.dirs[parent], ent)
			if seen || parent == "." {
				break
			}
			child, ent = parent, info{name: path.Base(parent), dir: true}
		}
	}
	return t
}

// lazyFS serves a lazy snapshot. Listing a folder reads only the tree. Opening a file fetches it
// into the blob store first, unless the store already holds it.
type lazyFS struct {
	ctx   context.Context
	d     *Driver
	m     mounts.Mount
	creds mounts.Credentials
	t     *lazyTree
}

func (l lazyFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	if f, ok := l.t.files[name]; ok {
		h, _, err := l.d.FetchAt(l.ctx, l.m, l.creds, l.m.Version, f)
		if err != nil {
			return nil, pathErr("open", name, err)
		}
		rc, err := l.d.blobs.Open(l.ctx, h)
		if err != nil {
			return nil, pathErr("open", name, err)
		}
		return &file{info: info{name: path.Base(name), size: f.Size}, rc: rc}, nil
	}
	if ents, ok := l.t.dirs[name]; ok {
		return newDir(name, append([]fs.DirEntry(nil), ents...)), nil
	}
	return nil, pathErr("open", name, fs.ErrNotExist)
}

// HostPathWith returns where p of m is on disk, as HostPath does. For a lazy snapshot it first
// fetches the file p names, or every file under the folder p names, into the view and the blob
// store. The files under p must fit the driver's Limits, or it returns ErrTooLarge and fetches
// nothing, so a call for the root of a big repo fails while a call for one board in it works. A
// file already in the view is not fetched again. A snapshot with a Manifest, and a live mount, are
// handled as HostPath handles them.
func (d *Driver) HostPathWith(ctx context.Context, m mounts.Mount, creds mounts.Credentials, p string) (string, error) {
	if !lazy(m) {
		return d.HostPath(m, p)
	}
	clean, err := mounts.CleanPath(p)
	if err != nil {
		return "", err
	}
	linker, ok := d.blobs.(blobs.Linker)
	if d.views == "" || !ok {
		return "", fmt.Errorf("%w: GitHub mount %q", mounts.ErrNotLocal, m.Name)
	}
	if err := mounts.ValidName(m.Name); err != nil {
		return "", err
	}
	if !commitSHA.MatchString(m.Version) {
		return "", fmt.Errorf("%w: snapshot version %q isn't a commit sha", mounts.ErrInvalidArgument, m.Version)
	}
	files, err := d.ListAt(ctx, m, creds, m.Version, clean)
	if err != nil {
		return "", err
	}
	view := filepath.Join(d.views, m.Name, m.Version)
	unlock := d.lockView(view)
	defer unlock()
	if err := os.MkdirAll(view, 0o750); err != nil {
		return "", err
	}
	for _, f := range files {
		to := filepath.Join(view, filepath.FromSlash(f.Path))
		if _, err := os.Lstat(to); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		h, _, err := d.FetchAt(ctx, m, creds, m.Version, f)
		if err != nil {
			return "", err
		}
		if err := linkInto(linker, h, to); err != nil {
			return "", fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	return filepath.Join(view, filepath.FromSlash(clean)), nil
}

// lockView serializes the fetches into one view, so two reads of one board download it once,
// while reads of other mounts go ahead.
func (d *Driver) lockView(view string) func() {
	d.mu.Lock()
	if d.viewLocks == nil {
		d.viewLocks = map[string]*sync.Mutex{}
	}
	l := d.viewLocks[view]
	if l == nil {
		l = &sync.Mutex{}
		d.viewLocks[view] = l
	}
	d.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// linkInto links h to a temp name beside to and renames it into place, so a reader never sees a
// half-written file, and another process filling the same view leaves one whole file.
func linkInto(l blobs.Linker, h blobs.Hash, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
		return err
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(to), ".fetch-"+hex.EncodeToString(b[:]))
	if err := l.Link(h, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, to); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
