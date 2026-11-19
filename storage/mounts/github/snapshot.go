package github

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

// snapshot streams the tarball of commit into the blob store and returns its manifest's hash.
// It keeps regular files only, so links and devices never reach the snapshot, and refuses a path
// that is absolute, unclean or climbs out of the repo.
func (d *Driver) snapshot(ctx context.Context, rp, commit string, ts oauth2.TokenSource) (blobs.Hash, error) {
	if !commitSHA.MatchString(commit) {
		return "", fmt.Errorf("%w: snapshot version %q isn't a commit sha", mounts.ErrInvalidArgument, commit)
	}
	resp, err := d.send(ctx, ts, http.MethodGet, rp+"/tarball/"+commit, nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := status(resp, false); err != nil {
		return "", err
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: the tarball isn't gzip: %v", mounts.ErrUnavailable, err)
	}
	tr := tar.NewReader(gz)
	var m blobs.Manifest
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w: reading the tarball: %v", mounts.ErrUnavailable, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		p, err := tarPath(hdr.Name)
		if err != nil {
			return "", err
		}
		total += hdr.Size
		if len(m)+1 > d.limits.MaxFiles || total > d.limits.MaxBytes {
			return "", fmt.Errorf("%w: the commit has more than %d files or %d bytes", mounts.ErrTooLarge, d.limits.MaxFiles, d.limits.MaxBytes)
		}
		h, err := d.blobs.Put(ctx, io.LimitReader(tr, hdr.Size))
		if err != nil {
			return "", err
		}
		m = append(m, blobs.ManifestEntry{Path: p, Hash: h, Size: hdr.Size})
	}
	h, err := blobs.PutManifest(ctx, d.blobs, m)
	if errors.Is(err, blobs.ErrInvalidManifest) {
		return "", fmt.Errorf("%w: %v", mounts.ErrInvalidPath, err)
	}
	return h, err
}

// tarPath drops the tarball's top folder ("owner-repo-sha/") from name, and refuses a path that is
// absolute, unclean or climbs out.
func tarPath(name string) (string, error) {
	_, p, ok := strings.Cut(name, "/")
	if !ok || p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("%w: tarball entry %q", mounts.ErrInvalidPath, name)
	}
	return p, nil
}

// tree is a snapshot's files and folders, built from its manifest once and kept, since a manifest
// never changes.
type tree struct {
	files map[string]blobs.ManifestEntry
	dirs  map[string][]fs.DirEntry
}

func (d *Driver) tree(ctx context.Context, m mounts.Mount) (*tree, error) {
	h, err := blobs.ParseHash(m.Manifest)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot mount %q has no manifest", mounts.ErrInvalidArgument, m.Name)
	}
	if d.blobs == nil {
		return nil, fmt.Errorf("%w: this GitHub driver has no blob store for snapshots", mounts.ErrInvalidArgument)
	}
	d.mu.Lock()
	t := d.manifests[h]
	d.mu.Unlock()
	if t != nil {
		return t, nil
	}
	man, err := blobs.GetManifest(ctx, d.blobs, h)
	if err != nil {
		return nil, err
	}
	t = &tree{files: map[string]blobs.ManifestEntry{}, dirs: map[string][]fs.DirEntry{".": nil}}
	for _, e := range man {
		t.files[e.Path] = e
		child, name := e.Path, path.Base(e.Path)
		ent := fs.DirEntry(info{name: name, size: e.Size})
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
	d.mu.Lock()
	d.manifests[h] = t
	d.mu.Unlock()
	return t, nil
}

// snapshotFS serves a snapshot's files from the blob store. Reading a file to the end checks its
// hash, so a changed blob reads as blobs.ErrCorrupt.
type snapshotFS struct {
	ctx   context.Context
	store blobs.Store
	t     *tree
}

func (s snapshotFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	if e, ok := s.t.files[name]; ok {
		rc, err := s.store.Open(s.ctx, e.Hash)
		if err != nil {
			return nil, pathErr("open", name, err)
		}
		return &file{info: info{name: path.Base(name), size: e.Size}, rc: rc}, nil
	}
	if ents, ok := s.t.dirs[name]; ok {
		return newDir(name, append([]fs.DirEntry(nil), ents...)), nil
	}
	return nil, pathErr("open", name, fs.ErrNotExist)
}

// HostPath returns where p of a snapshot mount is on disk, in the folder WithViews set. The first
// call for a mount builds its view there, as hard links to the blob store (copies where the
// filesystem refuses links), so a restart that lost the folder rebuilds it from the manifest
// without asking GitHub. A live mount, or a driver without WithViews or a linking blob store,
// returns ErrNotLocal.
func (d *Driver) HostPath(m mounts.Mount, p string) (string, error) {
	clean, err := mounts.CleanPath(p)
	if err != nil {
		return "", err
	}
	linker, ok := d.blobs.(blobs.Linker)
	if m.Mode != mounts.ModeSnapshot || d.views == "" || !ok {
		return "", fmt.Errorf("%w: GitHub mount %q", mounts.ErrNotLocal, m.Name)
	}
	if err := mounts.ValidName(m.Name); err != nil {
		return "", err
	}
	if !commitSHA.MatchString(m.Version) {
		return "", fmt.Errorf("%w: snapshot version %q isn't a commit sha", mounts.ErrInvalidArgument, m.Version)
	}
	view := filepath.Join(d.views, m.Name, m.Version)
	if err := d.buildView(m, view, linker); err != nil {
		return "", err
	}
	return filepath.Join(view, filepath.FromSlash(clean)), nil
}

// buildView makes view if it is missing. It builds in a temp folder beside it and renames, so a
// half-built view is never visible, and two processes building at once leave one whole view.
func (d *Driver) buildView(m mounts.Mount, view string, linker blobs.Linker) error {
	d.viewMu.Lock()
	defer d.viewMu.Unlock()
	if _, err := os.Stat(view); err == nil {
		return nil
	}
	t, err := d.tree(context.Background(), m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(view), 0o750); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(view), ".build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for p, e := range t.files {
		to := filepath.Join(tmp, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
			return err
		}
		if err := linker.Link(e.Hash, to); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, view); err != nil {
		if _, statErr := os.Stat(view); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}
