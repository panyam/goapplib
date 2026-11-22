package blobstest

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
)

// RunCached checks blobs.Cached over remotes from newRemote, each in front of a fresh local Dir.
// It runs RunStore and RunLinker on the Cached, then the checks of the local copy. A remote
// store's test calls it to show the store can back folder views.
func RunCached(t *testing.T, newRemote Factory) {
	ctx := context.Background()
	cached := func(t *testing.T, opts ...blobs.CachedOption) (Fixture, Fixture, *blobs.Dir, string) {
		remote := newRemote(t)
		root := t.TempDir()
		local := newDir(t, root)
		c := blobs.NewCached(remote.Store, local, opts...)
		return Fixture{
			Store: c,
			Later: func(t *testing.T, by time.Duration) {
				remote.Later(t, by)
				AgeDir(t, root, by)
			},
			Corrupt: func(t *testing.T, h blobs.Hash, b []byte) {
				remote.Corrupt(t, h, b)
				p, err := local.Path(h)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, b, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		}, remote, local, root
	}
	factory := func(t *testing.T) Fixture {
		f, _, _, _ := cached(t)
		return f
	}

	t.Run("Store", func(t *testing.T) { RunStore(t, factory) })
	t.Run("Linker", func(t *testing.T) { RunLinker(t, factory) })

	t.Run("PutWritesTheRemote", func(t *testing.T) {
		f, remote, _, _ := cached(t)
		data := []byte("on both sides")
		h := put(t, f.Store, data)
		if got, err := read(remote.Store, h); err != nil || !bytes.Equal(got, data) {
			t.Errorf("the remote read %q, %v after Put", got, err)
		}
	})

	t.Run("LostLocalFolderRefills", func(t *testing.T) {
		f, remote, _, root := cached(t)
		data := random(t, 256<<10)
		h := put(t, f.Store, data)
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		restarted := blobs.NewCached(remote.Store, newDir(t, root))
		if got, err := read(restarted, h); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("Open after losing the local folder read %d bytes, %v", len(got), err)
		}
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		restarted = blobs.NewCached(remote.Store, newDir(t, root))
		to := filepath.Join(t.TempDir(), "file")
		if err := restarted.Link(h, to); err != nil {
			t.Fatalf("Link after losing the local folder: %v", err)
		}
		if got, err := os.ReadFile(to); err != nil || !bytes.Equal(got, data) {
			t.Errorf("the linked file read %d bytes, %v", len(got), err)
		}
	})

	t.Run("CorruptRemoteIsNeverKept", func(t *testing.T) {
		f, remote, local, _ := cached(t)
		h := put(t, remote.Store, []byte("original bytes"))
		remote.Corrupt(t, h, []byte("changed bytes!"))
		if _, err := read(f.Store, h); !errors.Is(err, blobs.ErrCorrupt) {
			t.Errorf("Open of a corrupt remote blob = %v, want ErrCorrupt", err)
		}
		if err := f.Store.(blobs.Linker).Link(h, filepath.Join(t.TempDir(), "file")); !errors.Is(err, blobs.ErrCorrupt) {
			t.Errorf("Link of a corrupt remote blob = %v, want ErrCorrupt", err)
		}
		if ok, err := local.Has(ctx, h); ok || err != nil {
			t.Errorf("the local Dir has the corrupt blob: %v, %v", ok, err)
		}
	})

	t.Run("SweepDropsLocalCopiesWithTheRemote", func(t *testing.T) {
		f, remote, local, _ := cached(t)
		a, b := put(t, f.Store, []byte("a")), put(t, f.Store, []byte("b"))
		f.Later(t, time.Hour)
		if n, err := f.Store.Sweep(ctx, 10*time.Minute, keep(a)); err != nil || n != 1 {
			t.Fatalf("Sweep = %d, %v; want 1 (b)", n, err)
		}
		for _, s := range []blobs.Store{remote.Store, local} {
			if ok, _ := s.Has(ctx, a); !ok {
				t.Errorf("%T lost the referenced blob", s)
			}
			if ok, _ := s.Has(ctx, b); ok {
				t.Errorf("%T still has the swept blob", s)
			}
		}
	})

	t.Run("MaxAgeDropsOldLocalCopies", func(t *testing.T) {
		f, _, local, _ := cached(t, blobs.WithMaxAge(24*time.Hour))
		data := []byte("still wanted")
		h := put(t, f.Store, data)
		fresh := put(t, f.Store, []byte("fresh"))
		f.Later(t, 48*time.Hour)
		put(t, f.Store, []byte("fresh"))
		if _, err := f.Store.Sweep(ctx, 10*time.Minute, keep(h, fresh)); err != nil {
			t.Fatal(err)
		}
		if ok, _ := local.Has(ctx, h); ok {
			t.Error("Sweep kept a local copy older than the max age")
		}
		if ok, _ := local.Has(ctx, fresh); !ok {
			t.Error("Sweep dropped a local copy younger than the max age")
		}
		if got, err := read(f.Store, h); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("Open after the local copy aged out read %q, %v", got, err)
		}
		if ok, _ := local.Has(ctx, h); !ok {
			t.Error("Open didn't keep the refetched copy")
		}
	})
}

// AgeDir sets back the mtime of every file under root by by, which is how a Fixture for a store on
// disk, such as a blobs.Dir, implements Later.
func AgeDir(t *testing.T, root string, by time.Duration) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		fi, err := de.Info()
		if err != nil {
			return err
		}
		old := fi.ModTime().Add(-by)
		return os.Chtimes(p, old, old)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newDir(t *testing.T, root string) *blobs.Dir {
	t.Helper()
	d, err := blobs.NewDir(root)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
