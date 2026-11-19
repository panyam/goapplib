package folder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

// Moves returns true. A folder has no history, so a sync always reads it as it is now.
func (d *Driver) Moves(ctx context.Context, source, ref string, ts oauth2.TokenSource) (bool, error) {
	return true, nil
}

// ListAt walks the folder at under as it is now, or returns the one file under names. A folder has
// no versions, so any version but "" returns ErrInvalidArgument. Links and anything else that isn't
// a regular file are left out, and a path through a link returns ErrInvalidPath.
func (d *Driver) ListAt(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, version, under string) ([]mounts.File, error) {
	if version != "" {
		return nil, fmt.Errorf("%w: a folder has no versions, got %q", mounts.ErrInvalidArgument, version)
	}
	fsys := linkFree{dir: m.Source}
	start := under
	if start == "" {
		start = "."
	}
	info, err := fs.Stat(fsys, start)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode().IsRegular() {
		return []mounts.File{{Path: under, Size: info.Size()}}, nil
	}
	var out []mounts.File
	var total int64
	limits := mounts.DefaultLimits
	err = fs.WalkDir(fsys, start, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !de.Type().IsRegular() {
			return nil
		}
		fi, err := de.Info()
		if err != nil {
			return err
		}
		out = append(out, mounts.File{Path: path.Clean(p), Size: fi.Size()})
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

// FetchAt reads f from the folder and keeps it in the driver's blob store. fetched is true when
// the store didn't hold the contents yet.
func (d *Driver) FetchAt(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, version string, f mounts.File) (blobs.Hash, bool, error) {
	if d.blobs == nil {
		return "", false, fmt.Errorf("%w: this folder driver has no blob store; pass WithBlobs", mounts.ErrInvalidArgument)
	}
	if version != "" {
		return "", false, fmt.Errorf("%w: a folder has no versions, got %q", mounts.ErrInvalidArgument, version)
	}
	file, err := linkFree{dir: m.Source}.Open(f.Path)
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

var _ mounts.VersionReader = (*Driver)(nil)
