package bucket

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
)

// snapshotFS serves the objects of one listing at their pinned versions. Its folders come from the
// listing, so a listing never changes under it. With store set, it reads each file from the blob
// Prepare fetched, named in hashes, and never asks the bucket.
type snapshotFS struct {
	ctx    context.Context
	b      Bucket
	files  map[string]Object
	dirs   map[string][]fs.DirEntry
	store  blobs.Store
	hashes map[string]blobs.Hash
}

func newSnapshotFS(ctx context.Context, b Bucket, src Source, objs []Object) snapshotFS {
	s := snapshotFS{ctx: ctx, b: b, files: map[string]Object{}, dirs: map[string][]fs.DirEntry{".": nil}}
	for _, o := range objs {
		p := src.relPath(o.Key)
		if p == "" {
			continue
		}
		s.files[p] = o
		child, ent := p, fs.DirEntry(info{name: path.Base(p), size: o.Size})
		for {
			parent := path.Dir(child)
			_, seen := s.dirs[parent]
			s.dirs[parent] = append(s.dirs[parent], ent)
			if seen || parent == "." {
				break
			}
			child, ent = parent, info{name: path.Base(parent), dir: true}
		}
	}
	for _, ents := range s.dirs {
		sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	}
	return s
}

func (s snapshotFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if o, ok := s.files[name]; ok {
		if s.store != nil {
			rc, err := s.store.Open(s.ctx, s.hashes[name])
			if err != nil {
				return nil, &fs.PathError{Op: "open", Path: name, Err: err}
			}
			return &file{info: info{name: path.Base(name), size: o.Size}, rc: rc}, nil
		}
		rc, _, err := s.b.Get(s.ctx, o.Key, &o)
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		return &file{info: info{name: path.Base(name), size: o.Size}, rc: rc}, nil
	}
	if ents, ok := s.dirs[name]; ok {
		return &dir{info: info{name: path.Base(name), dir: true}, ents: append([]fs.DirEntry(nil), ents...)}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// liveFS serves a source's latest objects. A file reads its object's latest version, and a folder
// lists the bucket each time it is read, so a change shows on the next Open.
type liveFS struct {
	ctx context.Context
	b   Bucket
	src Source
}

func (l liveFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if name != "." {
		rc, size, err := l.b.Get(l.ctx, l.src.keyPrefix()+name, nil)
		if err == nil {
			return &file{info: info{name: path.Base(name), size: size}, rc: rc}, nil
		}
		if !errors.Is(err, mounts.ErrNotFound) {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
	}
	ents, err := l.readDir(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if len(ents) == 0 && name != "." {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &dir{info: info{name: path.Base(name), dir: true}, ents: ents}, nil
}

// readDir lists the folder name, leaving out keys that can't be a path in the mount.
func (l liveFS) readDir(name string) ([]fs.DirEntry, error) {
	d := l.src.keyPrefix()
	if name != "." {
		d += name + "/"
	}
	files, folders, err := l.b.ListDir(l.ctx, d)
	if err != nil {
		return nil, err
	}
	var ents []fs.DirEntry
	for _, o := range files {
		if p := l.src.relPath(o.Key); p != "" && !strings.Contains(strings.TrimPrefix(o.Key, d), "/") {
			ents = append(ents, info{name: path.Base(p), size: o.Size})
		}
	}
	for _, f := range folders {
		if f != "" && f != "." && f != ".." && !strings.ContainsAny(f, `/\`) {
			ents = append(ents, info{name: f, dir: true})
		}
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	return ents, nil
}

// info is a file's or folder's fs.FileInfo, and its fs.DirEntry. Objects carry no mode, and the
// mod time is zero, so a listing and an opened file agree.
type info struct {
	name string
	size int64
	dir  bool
}

func (i info) Name() string { return i.name }
func (i info) Size() int64  { return i.size }
func (i info) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i info) ModTime() time.Time         { return time.Time{} }
func (i info) IsDir() bool                { return i.dir }
func (i info) Sys() any                   { return nil }
func (i info) Type() fs.FileMode          { return i.Mode().Type() }
func (i info) Info() (fs.FileInfo, error) { return i, nil }

type file struct {
	info info
	rc   io.ReadCloser
}

func (f *file) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *file) Read(b []byte) (int, error) { return f.rc.Read(b) }
func (f *file) Close() error               { return f.rc.Close() }

type dir struct {
	info info
	ents []fs.DirEntry
	off  int
}

func (d *dir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *dir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.name, Err: errors.New("is a directory")}
}
func (d *dir) Close() error { return nil }

func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.ents[d.off:]
	if n <= 0 {
		d.off = len(d.ents)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if n > len(rest) {
		n = len(rest)
	}
	d.off += n
	return rest[:n], nil
}
