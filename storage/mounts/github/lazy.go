package github

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
)

// WithLazySnapshots makes Prepare pin a snapshot's commit and list its tree without downloading
// any file (agni issue 1877). The mount gets no Manifest. Open and HostPath then fetch a file the
// first time something reads it, so a server keeps only the files it serves. The Limits apply to
// each read rather than to the whole commit: one file for Open, and the files under a folder for
// HostPath. A repo whose boards together pass the cap can then be mounted, as long as no one folder
// that is read passes it.
//
// With WithViews and a linking blob store, the view is the cache. The listing is kept at
// <views>/<mount>/<commit>.tree.json, and each fetched file is linked into the view, so a restart
// lists the mount and serves what it fetched without asking GitHub, and a blob sweep, which names no
// file of a lazy mount, leaves the view's links whole.
//
// A snapshot an eager driver made keeps its Manifest and is served from it as before, so a server
// can turn this on over mounts it already has.
func WithLazySnapshots() Option { return func(d *Driver) { d.lazy = true } }

// WithLazyPastLimits makes Prepare snapshot a commit that fits the Limits whole, as a driver
// without this option does, and make one past them lazy, as WithLazySnapshots does (agni issue
// 1877). It lists the tree first, with each LFS object counted at its own size, so a commit past
// the limits downloads nothing before it is made lazy. A mount under the limits keeps an eager
// snapshot's guarantees: its view is rebuilt from the blobs alone, with no request to GitHub.
func WithLazyPastLimits() Option { return func(d *Driver) { d.lazyPast = true } }

// lazy reports whether m is a snapshot whose files are fetched on first use.
func lazy(m mounts.Mount) bool { return m.Mode == mounts.ModeSnapshot && m.Manifest == "" }

// prepareLazy lists the tree at m.Version from GitHub with the mount's credentials, even when a
// listing is kept on disk, so a new mount is always checked against the repo. With
// WithLazySnapshots it keeps the listing and reports the mount lazy. With WithLazyPastLimits it does
// so only when the commit is past the limits, and otherwise reports it eager, for Prepare to
// snapshot whole.
func (d *Driver) prepareLazy(ctx context.Context, m mounts.Mount, creds mounts.Credentials) (bool, error) {
	if !commitSHA.MatchString(m.Version) {
		return false, fmt.Errorf("%w: snapshot version %q isn't a commit sha", mounts.ErrInvalidArgument, m.Version)
	}
	all, err := d.treeAt(ctx, m, creds, m.Version)
	if err != nil {
		return false, err
	}
	if !d.lazy && d.within(all, "", m.Version) == nil {
		return false, nil
	}
	if file, ok := d.listingFile(m); ok {
		return true, writeListing(file, all)
	}
	return true, nil
}

// lazyTree is a lazy snapshot's files and folders, from the tree listing alone.
type lazyTree struct {
	all   []mounts.File // sorted by path, as treeAt returns them
	files map[string]mounts.File
	dirs  map[string][]fs.DirEntry
}

// lazyTree returns m's tree, built once per mount and commit and kept, since the registry's FS opens
// the mount again for every file it reads. It reads the kept listing before asking GitHub.
func (d *Driver) lazyTree(ctx context.Context, m mounts.Mount, creds mounts.Credentials) (*lazyTree, error) {
	if !commitSHA.MatchString(m.Version) {
		return nil, fmt.Errorf("%w: snapshot version %q isn't a commit sha", mounts.ErrInvalidArgument, m.Version)
	}
	key := m.Name + "@" + m.Version
	d.mu.Lock()
	t := d.lazyTrees[key]
	d.mu.Unlock()
	if t != nil {
		return t, nil
	}
	file, keep := d.listingFile(m)
	var all []mounts.File
	if b, err := os.ReadFile(file); keep && err == nil && json.Unmarshal(b, &all) == nil {
		t = buildLazyTree(all)
	} else {
		if all, err = d.treeAt(ctx, m, creds, m.Version); err != nil {
			return nil, err
		}
		if keep {
			if err := writeListing(file, all); err != nil {
				return nil, err
			}
		}
		t = buildLazyTree(all)
	}
	d.mu.Lock()
	if d.lazyTrees == nil || len(d.lazyTrees) >= treeCacheSize {
		d.lazyTrees = map[string]*lazyTree{}
	}
	d.lazyTrees[key] = t
	d.mu.Unlock()
	return t, nil
}

func buildLazyTree(all []mounts.File) *lazyTree {
	t := &lazyTree{all: all, files: make(map[string]mounts.File, len(all)), dirs: map[string][]fs.DirEntry{".": nil}}
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

// under returns the files at or under p, an empty p being the whole tree.
func (t *lazyTree) under(p string) []mounts.File {
	if p == "" {
		return t.all
	}
	var out []mounts.File
	for _, f := range t.all {
		if f.Path == p || strings.HasPrefix(f.Path, p+"/") {
			out = append(out, f)
		}
	}
	return out
}

// listingFile is where m's listing is kept, beside its view. ok is false when the driver keeps no
// views, or m's name can't name a folder.
func (d *Driver) listingFile(m mounts.Mount) (string, bool) {
	if !d.viewable() || mounts.ValidName(m.Name) != nil || !commitSHA.MatchString(m.Version) {
		return "", false
	}
	root := filepath.Clean(d.views)
	file := filepath.Clean(filepath.Join(root, m.Name, m.Version+".tree.json"))
	if !strings.HasPrefix(file, root+string(filepath.Separator)) {
		return "", false
	}
	return file, true
}

// viewable reports whether the driver builds views: a views folder and a blob store that can link.
func (d *Driver) viewable() bool {
	_, ok := d.blobs.(blobs.Linker)
	return d.views != "" && ok
}

// writeListing writes all to file through a temp file and a rename.
func writeListing(file string, all []mounts.File) error {
	b, err := json.Marshal(all)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".tree-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// lazyFS serves a lazy snapshot. Listing a folder reads only the tree. Opening a file fetches it
// first: into the view when the driver keeps views, or into the blob store when it doesn't.
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
		if l.d.viewable() {
			p, err := l.d.HostPathWith(l.ctx, l.m, l.creds, name)
			if err != nil {
				return nil, pathErr("open", name, err)
			}
			fh, err := os.Open(p)
			if err != nil {
				return nil, err
			}
			return &file{info: info{name: path.Base(name), size: f.Size}, rc: fh}, nil
		}
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
// fetches the file p names, or every file under the folder p names, into the view. The files under
// p must fit the driver's Limits, or it returns ErrTooLarge and fetches nothing, so a call for the
// root of a big repo fails while a call for one board in it works. A file already in the view is
// not fetched again. A snapshot with a Manifest, and a live mount, are handled as HostPath handles
// them.
func (d *Driver) HostPathWith(ctx context.Context, m mounts.Mount, creds mounts.Credentials, p string) (string, error) {
	if !lazy(m) {
		return d.HostPath(m, p)
	}
	clean, err := mounts.CleanPath(p)
	if err != nil {
		return "", err
	}
	if !d.viewable() {
		return "", fmt.Errorf("%w: GitHub mount %q", mounts.ErrNotLocal, m.Name)
	}
	if err := mounts.ValidName(m.Name); err != nil {
		return "", err
	}
	t, err := d.lazyTree(ctx, m, creds)
	if err != nil {
		return "", err
	}
	files := t.under(clean)
	if err := d.within(files, clean, m.Version); err != nil {
		return "", err
	}
	view := filepath.Join(d.views, m.Name, m.Version)
	unlock := d.lockView(view)
	defer unlock()
	if err := os.MkdirAll(view, 0o750); err != nil {
		return "", err
	}
	linker := d.blobs.(blobs.Linker)
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

// within checks files, the files under one path at commit, against the driver's Limits.
func (d *Driver) within(files []mounts.File, under, commit string) error {
	var total int64
	for _, f := range files {
		total += f.Size
	}
	if len(files) > d.limits.MaxFiles || total > d.limits.MaxBytes {
		return fmt.Errorf("%w: %s holds %d files and %d bytes at %s, over the cap of %d files and %d bytes", mounts.ErrTooLarge, shown(under), len(files), total, commit, d.limits.MaxFiles, d.limits.MaxBytes)
	}
	return nil
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
