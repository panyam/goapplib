package s3_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/drivers/s3"
	"github.com/panyam/goapplib/storage/drivers/s3/s3blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

func TestDriverContractWithViews(t *testing.T) {
	for _, versioned := range []bool{true, false} {
		name := "unversioned"
		if versioned {
			name = "versioned"
		}
		t.Run(name, func(t *testing.T) {
			mountstest.RunDriver(t, func(t *testing.T, files map[string]string) mountstest.DriverFixture {
				url, c := fake(t, versioned)
				for p, data := range files {
					put(t, c, "boards/jetson/"+p, data)
				}
				views, net := t.TempDir(), &switchable{}
				return mountstest.DriverFixture{
					Driver:    newDriver(t, url, s3.WithViews(views), s3.WithHTTPClient(&http.Client{Transport: net})),
					Source:    "s3://designs/boards/jetson",
					Missing:   "s3://nope/boards",
					Creds:     creds,
					Change:    func(t *testing.T, p, data string) { put(t, c, "boards/jetson/"+p, data) },
					HostPaths: []mounts.Mode{mounts.ModeSnapshot},
					Views:     views,
					Offline:   func(*testing.T) { net.down.Store(true) },
				}
			})
		})
	}
}

// TestViewWithBlobsInABucket keeps a mount's blobs in S3 through blobs.Cached, then loses the
// local copies and the views on a restart, with the source bucket out of reach. The view comes back
// from the blob bucket.
func TestViewWithBlobsInABucket(t *testing.T) {
	ctx := context.Background()
	url, c := fake(t, false)
	for p, data := range mountstest.DriverFiles {
		put(t, c, "boards/jetson/"+p, data)
	}
	if _, err := c.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String("blobs")}); err != nil {
		t.Fatal(err)
	}
	remote, err := s3blobs.New(ctx, "s3://blobs/agni", creds, s3blobs.WithS3Options(s3.WithEndpoint(url)), s3blobs.WithTempDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	local, views := t.TempDir(), t.TempDir()
	start := func(net http.RoundTripper) *bucket.Driver {
		d, err := blobs.NewDir(local)
		if err != nil {
			t.Fatal(err)
		}
		return s3.New(blobs.NewCached(remote, d), s3.WithEndpoint(url), s3.WithViews(views), s3.WithHTTPClient(&http.Client{Transport: net}))
	}

	d := start(http.DefaultTransport)
	m := mounts.Mount{Name: "jetson", Kind: mounts.KindS3, Source: "s3://designs/boards/jetson", Mode: mounts.ModeSnapshot}
	if m.Version, err = d.Resolve(ctx, m.Source, "", creds); err != nil {
		t.Fatal(err)
	}
	if m, err = d.Prepare(ctx, m, creds); err != nil {
		t.Fatal(err)
	}
	hostFiles(t, d, m)

	for _, dir := range []string{local, views} {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	}
	down := &switchable{}
	down.down.Store(true)
	hostFiles(t, start(down), m)
}

func hostFiles(t *testing.T, d mounts.Localizer, m mounts.Mount) {
	t.Helper()
	for p, want := range mountstest.DriverFiles {
		hp, err := d.HostPath(m, p)
		if err != nil {
			t.Fatalf("HostPath(%q) = %v", p, err)
		}
		if got, err := os.ReadFile(hp); err != nil || string(got) != want {
			t.Errorf("the host file for %q read %q, %v; want %q", p, got, err, want)
		}
	}
}

// switchable fails every request once down is set, so a test can take the service away.
type switchable struct{ down atomic.Bool }

func (s *switchable) RoundTrip(r *http.Request) (*http.Response, error) {
	if s.down.Load() {
		return nil, errors.New("the service is offline")
	}
	return http.DefaultTransport.RoundTrip(r)
}
