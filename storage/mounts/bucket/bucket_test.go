package bucket_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
	"github.com/panyam/goapplib/storage/mounts/bucket/buckettest"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

func newDriver(t *testing.T, versions bool, files map[string]string, opts ...bucket.Option) (*bucket.Driver, *buckettest.Bucket, *blobs.Dir) {
	t.Helper()
	b := buckettest.New(versions)
	for p, data := range files {
		b.Put("boards/jetson/"+p, []byte(data))
	}
	store, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return bucket.New(mounts.KindS3, "s3", b.Connector("designs"), store, opts...), b, store
}

func TestDriverContract(t *testing.T) {
	for _, versions := range []bool{true, false} {
		name := "unversioned"
		if versions {
			name = "versioned"
		}
		t.Run(name, func(t *testing.T) {
			mountstest.RunDriver(t, func(t *testing.T, files map[string]string) mountstest.DriverFixture {
				d, b, _ := newDriver(t, versions, files)
				return mountstest.DriverFixture{
					Driver:                d,
					Source:                "s3://designs/boards/jetson",
					Missing:               "s3://nope/boards",
					Change:                func(t *testing.T, p, data string) { b.Put("boards/jetson/"+p, []byte(data)) },
					SnapshotFailsOnChange: !versions,
				}
			})
		})
		t.Run(name+"/views", func(t *testing.T) {
			mountstest.RunDriver(t, func(t *testing.T, files map[string]string) mountstest.DriverFixture {
				views := t.TempDir()
				d, b, _ := newDriver(t, versions, files, bucket.WithViews(views))
				return mountstest.DriverFixture{
					Driver:    d,
					Source:    "s3://designs/boards/jetson",
					Missing:   "s3://nope/boards",
					Change:    func(t *testing.T, p, data string) { b.Put("boards/jetson/"+p, []byte(data)) },
					HostPaths: []mounts.Mode{mounts.ModeSnapshot},
					Views:     views,
					Offline:   func(*testing.T) { b.Offline() },
				}
			})
		})
	}
}

func TestParseSource(t *testing.T) {
	for in, want := range map[string]bucket.Source{
		"s3://designs":                {Bucket: "designs"},
		"s3://designs/":               {Bucket: "designs"},
		"s3://designs/boards/jetson/": {Bucket: "designs", Prefix: "boards/jetson"},
		"s3://designs/boards/jetson":  {Bucket: "designs", Prefix: "boards/jetson"},
	} {
		got, err := bucket.ParseSource("s3", in)
		if err != nil || got != want {
			t.Errorf("ParseSource(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"gs://designs", "s3://", "s3:///boards", "s3://designs/../x", "s3://designs/a//b"} {
		if _, err := bucket.ParseSource("s3", in); !errors.Is(err, mounts.ErrInvalidArgument) {
			t.Errorf("ParseSource(%q) = %v, want ErrInvalidArgument", in, err)
		}
	}
}

func TestEmptyPrefixIsNotFound(t *testing.T) {
	d, _, _ := newDriver(t, true, mountstest.DriverFiles)
	if _, err := d.Resolve(context.Background(), "s3://designs/boards/nope", "", nil); !errors.Is(err, mounts.ErrNotFound) {
		t.Fatalf("Resolve(empty prefix) = %v, want ErrNotFound", err)
	}
}

func TestVersionMovesWithAChange(t *testing.T) {
	ctx := context.Background()
	d, b, _ := newDriver(t, false, mountstest.DriverFiles)
	src := "s3://designs/boards/jetson"
	v1, err := d.Resolve(ctx, src, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := d.Resolve(ctx, src, "", nil); again != v1 {
		t.Fatalf("Resolve twice = %s, %s; want the same version with no change", v1, again)
	}
	b.Put("boards/jetson/README.md", []byte("changed"))
	if v2, _ := d.Resolve(ctx, src, "", nil); v2 == v1 {
		t.Fatalf("Resolve after a change = %s, want a new version", v2)
	}
}

func TestSnapshotThroughTheRegistry(t *testing.T) {
	ctx := context.Background()
	d, b, store := newDriver(t, true, mountstest.DriverFiles)
	recs, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(recs, []mounts.Driver{d})
	m, err := reg.Create(ctx, mounts.Mount{Name: "jetson", Kind: mounts.KindS3, Source: "s3://designs/boards/jetson", Mode: mounts.ModeSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	b.Put("boards/jetson/README.md", []byte("changed"))
	if got, err := fs.ReadFile(reg.FS(ctx), m.Name+"/README.md"); err != nil || string(got) != mountstest.DriverFiles["README.md"] {
		t.Fatalf("ReadFile = %q, %v; want the pinned bytes", got, err)
	}
	kept := map[blobs.Hash]bool{}
	if err := reg.BlobRefs(store)(ctx, func(h blobs.Hash) { kept[h] = true }); err != nil {
		t.Fatal(err)
	}
	if !kept[blobs.Hash(m.Version)] {
		t.Errorf("BlobRefs keeps %v, want the listing %s kept through a sweep", kept, m.Version)
	}

	files, err := reg.ListAt(ctx, m.Name, m.Version, "board")
	if err != nil || len(files) != 2 {
		t.Fatalf("ListAt(board) = %v, %v; want the two board files", files, err)
	}
	h, _, err := reg.FetchAt(ctx, m.Name, m.Version, files[0])
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := store.Has(ctx, h); !ok {
		t.Errorf("FetchAt returned %s, which the store doesn't hold", h)
	}
}

func TestBearerOnlyConnectorRefusesAWS(t *testing.T) {
	d := bucket.New(mounts.KindGCS, "gs", func(_ context.Context, _ string, c mounts.Credentials) (bucket.Bucket, error) {
		_, err := mounts.TokenSourceOf(c)
		return nil, err
	}, nil)
	if _, err := d.Resolve(context.Background(), "gs://designs", "", mounts.AWSCredentials{DefaultChain: true}); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Fatalf("Resolve = %v, want ErrInvalidArgument", err)
	}
}
