package folder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

// Moves returns true. A folder has no history, so a sync always reads it as it is now.
func (d *Driver) Moves(ctx context.Context, source, ref string, ts oauth2.TokenSource) (bool, error) {
	return true, nil
}

// versionPrefix marks a folder's version, so it can't be mistaken for a commit.
const versionPrefix = "mtime:"

// checkVersion accepts "" or a version Resolve returned. A folder is read only as it is now, so
// either one reads the same files. Anything else returns ErrInvalidArgument.
func checkVersion(version string) error {
	if version != "" && !strings.HasPrefix(version, versionPrefix) {
		return fmt.Errorf("%w: a folder's version starts with %q, got %q", mounts.ErrInvalidArgument, versionPrefix, version)
	}
	return nil
}

// ListAt walks the folder at under as it is now, or returns the one file under names. version is
// "" or one Resolve returned, and is ignored, since a folder can't be read as of an older version.
// Any other version returns ErrInvalidArgument. Links and anything else that isn't a regular file
// are left out, and a path through a link returns ErrInvalidPath.
func (d *Driver) ListAt(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, version, under string) ([]mounts.File, error) {
	if err := checkVersion(version); err != nil {
		return nil, err
	}
	var out []mounts.File
	var total int64
	limits := mounts.DefaultLimits
	err := d.walk(ctx, m.Source, under, func(p string, fi fs.FileInfo) error {
		out = append(out, mounts.File{Path: p, Size: fi.Size()})
		total += fi.Size()
		if len(out) > limits.MaxFiles || total > limits.MaxBytes {
			return fmt.Errorf("%w: %q holds over %d files or %d bytes", mounts.ErrTooLarge, under, limits.MaxFiles, limits.MaxBytes)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// version hashes each regular file's path, size and modification time in path order, and reads no
// file's contents. A version that didn't move means nothing changed, short of a write that kept
// both size and mtime. One that moved may still hold the same bytes, after a touch.
func (d *Driver) version(ctx context.Context, dir string) (string, error) {
	h := sha256.New()
	n := 0
	err := d.walk(ctx, dir, "", func(p string, fi fs.FileInfo) error {
		if n++; n > mounts.DefaultLimits.MaxFiles {
			return fmt.Errorf("%w: %q holds over %d files", mounts.ErrTooLarge, dir, mounts.DefaultLimits.MaxFiles)
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", p, fi.Size(), fi.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	return versionPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

// walk calls fn for each regular file at or under the path under, in path order, with its clean
// slash path and its info. It follows the driver's link rules, and calls nothing for a path that
// names nothing.
func (d *Driver) walk(ctx context.Context, dir, under string, fn func(p string, fi fs.FileInfo) error) error {
	fsys := linkFree{dir: dir, follow: d.follow}
	start := under
	if start == "" {
		start = "."
	}
	info, err := fs.Stat(fsys, start)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().IsRegular() {
		return fn(under, info)
	}
	// Two links can make a loop, so a walk that follows links visits each real folder once.
	seen := map[string]bool{}
	return fs.WalkDir(fsys, start, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if de.IsDir() && d.follow {
			real, err := filepath.EvalSymlinks(filepath.Join(dir, filepath.FromSlash(p)))
			if err != nil {
				return err
			}
			if seen[real] {
				return fs.SkipDir
			}
			seen[real] = true
		}
		if !de.Type().IsRegular() {
			return nil
		}
		fi, err := de.Info()
		if err != nil {
			return err
		}
		return fn(path.Clean(p), fi)
	})
}

// FetchAt reads f from the folder as it is now and keeps it in the driver's blob store. fetched is
// true when the store didn't hold the contents yet. version is checked as ListAt checks it.
func (d *Driver) FetchAt(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, version string, f mounts.File) (blobs.Hash, bool, error) {
	if d.blobs == nil {
		return "", false, fmt.Errorf("%w: this folder driver has no blob store; pass WithBlobs", mounts.ErrInvalidArgument)
	}
	if err := checkVersion(version); err != nil {
		return "", false, err
	}
	file, err := linkFree{dir: m.Source, follow: d.follow}.Open(f.Path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	b, err := io.ReadAll(file)
	if err != nil {
		return "", false, err
	}
	h := blobs.HashOf(b)
	had, err := d.blobs.Has(ctx, h)
	if err != nil {
		return "", false, err
	}
	if !had {
		if _, err := d.blobs.Put(ctx, bytes.NewReader(b)); err != nil {
			return "", false, err
		}
	}
	return h, !had, nil
}

// ListRefs returns an empty page. A folder has no tags and no commits.
func (d *Driver) ListRefs(ctx context.Context, source string, req mounts.ListRefsRequest, ts oauth2.TokenSource) (mounts.RefPage, error) {
	return mounts.RefPage{}, nil
}

var (
	_ mounts.VersionReader = (*Driver)(nil)
	_ mounts.RefLister     = (*Driver)(nil)
)
