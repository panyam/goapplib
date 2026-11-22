// Package folder is the mounts driver for a folder on the server's disk. Its mounts are live and
// writable, and have host paths. Only the server's configuration makes folder mounts (the registry
// refuses one from Create), so a client never picks the folder.
//
// Every operation refuses links anywhere in a path, so a link inside the folder can't reach a file
// outside it, or a second name for a file inside it. Listings leave out links, devices and anything
// else that isn't a regular file or a folder.
package folder

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

// Driver serves mounts.KindFolder mounts, whose Source is the folder's absolute path. It
// implements mounts.Writer, mounts.Localizer and mounts.VersionReader. It ignores token sources.
//
// A write's IfMatch check and the write itself happen under a lock per file held in this Driver,
// so two writers in one process can't both pass the check. Writers in two processes, or two
// Drivers, can. Share one Driver per process.
type Driver struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	blobs blobs.Store
	// follow lets reads go through a link whose target stays inside the folder. WithLinksInside
	// sets it.
	follow bool
}

// Option configures a Driver.
type Option func(*Driver)

// WithBlobs sets the blob store FetchAt keeps files in. Without it, FetchAt returns
// ErrInvalidArgument.
func WithBlobs(store blobs.Store) Option { return func(d *Driver) { d.blobs = store } }

// WithLinksInside lets reads follow a link whose target stays inside the mount's folder: listings,
// Open, HostPath, ListAt and FetchAt. A link that leads outside, directly or through another link,
// is still refused with ErrInvalidPath and left out of listings. Writes never go through a link.
// Without it, every link is refused, which is the default.
func WithLinksInside() Option { return func(d *Driver) { d.follow = true } }

// New returns a folder driver.
func New(opts ...Option) *Driver {
	d := &Driver{locks: map[string]*sync.Mutex{}}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Kind returns mounts.KindFolder.
func (d *Driver) Kind() mounts.Kind { return mounts.KindFolder }

// Modes returns mounts.ModeLive. A folder mount always shows the folder as it is now.
func (d *Driver) Modes() []mounts.Mode { return []mounts.Mode{mounts.ModeLive} }

// Resolve checks that source is an absolute path to a folder, and returns its version: "mtime:"
// and a hash over each regular file's path, size and modification time (goapplib issue 178). It
// reads no file's contents, so an app polls it to see whether a followed folder changed, and reads
// the files only when the version moved. A folder has no refs, so a non-empty ref returns
// ErrInvalidArgument, as does a relative path or a file. A missing folder returns ErrNotFound, and
// one over mounts.DefaultLimits' file count returns ErrTooLarge.
func (d *Driver) Resolve(ctx context.Context, source, ref string, ts oauth2.TokenSource) (string, error) {
	if !filepath.IsAbs(source) {
		return "", fmt.Errorf("%w: folder %q isn't an absolute path", mounts.ErrInvalidArgument, source)
	}
	if ref != "" {
		return "", fmt.Errorf("%w: a folder has no refs, got %q", mounts.ErrInvalidArgument, ref)
	}
	info, err := os.Stat(source)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: folder %q", mounts.ErrNotFound, source)
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %q isn't a folder", mounts.ErrInvalidArgument, source)
	}
	return d.version(ctx, source)
}

// Open returns the folder's files as they are now. The fs.FS refuses a path through a link with an
// error wrapping ErrInvalidPath, and its listings leave links out, so fs.WalkDir sees only regular
// files and folders. Opening a Git LFS pointer fails with ErrLFS, naming the path.
func (d *Driver) Open(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource) (fs.FS, error) {
	return linkFree{dir: m.Source, follow: d.follow}, nil
}

// Stat returns the file's version, "sha256:" and the hex SHA-256 of its contents. Content hashes,
// unlike modification times, don't change when nothing did, and can't collide when a file changes
// twice in one clock tick.
func (d *Driver) Stat(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, p string) (mounts.FileVersion, error) {
	p, err := filePath(p)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(m.Source)
	if err != nil {
		return "", err
	}
	defer root.Close()
	v, _, err := current(root, p)
	if err == nil && v == "" {
		err = fmt.Errorf("%w: %q", mounts.ErrNotFound, p)
	}
	return v, err
}

// Put writes req.Data to req.Path through a temp file and a rename, so a reader sees the old file
// or the new one and never part of either. It creates missing parent folders. The IfMatch check
// follows mounts.PutRequest. The result's Mount version is empty, since a folder has none.
func (d *Driver) Put(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, req mounts.PutRequest) (*mounts.WriteResult, error) {
	p, err := filePath(req.Path)
	if err != nil {
		return nil, err
	}
	unlock := d.lock(m.Source, p)
	defer unlock()
	root, err := os.OpenRoot(m.Source)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	v, _, err := current(root, p)
	if err != nil {
		return nil, err
	}
	switch {
	case req.IfMatch == mounts.AnyVersion:
	case req.IfMatch == "" && v != "":
		return nil, fmt.Errorf("%w: %q exists", mounts.ErrConflict, p)
	case req.IfMatch != "" && req.IfMatch != v:
		return nil, fmt.Errorf("%w: %q changed", mounts.ErrConflict, p)
	}
	if dir := path.Dir(p); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	tmp := path.Join(path.Dir(p), "."+path.Base(p)+".tmp-"+rand.Text())
	if err := root.WriteFile(tmp, req.Data, 0o644); err != nil {
		return nil, err
	}
	if err := root.Rename(tmp, p); err != nil {
		root.Remove(tmp)
		return nil, err
	}
	return &mounts.WriteResult{File: version(req.Data)}, nil
}

// Delete removes req.Path when IfMatch is its current version or AnyVersion. Empty IfMatch never
// matches an existing file, so it returns ErrConflict. A missing file returns ErrNotFound. Delete
// leaves empty parent folders in place.
func (d *Driver) Delete(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, req mounts.DeleteRequest) (*mounts.WriteResult, error) {
	p, err := filePath(req.Path)
	if err != nil {
		return nil, err
	}
	unlock := d.lock(m.Source, p)
	defer unlock()
	root, err := os.OpenRoot(m.Source)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	v, _, err := current(root, p)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, fmt.Errorf("%w: %q", mounts.ErrNotFound, p)
	}
	if req.IfMatch != mounts.AnyVersion && req.IfMatch != v {
		return nil, fmt.Errorf("%w: %q changed", mounts.ErrConflict, p)
	}
	if err := root.Remove(p); err != nil {
		return nil, err
	}
	return &mounts.WriteResult{}, nil
}

// HostPath returns the absolute path of p in the folder. The file need not exist, so a native tool
// can write its output there. A path through a link returns ErrInvalidPath, and a file that is a
// Git LFS pointer returns ErrLFS. A folder's path is returned without looking at the files in it.
func (d *Driver) HostPath(m mounts.Mount, p string) (string, error) {
	p, err := mounts.CleanPath(p)
	if err != nil {
		return "", err
	}
	if p == "" {
		return m.Source, nil
	}
	root, err := os.OpenRoot(m.Source)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := check(root, p, d.follow)
	if err != nil {
		return "", err
	}
	if info != nil && info.Mode().IsRegular() && info.Size() <= mounts.MaxLFSPointerSize {
		f, err := root.Open(p)
		if err != nil {
			return "", err
		}
		err = refusePointer(f, info, p)
		f.Close()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(m.Source, filepath.FromSlash(p)), nil
}

func (d *Driver) lock(dir, p string) func() {
	key := dir + "\x00" + p
	d.mu.Lock()
	l, ok := d.locks[key]
	if !ok {
		l = &sync.Mutex{}
		d.locks[key] = l
	}
	d.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func version(b []byte) mounts.FileVersion {
	sum := sha256.Sum256(b)
	return mounts.FileVersion("sha256:" + hex.EncodeToString(sum[:]))
}

// filePath cleans p and refuses the mount's root, which isn't a file.
func filePath(p string) (string, error) {
	p, err := mounts.CleanPath(p)
	if err == nil && p == "" {
		err = fmt.Errorf("%w: the mount's root isn't a file", mounts.ErrInvalidPath)
	}
	return p, err
}

// check walks p's components and refuses a link, or anything but a folder or a regular file, at
// any of them. It returns the last component's info, or nil when some component doesn't exist.
func check(root *os.Root, p string, follow bool) (fs.FileInfo, error) {
	parts := strings.Split(p, "/")
	var info fs.FileInfo
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		var err error
		info, err = root.Lstat(prefix)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			if !follow {
				return nil, fmt.Errorf("%w: %q is a link", mounts.ErrInvalidPath, prefix)
			}
			// os.Root follows a link only while it stays inside the root, and fails one that
			// leads out, so Stat through it decides.
			if info, err = root.Stat(prefix); errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			} else if err != nil {
				return nil, fmt.Errorf("%w: %q is a link that leads outside the folder", mounts.ErrInvalidPath, prefix)
			}
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil, fmt.Errorf("%w: %q isn't a file or a folder", mounts.ErrInvalidPath, prefix)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("%w: %q isn't a folder", mounts.ErrInvalidPath, prefix)
		}
	}
	return info, nil
}

// current returns the version of the file at p, or "" when it doesn't exist.
func current(root *os.Root, p string) (mounts.FileVersion, fs.FileInfo, error) {
	info, err := check(root, p, false)
	if err != nil || info == nil {
		return "", nil, err
	}
	if info.IsDir() {
		return "", nil, fmt.Errorf("%w: %q is a folder", mounts.ErrInvalidPath, p)
	}
	b, err := root.ReadFile(p)
	if err != nil {
		return "", nil, err
	}
	return version(b), info, nil
}

// linkFree is an fs.FS over a folder that refuses paths through links and leaves links out of
// listings.
type linkFree struct {
	dir    string
	follow bool
}

func (l linkFree) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	root, err := os.OpenRoot(l.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close() // files opened from a Root stay open after it closes
	if name != "." {
		info, err := check(root, name, l.follow)
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		if info == nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		return &dirFile{File: f, dir: l.dir, name: name, follow: l.follow}, nil
	}
	if err := refusePointer(f, info, name); err != nil {
		f.Close()
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return f, nil
}

// refusePointer returns mounts.ErrLFS when f is a Git LFS pointer, which a checkout made without
// "git lfs pull" holds in place of each LFS file. The folder has no remote to fetch the object
// from. It leaves f at its start.
func refusePointer(f *os.File, info fs.FileInfo, name string) error {
	if !info.Mode().IsRegular() || info.Size() > mounts.MaxLFSPointerSize {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(f, mounts.MaxLFSPointerSize+1))
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if ptr, ok := mounts.ParseLFSPointer(b); ok {
		return fmt.Errorf("%w: %s is a pointer to LFS object %s; run git lfs pull in the folder", mounts.ErrLFS, name, ptr.OID)
	}
	return nil
}

func (l linkFree) ReadDir(name string) ([]fs.DirEntry, error) {
	f, err := l.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d, ok := f.(*dirFile)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errors.New("not a folder")}
	}
	return d.ReadDir(-1)
}

// dirFile is an open folder whose ReadDir leaves out everything but regular files and folders.
type dirFile struct {
	*os.File
	entries []fs.DirEntry
	read    bool
	// dir, name and follow let ReadDir list a link as what it points at, when the link stays
	// inside the folder.
	dir, name string
	follow    bool
}

func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if !d.read {
		all, err := d.File.ReadDir(-1)
		if err != nil {
			return nil, err
		}
		for _, e := range all {
			if e.Type().IsRegular() || e.IsDir() {
				d.entries = append(d.entries, e)
			} else if e.Type()&fs.ModeSymlink != 0 && d.follow {
				if t, ok := d.target(e.Name()); ok {
					d.entries = append(d.entries, t)
				}
			}
		}
		sort.Slice(d.entries, func(i, j int) bool { return d.entries[i].Name() < d.entries[j].Name() })
		d.read = true
	}
	if n <= 0 {
		out := d.entries
		d.entries = nil
		return out, nil
	}
	if len(d.entries) == 0 {
		return nil, io.EOF
	}
	k := min(n, len(d.entries))
	out := d.entries[:k]
	d.entries = d.entries[k:]
	return out, nil
}

// target is the entry a link in this folder lists as: what it points at, under the link's name,
// when that is a regular file or a folder inside the root.
func (d *dirFile) target(name string) (fs.DirEntry, bool) {
	root, err := os.OpenRoot(d.dir)
	if err != nil {
		return nil, false
	}
	defer root.Close()
	info, err := root.Stat(path.Join(d.name, name))
	if err != nil || !info.Mode().IsRegular() && !info.IsDir() {
		return nil, false
	}
	return fs.FileInfoToDirEntry(info), true
}
