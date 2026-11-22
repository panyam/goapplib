// Package gcsblobs is a blobs.Store in a Cloud Storage bucket, under "gs://<bucket>/<prefix>", so
// a server on Cloud Run or GKE keeps its blobs without a persistent disk. Each blob is the object
// <prefix>/sha256/ab/cdef..., the layout blobs.Dir uses on disk.
//
// Put spools its input to a local temp file to learn the hash, then uploads once with
// ifGenerationMatch=0, so two servers putting the same bytes never overwrite each other. A blob
// that is there already gets its written time moved forward in its metadata, so Sweep's grace
// period starts again as Store.Put promises.
//
// A bucket can't hard-link, so Store isn't a blobs.Linker. An app that needs blobs as files keeps
// a local cache in front of it.
package gcsblobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"github.com/panyam/goapplib/storage/blobs"
	gcsdrv "github.com/panyam/goapplib/storage/drivers/gcs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

// writtenKey is the object metadata that records when a blob was last put, in Unix seconds. Sweep
// ages a blob by it, since a refresh can't move an object's creation time.
const writtenKey = "goapplib-written"

// Store keeps blobs in one bucket prefix. It is safe for concurrent use, and several servers may
// share one prefix.
type Store struct {
	b      *storage.BucketHandle
	name   string
	prefix string // "" or ending in "/"
	tmp    string
	now    func() time.Time
}

type options struct {
	gcs []gcsdrv.Option
	tmp string
	now func() time.Time
}

// Option configures a Store.
type Option func(*options)

// WithGCSOptions passes options to the Cloud Storage client, such as gcs.WithEndpoint for a test's
// fake.
func WithGCSOptions(opts ...gcsdrv.Option) Option {
	return func(o *options) { o.gcs = append(o.gcs, opts...) }
}

// WithTempDir sets where Put spools its input. The default is os.TempDir(). It needs room for the
// largest blob a caller puts.
func WithTempDir(dir string) Option { return func(o *options) { o.tmp = dir } }

// WithClock sets the clock Put records and Sweep ages blobs by. The default is time.Now.
func WithClock(now func() time.Time) Option { return func(o *options) { o.now = now } }

// New returns a Store over source, "gs://<bucket>/<prefix>", sending creds' bearer token, which a
// connections KindGCP connection gives. It makes no request, so a missing bucket shows up on the
// first call.
func New(ctx context.Context, source string, creds mounts.Credentials, opts ...Option) (*Store, error) {
	src, err := bucket.ParseSource("gs", source)
	if err != nil {
		return nil, err
	}
	o := options{now: time.Now}
	for _, f := range opts {
		f(&o)
	}
	c, err := gcsdrv.NewClient(ctx, creds, o.gcs...)
	if err != nil {
		return nil, err
	}
	s := &Store{b: c.Bucket(src.Bucket), name: src.Bucket, tmp: o.tmp, now: o.now}
	if src.Prefix != "" {
		s.prefix = src.Prefix + "/"
	}
	return s, nil
}

func (s *Store) key(h blobs.Hash) (string, error) {
	k, err := blobs.Key(h)
	if err != nil {
		return "", err
	}
	return s.prefix + k, nil
}

func (s *Store) written() map[string]string {
	return map[string]string{writtenKey: strconv.FormatInt(s.now().Unix(), 10)}
}

// Put implements blobs.Store.
func (s *Store) Put(ctx context.Context, r io.Reader) (blobs.Hash, error) {
	f, h, _, err := blobs.Spool(r, s.tmp)
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	k, _ := s.key(h)
	obj := s.b.Object(k)
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := obj.If(storage.Conditions{DoesNotExist: true}).NewWriter(wctx)
	w.Metadata = s.written()
	if _, err := io.Copy(w, f); err != nil {
		cancel()
		w.Close()
		return "", s.err(k, err)
	}
	err = w.Close()
	if err == nil {
		return h, nil
	}
	if !exists(err) {
		return "", s.err(k, err)
	}
	if _, err := obj.Update(ctx, storage.ObjectAttrsToUpdate{Metadata: s.written()}); err != nil {
		return "", s.err(k, err)
	}
	return h, nil
}

// Open implements blobs.Store.
func (s *Store) Open(ctx context.Context, h blobs.Hash) (io.ReadCloser, error) {
	k, err := s.key(h)
	if err != nil {
		return nil, err
	}
	rc, err := s.b.Object(k).NewReader(ctx)
	if err != nil {
		return nil, s.err(k, err)
	}
	return blobs.Verify(rc, h), nil
}

// Has implements blobs.Store.
func (s *Store) Has(ctx context.Context, h blobs.Hash) (bool, error) {
	k, err := s.key(h)
	if err != nil {
		return false, err
	}
	_, err = s.b.Object(k).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if err = s.err(k, err); errors.Is(err, blobs.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// Sweep implements blobs.Store. One listing of the prefix carries each object's metadata, so it
// makes no request per blob except the deletes. Objects under the prefix that aren't blobs are
// left alone.
func (s *Store) Sweep(ctx context.Context, grace time.Duration, refs ...blobs.Referrer) (int, error) {
	keep := map[blobs.Hash]bool{}
	for _, ref := range refs {
		if err := ref(ctx, func(h blobs.Hash) { keep[h] = true }); err != nil {
			return 0, err
		}
	}
	cutoff := s.now().Add(-grace)
	removed := 0
	it := s.b.Objects(ctx, &storage.Query{Prefix: s.prefix + "sha256/"})
	for {
		a, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return removed, s.err(s.prefix, err)
		}
		h, ok := blobs.HashOfKey(a.Name[len(s.prefix):])
		if !ok || keep[h] || writtenAt(a).After(cutoff) {
			continue
		}
		if err := s.b.Object(a.Name).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
			return removed, s.err(a.Name, err)
		}
		removed++
	}
	return removed, nil
}

// writtenAt is when a blob was last put: its metadata where a Store wrote it, else the object's
// creation time, for a blob copied in some other way.
func writtenAt(a *storage.ObjectAttrs) time.Time {
	if sec, err := strconv.ParseInt(a.Metadata[writtenKey], 10, 64); err == nil {
		return time.Unix(sec, 0)
	}
	return a.Created
}

// exists reports whether a conditional write failed because the object is there: 412 on the
// precondition, or 409 when another write of the same name is in flight.
func exists(err error) bool {
	var ge *googleapi.Error
	return errors.As(err, &ge) && (ge.Code == http.StatusPreconditionFailed || ge.Code == http.StatusConflict)
}

// err maps a Cloud Storage error onto the blobs and mounts sentinels: a missing object is
// blobs.ErrNotFound, a missing bucket mounts.ErrNotFound, a refusal mounts.ErrPermissionDenied,
// and anything else mounts.ErrUnavailable.
func (s *Store) err(key string, err error) error {
	where := "gs://" + s.name + "/" + key
	var ge *googleapi.Error
	switch {
	case errors.Is(err, storage.ErrBucketNotExist):
		return fmt.Errorf("%w: %s: no such bucket", mounts.ErrNotFound, where)
	case errors.Is(err, storage.ErrObjectNotExist):
		return fmt.Errorf("%w: %s", blobs.ErrNotFound, where)
	case errors.As(err, &ge) && (ge.Code == http.StatusForbidden || ge.Code == http.StatusUnauthorized):
		return fmt.Errorf("%w: %s: %v", mounts.ErrPermissionDenied, where, err)
	case errors.As(err, &ge) && ge.Code == http.StatusNotFound:
		return fmt.Errorf("%w: %s", blobs.ErrNotFound, where)
	}
	return fmt.Errorf("%w: %s: %v", mounts.ErrUnavailable, where, err)
}

var _ blobs.Store = (*Store)(nil)
