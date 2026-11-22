// Package gcs serves mounts over prefixes in Google Cloud Storage buckets,
// "gs://<bucket>/<prefix>", through cloud.google.com/go/storage. It is its own module so an app
// that doesn't mount GCS never downloads the SDK. The listing, pinning and fs.FS logic is in
// storage/mounts/bucket, and this package adapts GCS to its Bucket interface.
//
// Every GCS object has a generation, whether or not the bucket keeps old versions, so a snapshot
// always pins generations. On a bucket with versioning on, a snapshot keeps serving the old
// generation after the object changes. With it off, GCS drops the old generation, and reading it
// fails with mounts.ErrConflict and the path named.
//
// The driver signs with mounts.BearerCredentials, which a connections KindGCP connection gives.
// Nil credentials read a public bucket without signing. Any other kind returns
// mounts.ErrInvalidArgument.
package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type options struct {
	endpoint string
	client   *http.Client
	limits   mounts.Limits
}

// Option configures the driver.
type Option func(*options)

// WithEndpoint sends JSON API requests to url, such as a test's fake at
// "http://127.0.0.1:4443/storage/v1/", in place of Google's.
func WithEndpoint(url string) Option { return func(o *options) { o.endpoint = url } }

// WithHTTPClient sets the base HTTP client. The driver adds the bearer token on top of its
// transport.
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.client = c } }

// WithLimits caps a source's objects and bytes, as bucket.WithLimits does.
func WithLimits(l mounts.Limits) Option { return func(o *options) { o.limits = l } }

// New returns a driver for mounts.KindGCS. store keeps each Resolve's listing, as bucket.New
// describes.
func New(store blobs.Store, opts ...Option) *bucket.Driver {
	var o options
	for _, f := range opts {
		f(&o)
	}
	return bucket.New(mounts.KindGCS, "gs", o.connect, store, bucket.WithLimits(o.limits))
}

func (o options) connect(ctx context.Context, name string, creds mounts.Credentials) (bucket.Bucket, error) {
	ts, err := mounts.TokenSourceOf(creds)
	if err != nil {
		return nil, err
	}
	base := http.DefaultTransport
	if o.client != nil && o.client.Transport != nil {
		base = o.client.Transport
	}
	hc := &http.Client{Transport: base}
	if ts != nil {
		hc.Transport = &oauth2.Transport{Source: ts, Base: base}
	}
	opts := []option.ClientOption{option.WithHTTPClient(hc)}
	if o.endpoint != "" {
		opts = append(opts, option.WithEndpoint(o.endpoint))
	}
	c, err := storage.NewClient(ctx, append(opts, storage.WithJSONReads())...)
	if err != nil {
		return nil, fmt.Errorf("%w: gs://%s: %v", mounts.ErrUnavailable, name, err)
	}
	return &gcsBucket{b: c.Bucket(name), name: name}, nil
}

type gcsBucket struct {
	b    *storage.BucketHandle
	name string
}

func object(a *storage.ObjectAttrs) bucket.Object {
	return bucket.Object{Key: a.Name, Version: strconv.FormatInt(a.Generation, 10), ETag: a.Etag, Size: a.Size}
}

// List lists every live object under prefix with its generation.
func (g *gcsBucket) List(ctx context.Context, prefix string) ([]bucket.Object, error) {
	var out []bucket.Object
	it := g.b.Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		a, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, g.err(prefix, err, false)
		}
		out = append(out, object(a))
	}
}

// ListDir lists one folder level with "/" as the delimiter.
func (g *gcsBucket) ListDir(ctx context.Context, dir string) ([]bucket.Object, []string, error) {
	var files []bucket.Object
	var folders []string
	it := g.b.Objects(ctx, &storage.Query{Prefix: dir, Delimiter: "/"})
	for {
		a, err := it.Next()
		if err == iterator.Done {
			return files, folders, nil
		}
		if err != nil {
			return nil, nil, g.err(dir, err, false)
		}
		if a.Prefix != "" {
			folders = append(folders, strings.TrimSuffix(strings.TrimPrefix(a.Prefix, dir), "/"))
			continue
		}
		files = append(files, object(a))
	}
}

// Get reads key, at pin's generation when pin is set.
func (g *gcsBucket) Get(ctx context.Context, key string, pin *bucket.Object) (io.ReadCloser, int64, error) {
	obj := g.b.Object(key)
	if pin != nil {
		gen, err := strconv.ParseInt(pin.Version, 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: gs://%s/%s: generation %q", mounts.ErrInvalidArgument, g.name, key, pin.Version)
		}
		obj = obj.Generation(gen)
	}
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, 0, g.err(key, err, pin != nil)
	}
	return r, r.Attrs.Size, nil
}

// err maps a GCS error onto the mounts sentinels. With pinned set, a missing object means the
// pinned generation is gone, which is mounts.ErrConflict.
func (g *gcsBucket) err(what string, err error, pinned bool) error {
	where := "gs://" + g.name + "/" + what
	code := 0
	var ge *googleapi.Error
	if errors.As(err, &ge) {
		code = ge.Code
	}
	switch {
	case errors.Is(err, storage.ErrBucketNotExist):
		return fmt.Errorf("%w: %s: no such bucket", mounts.ErrNotFound, where)
	case errors.Is(err, storage.ErrObjectNotExist) || code == http.StatusNotFound:
		if pinned {
			return fmt.Errorf("%w: %s: the snapshot's generation is gone", mounts.ErrConflict, where)
		}
		return fmt.Errorf("%w: %s", mounts.ErrNotFound, where)
	case code == http.StatusForbidden || code == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s: %v", mounts.ErrPermissionDenied, where, err)
	}
	return fmt.Errorf("%w: %s: %v", mounts.ErrUnavailable, where, err)
}
