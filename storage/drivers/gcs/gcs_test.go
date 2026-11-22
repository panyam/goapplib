package gcs_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/drivers/gcs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
	"golang.org/x/oauth2"
)

var creds = mounts.Bearer(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "ya29.fake"}))

// fake starts fake-gcs-server with one bucket, "designs".
func fake(t *testing.T, versioned bool) *fakestorage.Server {
	t.Helper()
	srv, err := fakestorage.NewServerWithOptions(fakestorage.Options{Scheme: "http", Host: "127.0.0.1", Port: 0, NoListener: false})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	srv.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: "designs", VersioningEnabled: versioned})
	return srv
}

func put(srv *fakestorage.Server, key, data string) {
	srv.CreateObject(fakestorage.Object{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "designs", Name: key}, Content: []byte(data)})
}

func newDriver(t *testing.T, srv *fakestorage.Server) *bucket.Driver {
	t.Helper()
	store, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return gcs.New(store, gcs.WithEndpoint(srv.URL()+"/storage/v1/"))
}

func TestDriverContract(t *testing.T) {
	for _, versioned := range []bool{true, false} {
		name := "unversioned"
		if versioned {
			name = "versioned"
		}
		t.Run(name, func(t *testing.T) {
			mountstest.RunDriver(t, func(t *testing.T, files map[string]string) mountstest.DriverFixture {
				srv := fake(t, versioned)
				for p, data := range files {
					put(srv, "boards/jetson/"+p, data)
				}
				return mountstest.DriverFixture{
					Driver:                newDriver(t, srv),
					Source:                "gs://designs/boards/jetson",
					Missing:               "gs://nope/boards",
					Creds:                 creds,
					Change:                func(t *testing.T, p, data string) { put(srv, "boards/jetson/"+p, data) },
					SnapshotFailsOnChange: !versioned,
				}
			})
		})
	}
}

func TestAWSCredentialsAreRefused(t *testing.T) {
	d := newDriver(t, fake(t, false))
	_, err := d.Resolve(context.Background(), "gs://designs", "", mounts.AWSCredentials{DefaultChain: true})
	if !errors.Is(err, mounts.ErrInvalidArgument) || !strings.Contains(err.Error(), "aws") {
		t.Fatalf("Resolve with AWS credentials = %v, want ErrInvalidArgument naming aws", err)
	}
}

func TestPublicBucketNeedsNoCredentials(t *testing.T) {
	srv := fake(t, false)
	put(srv, "README.md", "hello")
	m := mounts.Mount{Name: "pub", Kind: mounts.KindGCS, Source: "gs://designs", Mode: mounts.ModeLive}
	fsys, err := newDriver(t, srv).Open(context.Background(), m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := fs.ReadFile(fsys, "README.md"); err != nil || string(got) != "hello" {
		t.Fatalf("ReadFile = %q, %v; want hello", got, err)
	}
}

// TestLiveGCS reads a real bucket with STORAGE_LIVE_GCS set to a source such as
// gs://bucket/prefix, signing with Application Default Credentials.
func TestLiveGCS(t *testing.T) {
	source := os.Getenv("STORAGE_LIVE_GCS")
	if source == "" {
		t.Skip("set STORAGE_LIVE_GCS=gs://bucket/prefix to read a real bucket")
	}
	ctx := context.Background()
	ts, err := googleDefault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := gcs.New(store)
	c := mounts.Bearer(ts)
	m := mounts.Mount{Name: "live", Kind: mounts.KindGCS, Source: source, Mode: mounts.ModeSnapshot}
	if m.Version, err = d.Resolve(ctx, source, "", c); err != nil {
		t.Fatal(err)
	}
	if m, err = d.Prepare(ctx, m, c); err != nil {
		t.Fatal(err)
	}
	fsys, err := d.Open(ctx, m, c)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	err = fs.WalkDir(fsys, ".", func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			n++
			_, err = fs.ReadFile(fsys, p)
		}
		return err
	})
	if err != nil || n == 0 {
		t.Fatalf("walked %d files, %v; want at least one, read without error", n, err)
	}
}
