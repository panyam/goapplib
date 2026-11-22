package gcs

import (
	"context"

	"cloud.google.com/go/storage"
	"github.com/panyam/goapplib/storage/mounts"
)

// NewClient returns a Cloud Storage client that sends creds' bearer token, built the way the mount
// driver builds its own, for other code in this module, such as gcsblobs. Nil creds sends no
// token, for a public bucket. AWS credentials return mounts.ErrInvalidArgument.
func NewClient(ctx context.Context, creds mounts.Credentials, opts ...Option) (*storage.Client, error) {
	var o options
	for _, f := range opts {
		f(&o)
	}
	return o.newClient(ctx, creds)
}
