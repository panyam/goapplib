package gcsblobs_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/blobs/blobstest"
	gcsdrv "github.com/panyam/goapplib/storage/drivers/gcs"
	"github.com/panyam/goapplib/storage/drivers/gcs/gcsblobs"
	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var creds = mounts.Bearer(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "ya29.fake"}))

func fake(t *testing.T) *fakestorage.Server {
	t.Helper()
	srv, err := fakestorage.NewServerWithOptions(fakestorage.Options{Scheme: "http", Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	srv.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: "blobs"})
	return srv
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func newStore(t *testing.T, srv *fakestorage.Server, source string, clk *clock) *gcsblobs.Store {
	t.Helper()
	s, err := gcsblobs.New(context.Background(), source, creds,
		gcsblobs.WithGCSOptions(gcsdrv.WithEndpoint(srv.URL()+"/storage/v1/")), gcsblobs.WithTempDir(t.TempDir()), gcsblobs.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStoreKeepsTheStoreContract(t *testing.T) {
	for _, source := range []string{"gs://blobs/agni/uploads", "gs://blobs"} {
		t.Run(source, func(t *testing.T) {
			blobstest.RunStore(t, func(t *testing.T) blobstest.Fixture {
				srv := fake(t)
				clk := &clock{t: time.Now()}
				prefix := strings.TrimPrefix(strings.TrimPrefix(source, "gs://blobs"), "/")
				if prefix != "" {
					prefix += "/"
				}
				return blobstest.Fixture{
					Store: newStore(t, srv, source, clk),
					Later: func(t *testing.T, by time.Duration) { clk.add(by) },
					Corrupt: func(t *testing.T, h blobs.Hash, b []byte) {
						k, _ := blobs.Key(h)
						srv.CreateObject(fakestorage.Object{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "blobs", Name: prefix + k}, Content: b})
					},
				}
			})
		})
	}
}

func TestBlobsUseDirsLayoutUnderThePrefix(t *testing.T) {
	srv := fake(t)
	s := newStore(t, srv, "gs://blobs/agni/uploads", &clock{t: time.Now()})
	h, err := s.Put(context.Background(), strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	k, _ := blobs.Key(h)
	if _, err := srv.GetObject("blobs", "agni/uploads/"+k); err != nil {
		t.Errorf("no object at agni/uploads/%s: %v", k, err)
	}
}

func TestSweepLeavesObjectsThatArentBlobs(t *testing.T) {
	srv := fake(t)
	clk := &clock{t: time.Now()}
	s := newStore(t, srv, "gs://blobs/agni", clk)
	srv.CreateObject(fakestorage.Object{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "blobs", Name: "agni/sha256/README.txt"}, Content: []byte("notes")})
	clk.add(time.Hour)
	if n, err := s.Sweep(context.Background(), time.Minute); err != nil || n != 0 {
		t.Fatalf("Sweep = %d, %v; want 0", n, err)
	}
	if _, err := srv.GetObject("blobs", "agni/sha256/README.txt"); err != nil {
		t.Errorf("Sweep removed an object that isn't a blob: %v", err)
	}
}

func TestAWSCredentialsAreRefused(t *testing.T) {
	_, err := gcsblobs.New(context.Background(), "gs://blobs", mounts.AWSCredentials{DefaultChain: true})
	if !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Fatalf("New with AWS credentials = %v, want ErrInvalidArgument", err)
	}
}

// TestLiveGCS puts, reads and sweeps one blob in a real bucket. It skips unless STORAGE_LIVE_GCS is
// "gs://<bucket>/<prefix>", and uses Application Default Credentials.
func TestLiveGCS(t *testing.T) {
	source := os.Getenv("STORAGE_LIVE_GCS")
	if source == "" {
		t.Skip("set STORAGE_LIVE_GCS=gs://<bucket>/<prefix> to run against Cloud Storage")
	}
	ctx := context.Background()
	ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/devstorage.read_write")
	if err != nil {
		t.Fatal(err)
	}
	s, err := gcsblobs.New(ctx, source+"/gcsblobs-live", mounts.Bearer(ts))
	if err != nil {
		t.Fatal(err)
	}
	data := "goapplib gcsblobs live test " + time.Now().Format(time.RFC3339Nano)
	h, err := s.Put(ctx, strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, strings.NewReader(data)); err != nil {
		t.Fatalf("a second Put of the same bytes: %v", err)
	}
	if ok, err := s.Has(ctx, h); !ok || err != nil {
		t.Fatalf("Has = %v, %v", ok, err)
	}
	if n, err := s.Sweep(ctx, 0); err != nil || n < 1 {
		t.Fatalf("Sweep with no referrers = %d, %v; want the test blob removed", n, err)
	}
}
