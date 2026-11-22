// Package s3blobs is a blobs.Store in an S3 bucket, under "s3://<bucket>/<prefix>", so a server
// on Kubernetes or another host without a persistent disk keeps its blobs in S3. Each blob is the
// object <prefix>/sha256/ab/cdef..., the layout blobs.Dir uses on disk.
//
// Put spools its input to a local temp file to learn the hash, then uploads once with
// If-None-Match: *, so two servers putting the same bytes never overwrite each other. A blob that
// is there already gets its written time moved forward, by copying the object onto itself with new
// metadata, so Sweep's grace period starts again as Store.Put promises.
//
// A bucket can't hard-link, so Store isn't a blobs.Linker. An app that needs blobs as files keeps
// a local cache in front of it.
package s3blobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/panyam/goapplib/storage/blobs"
	s3drv "github.com/panyam/goapplib/storage/drivers/s3"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
)

// writtenKey is the object metadata that records when a blob was last put, in Unix seconds. Sweep
// ages a blob by it, since a refresh can't move an object's creation time.
const writtenKey = "goapplib-written"

// Store keeps blobs in one bucket prefix. It is safe for concurrent use, and several servers may
// share one prefix.
type Store struct {
	c      *s3.Client
	bucket string
	prefix string // "" or ending in "/"
	tmp    string
	now    func() time.Time
}

type options struct {
	s3  []s3drv.Option
	tmp string
	now func() time.Time
}

// Option configures a Store.
type Option func(*options)

// WithS3Options passes options to the S3 client, such as s3.WithEndpoint for an S3-compatible
// store or a test's fake.
func WithS3Options(opts ...s3drv.Option) Option {
	return func(o *options) { o.s3 = append(o.s3, opts...) }
}

// WithTempDir sets where Put spools its input. The default is os.TempDir(). It needs room for the
// largest blob a caller puts.
func WithTempDir(dir string) Option { return func(o *options) { o.tmp = dir } }

// WithClock sets the clock Put records and Sweep ages blobs by. The default is time.Now.
func WithClock(now func() time.Time) Option { return func(o *options) { o.now = now } }

// New returns a Store over source, "s3://<bucket>/<prefix>", signed with creds, which a
// connections KindAWS connection gives. Nil creds sends unsigned requests, which only a public
// bucket with public writes would accept. It makes no request, so a missing bucket shows up on
// the first call.
func New(ctx context.Context, source string, creds mounts.Credentials, opts ...Option) (*Store, error) {
	src, err := bucket.ParseSource("s3", source)
	if err != nil {
		return nil, err
	}
	o := options{now: time.Now}
	for _, f := range opts {
		f(&o)
	}
	c, err := s3drv.NewClient(ctx, creds, o.s3...)
	if err != nil {
		return nil, err
	}
	s := &Store{c: c, bucket: src.Bucket, tmp: o.tmp, now: o.now}
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
	f, h, n, err := blobs.Spool(r, s.tmp)
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	k, _ := s.key(h)
	_, err = s.c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.bucket, Key: &k, Body: f, ContentLength: aws.Int64(n),
		IfNoneMatch: aws.String("*"), Metadata: s.written(),
	})
	if err == nil {
		return h, nil
	}
	if !exists(err) {
		return "", s.err(k, err)
	}
	// Copying the object onto itself with new metadata is S3's only way to move its time.
	_, err = s.c.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket: &s.bucket, Key: &k, CopySource: aws.String(s.bucket + "/" + k),
		MetadataDirective: types.MetadataDirectiveReplace, Metadata: s.written(),
	})
	if err != nil {
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
	out, err := s.c.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &k})
	if err != nil {
		return nil, s.err(k, err)
	}
	return blobs.Verify(out.Body, h), nil
}

// Has implements blobs.Store.
func (s *Store) Has(ctx context.Context, h blobs.Hash) (bool, error) {
	k, err := s.key(h)
	if err != nil {
		return false, err
	}
	_, err = s.c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &k})
	if err == nil {
		return true, nil
	}
	if err = s.err(k, err); errors.Is(err, blobs.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// Sweep implements blobs.Store. It lists the prefix once, and reads the metadata only of blobs no
// referrer names, to learn when each was last put. Objects under the prefix that aren't blobs are
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
	pages := s3.NewListObjectsV2Paginator(s.c, &s3.ListObjectsV2Input{Bucket: &s.bucket, Prefix: aws.String(s.prefix + "sha256/")})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return removed, s.err(s.prefix, err)
		}
		for _, obj := range page.Contents {
			k := aws.ToString(obj.Key)
			h, ok := blobs.HashOfKey(k[len(s.prefix):])
			if !ok || keep[h] {
				continue
			}
			head, err := s.c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &k})
			if err != nil {
				if errors.Is(s.err(k, err), blobs.ErrNotFound) {
					continue
				}
				return removed, s.err(k, err)
			}
			if writtenAt(head.Metadata, aws.ToTime(head.LastModified)).After(cutoff) {
				continue
			}
			if _, err := s.c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &k}); err != nil {
				return removed, s.err(k, err)
			}
			removed++
		}
	}
	return removed, nil
}

// writtenAt is when a blob was last put: its metadata where a Store wrote it, else the object's
// own time, for a blob copied in some other way.
func writtenAt(meta map[string]string, modified time.Time) time.Time {
	for k, v := range meta {
		if k == writtenKey || http.CanonicalHeaderKey(k) == http.CanonicalHeaderKey(writtenKey) {
			if sec, err := strconv.ParseInt(v, 10, 64); err == nil {
				return time.Unix(sec, 0)
			}
		}
	}
	return modified
}

// exists reports whether a conditional PutObject failed because the key is taken. S3 answers 412,
// or 409 when another upload of the same key is in flight.
func exists(err error) bool {
	code, status := codeOf(err)
	return code == "PreconditionFailed" || code == "ConditionalRequestConflict" ||
		status == http.StatusPreconditionFailed || status == http.StatusConflict
}

func codeOf(err error) (string, int) {
	code, status := "", 0
	var ae smithy.APIError
	if errors.As(err, &ae) {
		code = ae.ErrorCode()
	}
	var re *awshttp.ResponseError
	if errors.As(err, &re) {
		status = re.HTTPStatusCode()
	}
	return code, status
}

// err maps an S3 error onto the blobs and mounts sentinels: a missing key is blobs.ErrNotFound, a
// missing bucket mounts.ErrNotFound, a refusal mounts.ErrPermissionDenied, and anything else
// mounts.ErrUnavailable.
func (s *Store) err(key string, err error) error {
	code, status := codeOf(err)
	where := "s3://" + s.bucket + "/" + key
	switch {
	case code == "NoSuchBucket":
		return fmt.Errorf("%w: %s: no such bucket", mounts.ErrNotFound, where)
	case code == "NoSuchKey" || code == "NotFound" || status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", blobs.ErrNotFound, where)
	case code == "AccessDenied" || status == http.StatusForbidden:
		return fmt.Errorf("%w: %s: %v", mounts.ErrPermissionDenied, where, err)
	}
	return fmt.Errorf("%w: %s: %v", mounts.ErrUnavailable, where, err)
}

var _ blobs.Store = (*Store)(nil)
