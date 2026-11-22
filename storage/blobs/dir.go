package blobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Dir is a Store and a Linker over a folder on this server's disk. A blob lives at
// <root>/sha256/ab/cdef..., which is agni's layout, so agni's existing blobs folder works as a
// Dir's root unchanged.
//
// Writes go through a temp file and a rename, so a reader sees a whole blob or none, and several
// processes can share one Dir. Every read and removal goes through os.Root, so a link planted in
// the folder can't lead outside it.
type Dir struct{ root string }

// NewDir returns the Dir rooted at root, creating its folder if absent.
func NewDir(root string) (*Dir, error) {
	d := &Dir{root: filepath.Join(root, "sha256")}
	if err := os.MkdirAll(d.root, 0o750); err != nil {
		return nil, err
	}
	return d, nil
}

func rel(h Hash) (string, error) {
	x, err := hexOf(h)
	if err != nil {
		return "", err
	}
	return path.Join(x[:2], x[2:]), nil
}

// Path returns where h's blob is on disk, whether or not the Dir holds it, for a caller that
// builds a view some other way than Link. It returns ErrInvalidHash for a malformed h.
func (d *Dir) Path(h Hash) (string, error) {
	r, err := rel(h)
	if err != nil {
		return "", err
	}
	return filepath.Join(d.root, filepath.FromSlash(r)), nil
}

// Put implements Store.
func (d *Dir) Put(ctx context.Context, r io.Reader) (Hash, error) {
	tmp, err := os.CreateTemp(d.root, ".put-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	sum := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, sum), r); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	h := Hash("sha256:" + hex.EncodeToString(sum.Sum(nil)))
	p, _ := d.Path(h)
	if _, err := os.Stat(p); err == nil {
		now := time.Now()
		return h, os.Chtimes(p, now, now)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return "", err
	}
	return h, nil
}

// Open implements Store.
func (d *Dir) Open(ctx context.Context, h Hash) (io.ReadCloser, error) {
	r, err := rel(h)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(d.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(r)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, h)
	}
	if err != nil {
		return nil, err
	}
	return Verify(f, h), nil
}

// Has implements Store.
func (d *Dir) Has(ctx context.Context, h Hash) (bool, error) {
	p, err := d.Path(h)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Sweep implements Store. It also removes temp files older than grace, which a Put that crashed
// left behind.
func (d *Dir) Sweep(ctx context.Context, grace time.Duration, refs ...Referrer) (int, error) {
	keep := map[Hash]bool{}
	for _, ref := range refs {
		if err := ref(ctx, func(h Hash) { keep[h] = true }); err != nil {
			return 0, err
		}
	}
	cutoff := time.Now().Add(-grace)
	root, err := os.OpenRoot(d.root)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	removed := 0
	err = fs.WalkDir(root.FS(), ".", func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		temp := strings.HasPrefix(de.Name(), ".")
		if !temp {
			h := Hash("sha256:" + strings.ReplaceAll(p, "/", ""))
			if _, err := hexOf(h); err != nil || keep[h] {
				return nil
			}
		}
		fi, err := de.Info()
		if err != nil || fi.ModTime().After(cutoff) {
			return err
		}
		if err := root.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("sweeping %s: %w", p, err)
		}
		if !temp {
			removed++
		}
		return nil
	})
	return removed, err
}

var _ Linker = (*Dir)(nil)

// Link implements Linker. The copy fallback reads through Open, so it copies only a blob that
// still matches its hash.
func (d *Dir) Link(h Hash, to string) error {
	from, err := d.Path(h)
	if err != nil {
		return err
	}
	if ok, err := d.Has(context.Background(), h); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, h)
	}
	if err := os.Link(from, to); err == nil {
		return nil
	}
	r, err := d.Open(context.Background(), h)
	if err != nil {
		return err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o600)
}
