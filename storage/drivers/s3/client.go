package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/panyam/goapplib/storage/mounts"
)

// NewClient returns an S3 client signed with creds, built the way the mount driver builds its own,
// for other code in this module that talks to S3, such as s3blobs. A nil creds sends unsigned
// requests, for a public bucket. Bearer credentials return mounts.ErrInvalidArgument, since S3
// signs every request with SigV4.
func NewClient(ctx context.Context, creds mounts.Credentials, opts ...Option) (*s3.Client, error) {
	o := options{region: DefaultRegion}
	for _, f := range opts {
		f(&o)
	}
	c := &connector{o: o}
	switch v := creds.(type) {
	case nil:
		return c.client(ctx, mounts.AWSCredentials{}, true)
	case mounts.AWSCredentials:
		return c.client(ctx, v, false)
	}
	return nil, fmt.Errorf("%w: S3 can't sign with %s credentials", mounts.ErrInvalidArgument, creds.CredentialKind())
}
