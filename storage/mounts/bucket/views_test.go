package bucket_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/blobs/blobstest"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

func TestPrepareFailsWhenAPinnedFileChanged(t *testing.T) {
	ctx := context.Background()
	d, b, _ := newDriver(t, false, mountstest.DriverFiles, bucket.WithViews(t.TempDir()))
	m := mounts.Mount{Name: "jetson", Kind: mounts.KindS3, Source: "s3://designs/boards/jetson", Mode: mounts.ModeSnapshot}
	v, err := d.Resolve(ctx, m.Source, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	m.Version = v
	b.Put("boards/jetson/README.md", []byte("changed between Resolve and Prepare"))
	if _, err := d.Prepare(ctx, m, nil); !errors.Is(err, mounts.ErrConflict) {
		t.Fatalf("Prepare after a pinned file changed = %v, want ErrConflict", err)
	}
}

func TestSweepKeepsAViewsFiles(t *testing.T) {
	ctx := context.Background()
	views := t.TempDir()
	d, b, store := newDriver(t, true, mountstest.DriverFiles, bucket.WithViews(views))
	recs, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(recs, []mounts.Driver{d})
	if _, err := reg.Create(ctx, mounts.Mount{Name: "jetson", Kind: mounts.KindS3, Source: "s3://designs/boards/jetson", Mode: mounts.ModeSnapshot}); err != nil {
		t.Fatal(err)
	}
	ageStore(t, store)
	if n, err := store.Sweep(ctx, time.Minute, reg.BlobRefs(store)); err != nil || n != 0 {
		t.Fatalf("Sweep = %d, %v; want nothing removed", n, err)
	}
	b.Offline()
	hp, err := reg.HostPath(ctx, mounts.URI{Mount: "jetson", Path: "board/main.kicad_sch"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(hp); err != nil || string(got) != mountstest.DriverFiles["board/main.kicad_sch"] {
		t.Errorf("the host file read %q, %v", got, err)
	}
	if !strings.HasPrefix(hp, views+string(filepath.Separator)) {
		t.Errorf("HostPath = %s, want it under the views folder %s", hp, views)
	}
}

func TestHostPathNeedsViewsAndALinker(t *testing.T) {
	ctx := context.Background()
	m := mounts.Mount{Name: "jetson", Kind: mounts.KindS3, Source: "s3://designs/boards/jetson", Mode: mounts.ModeSnapshot}
	prepare := func(t *testing.T, d *bucket.Driver) mounts.Mount {
		m := m
		v, err := d.Resolve(ctx, m.Source, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		m.Version = v
		if m, err = d.Prepare(ctx, m, nil); err != nil {
			t.Fatal(err)
		}
		return m
	}

	t.Run("NoViews", func(t *testing.T) {
		d, _, _ := newDriver(t, true, mountstest.DriverFiles)
		if _, err := d.HostPath(prepare(t, d), "README.md"); !errors.Is(err, mounts.ErrNotLocal) {
			t.Errorf("HostPath without views = %v, want ErrNotLocal", err)
		}
	})

	t.Run("NotALinker", func(t *testing.T) {
		_, b, store := newDriver(t, true, mountstest.DriverFiles)
		d := bucket.New(mounts.KindS3, "s3", b.Connector("designs"), notLinker{store}, bucket.WithViews(t.TempDir()))
		if _, err := d.HostPath(prepare(t, d), "README.md"); !errors.Is(err, mounts.ErrNotLocal) {
			t.Errorf("HostPath over a store that can't link = %v, want ErrNotLocal", err)
		}
	})

	t.Run("MadeBeforeViews", func(t *testing.T) {
		_, b, store := newDriver(t, true, mountstest.DriverFiles)
		old := prepare(t, bucket.New(mounts.KindS3, "s3", b.Connector("designs"), store))
		d := bucket.New(mounts.KindS3, "s3", b.Connector("designs"), store, bucket.WithViews(t.TempDir()))
		if _, err := d.HostPath(old, "README.md"); !errors.Is(err, mounts.ErrNotLocal) {
			t.Errorf("HostPath of a snapshot made before views = %v, want ErrNotLocal", err)
		}
	})
}

// notLinker hides a store's Link method.
type notLinker struct{ blobs.Store }

func ageStore(t *testing.T, d *blobs.Dir) {
	t.Helper()
	p, err := d.Path(blobs.HashOf(nil))
	if err != nil {
		t.Fatal(err)
	}
	blobstest.AgeDir(t, filepath.Dir(filepath.Dir(p)), time.Hour)
}

// TestOneFileWithoutAView gets a host file for one object of a snapshot, without WithViews, so
// creating the mount downloads nothing and only that object is fetched.
func TestOneFileWithoutAView(t *testing.T) {
	ctx := context.Background()
	d, b, store := newDriver(t, false, mountstest.DriverFiles)
	recs, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(recs, []mounts.Driver{d})
	m, err := reg.Create(ctx, mounts.Mount{Name: "jetson", Kind: mounts.KindS3, Source: "s3://designs/boards/jetson", Mode: mounts.ModeSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	files, err := reg.ListAt(ctx, m.Name, m.Version, "board/main.kicad_sch")
	if err != nil || len(files) != 1 {
		t.Fatalf("ListAt = %v, %v; want the one file", files, err)
	}
	h, _, err := reg.FetchAt(ctx, m.Name, m.Version, files[0])
	if err != nil {
		t.Fatal(err)
	}
	b.Put("boards/jetson/board/main.kicad_sch", []byte("changed after the fetch"))
	to := filepath.Join(t.TempDir(), "main.kicad_sch")
	if err := store.Link(h, to); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(to); err != nil || string(got) != mountstest.DriverFiles["board/main.kicad_sch"] {
		t.Errorf("the linked file read %q, %v; want the pinned bytes", got, err)
	}
	other := blobs.HashOf([]byte(mountstest.DriverFiles["README.md"]))
	if ok, _ := store.Has(ctx, other); ok {
		t.Error("the store holds README.md, which nothing fetched")
	}
}
