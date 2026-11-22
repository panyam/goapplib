// Package s3 serves mounts over prefixes in S3 buckets, "s3://<bucket>/<prefix>", through the
// AWS SDK. It is its own module so an app that doesn't mount S3 never downloads the SDK. The
// listing, pinning and fs.FS logic is in storage/mounts/bucket, and this package adapts S3 to its
// Bucket interface.
//
// A snapshot pins each object's VersionId where the bucket has versioning on. Where it is off or
// suspended, or the connection may not list versions (s3:ListBucketVersions), it pins each
// object's ETag instead and reads it with If-Match. Such a read fails with mounts.ErrConflict
// once the object changes.
//
// The driver signs with mounts.AWSCredentials, which a connections KindAWS connection gives. Nil
// credentials read a public bucket without signing. Any other kind returns
// mounts.ErrInvalidArgument.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
)

// DefaultRegion is the region a request goes to when neither the credentials nor WithRegion name
// one.
const DefaultRegion = "us-east-1"

type options struct {
	endpoint string
	region   string
	client   *http.Client
	limits   mounts.Limits
	views    string
}

// Option configures the driver.
type Option func(*options)

// WithEndpoint sends requests to url, with path-style addressing, in place of AWS's own S3. It is
// for an S3-compatible store, such as MinIO, or a test's fake.
func WithEndpoint(url string) Option { return func(o *options) { o.endpoint = url } }

// WithRegion sets the region for credentials that name none. The default is DefaultRegion.
func WithRegion(region string) Option { return func(o *options) { o.region = region } }

// WithHTTPClient sets the HTTP client for S3 and STS requests.
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.client = c } }

// WithLimits caps a source's objects and bytes, as bucket.WithLimits does.
func WithLimits(l mounts.Limits) Option { return func(o *options) { o.limits = l } }

// WithViews sets the folder where HostPath builds snapshot views, as bucket.WithViews does. With a
// store that can link, creating a snapshot mount then downloads all of its files.
func WithViews(dir string) Option { return func(o *options) { o.views = dir } }

// New returns a driver for mounts.KindS3. store keeps each Resolve's listing, as bucket.New
// describes. Clients are kept per set of credentials, so an assumed role's session is reused
// until it expires.
func New(store blobs.Store, opts ...Option) *bucket.Driver {
	o := options{region: DefaultRegion}
	for _, f := range opts {
		f(&o)
	}
	c := &connector{o: o, clients: map[mounts.AWSCredentials]*s3.Client{}}
	return bucket.New(mounts.KindS3, "s3", c.connect, store, bucket.WithLimits(o.limits), bucket.WithViews(o.views))
}

type connector struct {
	o       options
	mu      sync.Mutex
	clients map[mounts.AWSCredentials]*s3.Client
}

func (c *connector) connect(ctx context.Context, name string, creds mounts.Credentials) (bucket.Bucket, error) {
	var key mounts.AWSCredentials
	anon := false
	switch v := creds.(type) {
	case nil:
		anon = true
	case mounts.AWSCredentials:
		key = v
	default:
		return nil, fmt.Errorf("%w: S3 can't sign with %s credentials", mounts.ErrInvalidArgument, creds.CredentialKind())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cl, ok := c.clients[key]
	if !ok || anon {
		var err error
		if cl, err = c.client(ctx, key, anon); err != nil {
			return nil, err
		}
		if !anon {
			c.clients[key] = cl
		}
	}
	return &s3Bucket{c: cl, name: name}, nil
}

func (c *connector) client(ctx context.Context, k mounts.AWSCredentials, anon bool) (*s3.Client, error) {
	region := k.Region
	if region == "" {
		region = c.o.region
	}
	var provider aws.CredentialsProvider
	switch {
	case anon:
		provider = aws.AnonymousCredentials{}
	case k.DefaultChain:
		opts := []func(*config.LoadOptions) error{config.WithRegion(region)}
		if c.o.client != nil {
			opts = append(opts, config.WithHTTPClient(c.o.client))
		}
		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("%w: the AWS default credential chain: %v", mounts.ErrUnavailable, err)
		}
		provider = cfg.Credentials
	default:
		if k.AccessKeyID == "" || k.SecretAccessKey == "" {
			return nil, fmt.Errorf("%w: AWS credentials need a key pair or the default chain", mounts.ErrInvalidArgument)
		}
		provider = credentials.NewStaticCredentialsProvider(k.AccessKeyID, k.SecretAccessKey, k.SessionToken)
	}
	if k.RoleARN != "" && !anon {
		stsClient := sts.New(sts.Options{Region: region, Credentials: provider, HTTPClient: c.httpClient()})
		provider = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(stsClient, k.RoleARN))
	}
	return s3.New(s3.Options{
		Region:       region,
		Credentials:  provider,
		HTTPClient:   c.httpClient(),
		BaseEndpoint: endpoint(c.o.endpoint),
		UsePathStyle: c.o.endpoint != "",
	}), nil
}

func (c *connector) httpClient() s3.HTTPClient {
	if c.o.client == nil {
		return awshttp.NewBuildableClient()
	}
	return c.o.client
}

func endpoint(u string) *string {
	if u == "" {
		return nil
	}
	return aws.String(u)
}

// s3Bucket is one bucket through one client.
type s3Bucket struct {
	c    *s3.Client
	name string
}

// List lists every latest object under prefix with its VersionId, falling back to ETags when the
// bucket or the credentials don't allow listing versions.
func (b *s3Bucket) List(ctx context.Context, prefix string) ([]bucket.Object, error) {
	out, err := b.listVersions(ctx, prefix)
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, mounts.ErrPermissionDenied) && !errors.Is(err, errNotImplemented) {
		return nil, err
	}
	return b.listObjects(ctx, prefix)
}

var errNotImplemented = errors.New("not implemented")

func (b *s3Bucket) listVersions(ctx context.Context, prefix string) ([]bucket.Object, error) {
	var out []bucket.Object
	p := s3.NewListObjectVersionsPaginator(b.c, &s3.ListObjectVersionsInput{Bucket: &b.name, Prefix: &prefix})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, b.err("listing versions of "+prefix, err, false)
		}
		for _, v := range page.Versions {
			if !aws.ToBool(v.IsLatest) {
				continue
			}
			o := bucket.Object{Key: aws.ToString(v.Key), ETag: aws.ToString(v.ETag), Size: aws.ToInt64(v.Size)}
			if id := aws.ToString(v.VersionId); id != "null" {
				o.Version = id
			}
			out = append(out, o)
		}
	}
	return out, nil
}

func (b *s3Bucket) listObjects(ctx context.Context, prefix string) ([]bucket.Object, error) {
	var out []bucket.Object
	p := s3.NewListObjectsV2Paginator(b.c, &s3.ListObjectsV2Input{Bucket: &b.name, Prefix: &prefix})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, b.err("listing "+prefix, err, false)
		}
		for _, o := range page.Contents {
			out = append(out, bucket.Object{Key: aws.ToString(o.Key), ETag: aws.ToString(o.ETag), Size: aws.ToInt64(o.Size)})
		}
	}
	return out, nil
}

// ListDir lists one folder level with "/" as the delimiter.
func (b *s3Bucket) ListDir(ctx context.Context, dir string) ([]bucket.Object, []string, error) {
	var files []bucket.Object
	var folders []string
	p := s3.NewListObjectsV2Paginator(b.c, &s3.ListObjectsV2Input{Bucket: &b.name, Prefix: &dir, Delimiter: aws.String("/")})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, nil, b.err("listing "+dir, err, false)
		}
		for _, o := range page.Contents {
			files = append(files, bucket.Object{Key: aws.ToString(o.Key), ETag: aws.ToString(o.ETag), Size: aws.ToInt64(o.Size)})
		}
		for _, cp := range page.CommonPrefixes {
			folders = append(folders, strings.TrimSuffix(strings.TrimPrefix(aws.ToString(cp.Prefix), dir), "/"))
		}
	}
	return files, folders, nil
}

// Get reads key, at pin's VersionId, or with If-Match on pin's ETag.
func (b *s3Bucket) Get(ctx context.Context, key string, pin *bucket.Object) (io.ReadCloser, int64, error) {
	in := &s3.GetObjectInput{Bucket: &b.name, Key: &key}
	if pin != nil {
		if pin.Version != "" {
			in.VersionId = aws.String(pin.Version)
		} else {
			in.IfMatch = aws.String(pin.ETag)
		}
	}
	out, err := b.c.GetObject(ctx, in)
	if err != nil {
		return nil, 0, b.err(key, err, pin != nil)
	}
	return out.Body, aws.ToInt64(out.ContentLength), nil
}

// err maps an S3 error onto the mounts sentinels. With pinned set, a missing key or version, or a
// failed If-Match, means the pinned contents are gone, which is mounts.ErrConflict.
func (b *s3Bucket) err(what string, err error, pinned bool) error {
	code, status := "", 0
	var ae smithy.APIError
	if errors.As(err, &ae) {
		code = ae.ErrorCode()
	}
	var re *awshttp.ResponseError
	if errors.As(err, &re) {
		status = re.HTTPStatusCode()
	}
	where := "s3://" + b.name + "/" + what
	switch {
	case code == "NoSuchBucket":
		return fmt.Errorf("%w: %s: no such bucket", mounts.ErrNotFound, where)
	case code == "PreconditionFailed" || status == http.StatusPreconditionFailed:
		return fmt.Errorf("%w: %s changed since the snapshot listed it", mounts.ErrConflict, where)
	case code == "NoSuchKey" || code == "NoSuchVersion" || code == "NotFound" || status == http.StatusNotFound:
		if pinned {
			return fmt.Errorf("%w: %s: the snapshot's version is gone", mounts.ErrConflict, where)
		}
		return fmt.Errorf("%w: %s", mounts.ErrNotFound, where)
	case code == "AccessDenied" || status == http.StatusForbidden:
		return fmt.Errorf("%w: %s: %v", mounts.ErrPermissionDenied, where, err)
	case code == "NotImplemented" || status == http.StatusNotImplemented:
		return fmt.Errorf("%w: %s: %w", mounts.ErrUnavailable, where, errNotImplemented)
	}
	return fmt.Errorf("%w: %s: %v", mounts.ErrUnavailable, where, err)
}
