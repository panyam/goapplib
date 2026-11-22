package s3blobs_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/blobs/blobstest"
	s3drv "github.com/panyam/goapplib/storage/drivers/s3"
	"github.com/panyam/goapplib/storage/drivers/s3/s3blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

var creds = mounts.AWSCredentials{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", Region: "us-east-1"}

// fake starts gofakes3 with one bucket, "blobs", and returns its URL, a client for reaching
// behind the store, and a count of If-None-Match refusals.
func fake(t *testing.T) (string, *awss3.Client, *int) {
	t.Helper()
	refused := new(int)
	srv := httptest.NewServer(ifNoneMatch(gofakes3.New(s3mem.New()).Server(), refused))
	t.Cleanup(srv.Close)
	c := awss3.New(awss3.Options{
		Region: "us-east-1", BaseEndpoint: aws.String(srv.URL), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "secret", ""),
	})
	if _, err := c.CreateBucket(context.Background(), &awss3.CreateBucketInput{Bucket: aws.String("blobs")}); err != nil {
		t.Fatal(err)
	}
	return srv.URL, c, refused
}

// ifNoneMatch answers a PUT with "If-None-Match: *" for a key that exists with S3's 412, since
// gofakes3 ignores the header. S3 has honoured it since August 2024.
func ifNoneMatch(next http.Handler, refused *int) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.Header.Get("If-None-Match") == "*" && r.Header.Get("X-Amz-Copy-Source") == "" {
			mu.Lock()
			defer mu.Unlock()
			head := httptest.NewRecorder()
			hr := r.Clone(r.Context())
			hr.Method, hr.Body = http.MethodHead, http.NoBody
			next.ServeHTTP(head, hr)
			if head.Code == http.StatusOK {
				*refused++
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusPreconditionFailed)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>PreconditionFailed</Code><Message>At least one of the pre-conditions you specified did not hold</Message></Error>`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func newStore(t *testing.T, url, source string, clk *clock) *s3blobs.Store {
	t.Helper()
	s, err := s3blobs.New(context.Background(), source, creds,
		s3blobs.WithS3Options(s3drv.WithEndpoint(url)), s3blobs.WithTempDir(t.TempDir()), s3blobs.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStoreKeepsTheStoreContract(t *testing.T) {
	for _, source := range []string{"s3://blobs/agni/uploads", "s3://blobs"} {
		t.Run(source, func(t *testing.T) {
			blobstest.RunStore(t, func(t *testing.T) blobstest.Fixture {
				url, c, _ := fake(t)
				clk := &clock{t: time.Now()}
				s := newStore(t, url, source, clk)
				prefix := strings.TrimPrefix(strings.TrimPrefix(source, "s3://blobs"), "/")
				if prefix != "" {
					prefix += "/"
				}
				return blobstest.Fixture{
					Store: s,
					Later: func(t *testing.T, by time.Duration) { clk.add(by) },
					Corrupt: func(t *testing.T, h blobs.Hash, b []byte) {
						k, _ := blobs.Key(h)
						if _, err := c.PutObject(context.Background(), &awss3.PutObjectInput{Bucket: aws.String("blobs"), Key: aws.String(prefix + k), Body: bytes.NewReader(b)}); err != nil {
							t.Fatal(err)
						}
					},
				}
			})
		})
	}
}

func TestBlobsUseDirsLayoutUnderThePrefix(t *testing.T) {
	url, c, refused := fake(t)
	s := newStore(t, url, "s3://blobs/agni/uploads", &clock{t: time.Now()})
	h, err := s.Put(context.Background(), strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	k, _ := blobs.Key(h)
	if _, err := c.HeadObject(context.Background(), &awss3.HeadObjectInput{Bucket: aws.String("blobs"), Key: aws.String("agni/uploads/" + k)}); err != nil {
		t.Errorf("no object at agni/uploads/%s: %v", k, err)
	}
	if _, err := s.Put(context.Background(), strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	if *refused != 1 {
		t.Errorf("the second Put of the same bytes met %d If-None-Match refusals, want 1, so it never overwrote the object", *refused)
	}
}

func TestSweepLeavesObjectsThatArentBlobs(t *testing.T) {
	url, c, _ := fake(t)
	clk := &clock{t: time.Now()}
	s := newStore(t, url, "s3://blobs/agni", clk)
	stray := "agni/sha256/README.txt"
	if _, err := c.PutObject(context.Background(), &awss3.PutObjectInput{Bucket: aws.String("blobs"), Key: aws.String(stray), Body: strings.NewReader("notes")}); err != nil {
		t.Fatal(err)
	}
	clk.add(time.Hour)
	if n, err := s.Sweep(context.Background(), time.Minute); err != nil || n != 0 {
		t.Fatalf("Sweep = %d, %v; want 0", n, err)
	}
	if _, err := c.HeadObject(context.Background(), &awss3.HeadObjectInput{Bucket: aws.String("blobs"), Key: aws.String(stray)}); err != nil {
		t.Errorf("Sweep removed an object that isn't a blob: %v", err)
	}
}

func TestBearerCredentialsAreRefused(t *testing.T) {
	_, err := s3blobs.New(context.Background(), "s3://blobs", mounts.Bearer(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"})))
	if !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Fatal("New with bearer credentials should fail, since S3 signs with SigV4")
	}
}

// TestLiveS3 puts, reads and sweeps one blob in a real bucket. It skips unless STORAGE_LIVE_S3 is
// "s3://<bucket>/<prefix>", and signs with the default credential chain.
func TestLiveS3(t *testing.T) {
	source := os.Getenv("STORAGE_LIVE_S3")
	if source == "" {
		t.Skip("set STORAGE_LIVE_S3=s3://<bucket>/<prefix> to run against S3")
	}
	ctx := context.Background()
	s, err := s3blobs.New(ctx, source+"/s3blobs-live", mounts.AWSCredentials{DefaultChain: true, Region: os.Getenv("AWS_REGION")})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("goapplib s3blobs live test " + time.Now().Format(time.RFC3339Nano))
	h, err := s.Put(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, bytes.NewReader(data)); err != nil {
		t.Fatalf("a second Put of the same bytes: %v", err)
	}
	if ok, err := s.Has(ctx, h); !ok || err != nil {
		t.Fatalf("Has = %v, %v", ok, err)
	}
	if n, err := s.Sweep(ctx, 0); err != nil || n < 1 {
		t.Fatalf("Sweep with no referrers = %d, %v; want the test blob removed", n, err)
	}
}
