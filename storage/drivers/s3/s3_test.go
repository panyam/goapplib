package s3_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/drivers/s3"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

var creds = mounts.AWSCredentials{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", Region: "us-east-1"}

// fake starts gofakes3 with one bucket, "designs", and returns its URL and a client for seeding it.
func fake(t *testing.T, versioned bool) (string, *awss3.Client) {
	t.Helper()
	return fakeWith(t, versioned, nil)
}

// fakeWith is fake with wrap around the server, such as one that refuses a call.
func fakeWith(t *testing.T, versioned bool, wrap func(http.Handler) http.Handler) (string, *awss3.Client) {
	t.Helper()
	h := ifMatch(gofakes3.New(s3mem.New()).Server())
	if wrap != nil {
		h = wrap(h)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := awss3.New(awss3.Options{
		Region: "us-east-1", BaseEndpoint: aws.String(srv.URL), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "secret", ""),
	})
	ctx := context.Background()
	if _, err := c.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String("designs")}); err != nil {
		t.Fatal(err)
	}
	if versioned {
		if _, err := c.PutBucketVersioning(ctx, &awss3.PutBucketVersioningInput{Bucket: aws.String("designs"),
			VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}}); err != nil {
			t.Fatal(err)
		}
	}
	return srv.URL, c
}

// ifMatch answers a GET whose If-Match names another ETag with S3's 412, since gofakes3 checks
// If-Match only on a PUT. S3 itself checks it on a GET.
func ifMatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := r.Header.Get("If-Match")
		if r.Method == http.MethodGet && want != "" {
			head := httptest.NewRecorder()
			hr := r.Clone(r.Context())
			hr.Method = http.MethodHead
			hr.Header.Del("If-Match")
			next.ServeHTTP(head, hr)
			if head.Code == http.StatusOK && head.Header().Get("ETag") != want {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusPreconditionFailed)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>PreconditionFailed</Code><Message>At least one of the pre-conditions you specified did not hold</Message></Error>`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func put(t *testing.T, c *awss3.Client, key, data string) {
	t.Helper()
	if _, err := c.PutObject(context.Background(), &awss3.PutObjectInput{Bucket: aws.String("designs"), Key: aws.String(key), Body: bytes.NewReader([]byte(data))}); err != nil {
		t.Fatal(err)
	}
}

func newDriver(t *testing.T, url string, opts ...s3.Option) *bucket.Driver {
	t.Helper()
	store, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s3.New(store, append([]s3.Option{s3.WithEndpoint(url)}, opts...)...)
}

func TestDriverContract(t *testing.T) {
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
				return mountstest.DriverFixture{
					Driver:                newDriver(t, url),
					Source:                "s3://designs/boards/jetson",
					Missing:               "s3://nope/boards",
					Creds:                 creds,
					Change:                func(t *testing.T, p, data string) { put(t, c, "boards/jetson/"+p, data) },
					SnapshotFailsOnChange: !versioned,
				}
			})
		})
	}
}

func TestPublicBucketNeedsNoCredentials(t *testing.T) {
	url, c := fake(t, false)
	put(t, c, "README.md", "hello")
	d := newDriver(t, url)
	m := mounts.Mount{Name: "pub", Kind: mounts.KindS3, Source: "s3://designs", Mode: mounts.ModeLive}
	fsys, err := d.Open(context.Background(), m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := fs.ReadFile(fsys, "README.md"); err != nil || string(got) != "hello" {
		t.Fatalf("ReadFile = %q, %v; want hello", got, err)
	}
}

func TestBearerCredentialsAreRefused(t *testing.T) {
	url, _ := fake(t, false)
	d := newDriver(t, url)
	_, err := d.Resolve(context.Background(), "s3://designs", "", mounts.Bearer(nil))
	if err != nil {
		t.Fatalf("Bearer(nil) is nil credentials, so Resolve should work: %v", err)
	}
	_, err = d.Resolve(context.Background(), "s3://designs", "", mounts.BearerCredentials{})
	if !errors.Is(err, mounts.ErrInvalidArgument) || !strings.Contains(err.Error(), "bearer") {
		t.Fatalf("Resolve with bearer credentials = %v, want ErrInvalidArgument naming bearer", err)
	}
}

// TestLiveS3 reads a real bucket with STORAGE_LIVE_S3 set to a source such as
// s3://bucket/prefix, signing with the AWS default credential chain.
func TestLiveS3(t *testing.T) {
	source := os.Getenv("STORAGE_LIVE_S3")
	if source == "" {
		t.Skip("set STORAGE_LIVE_S3=s3://bucket/prefix to read a real bucket")
	}
	ctx := context.Background()
	store, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := s3.New(store)
	c := mounts.AWSCredentials{DefaultChain: true, Region: os.Getenv("AWS_REGION")}
	m := mounts.Mount{Name: "live", Kind: mounts.KindS3, Source: source, Mode: mounts.ModeSnapshot}
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

// TestListingFallsBackToETagsWithoutListVersions checks a reader role with no
// s3:ListBucketVersions. S3 refuses ListObjectVersions with 403, so the driver lists with
// ListObjectsV2 and pins ETags. On a versioned bucket a snapshot then fails once the object
// changes, where a version pin would have served the old bytes.
func TestListingFallsBackToETagsWithoutListVersions(t *testing.T) {
	ctx := context.Background()
	noVersions := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := r.URL.Query()["versions"]; ok {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	url, c := fakeWith(t, true, noVersions)
	put(t, c, "boards/jetson/README.md", "rev 1")
	d := newDriver(t, url)
	src := "s3://designs/boards/jetson"
	v, err := d.Resolve(ctx, src, "", creds)
	if err != nil {
		t.Fatalf("Resolve with ListObjectVersions refused = %v, want the ListObjectsV2 fallback", err)
	}
	m, err := d.Prepare(ctx, mounts.Mount{Name: "jetson", Kind: mounts.KindS3, Source: src, Version: v, Mode: mounts.ModeSnapshot}, creds)
	if err != nil {
		t.Fatal(err)
	}
	fsys, err := d.Open(ctx, m, creds)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := fs.ReadFile(fsys, "README.md"); err != nil || string(b) != "rev 1" {
		t.Fatalf("ReadFile before the change = %q, %v", b, err)
	}
	put(t, c, "boards/jetson/README.md", "rev 2")
	if b, err := fs.ReadFile(fsys, "README.md"); !errors.Is(err, mounts.ErrConflict) {
		t.Errorf("ReadFile after the change = %q, %v; want ErrConflict from the ETag pin", b, err)
	}
}
