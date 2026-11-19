package github

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"time"
)

// info is the fs.FileInfo and fs.DirEntry for a file or folder in a mount. GitHub keeps no
// modification times per file, so ModTime is zero.
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

// dirFile is an open folder, listing entries in name order.
type dirFile struct {
	info    info
	entries []fs.DirEntry
	off     int
}

func newDir(name string, entries []fs.DirEntry) *dirFile {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return &dirFile{info: info{name: path.Base(name), dir: true}, entries: entries}
}

func (f *dirFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *dirFile) Close() error               { return nil }
func (f *dirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: f.info.name, Err: fs.ErrInvalid}
}

func (f *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := f.entries[f.off:]
	if n <= 0 {
		f.off = len(f.entries)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if n > len(rest) {
		n = len(rest)
	}
	f.off += n
	return rest[:n], nil
}

// file is an open file whose contents come from rc.
type file struct {
	info info
	rc   io.ReadCloser
}

func (f *file) Stat() (fs.FileInfo, error)     { return f.info, nil }
func (f *file) Read(p []byte) (int, error)     { return f.rc.Read(p) }
func (f *file) Close() error                   { return f.rc.Close() }
func pathErr(op, name string, err error) error { return &fs.PathError{Op: op, Path: name, Err: err} }
