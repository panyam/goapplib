package connections

import (
	"context"
	"fmt"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/oneauth/outbound"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GCSReadScope is the OAuth scope a KindGCP connection's tokens carry. Mounts only read, so the
// token can't change a bucket.
const GCSReadScope = "https://www.googleapis.com/auth/devstorage.read_only"

// awsCredentials builds a KindAWS connection's credentials, resolving the secret key on each call
// so a rotated file is picked up.
func awsCredentials(c Connection) (mounts.Credentials, error) {
	out := mounts.AWSCredentials{RoleARN: c.RoleARN, Region: c.Region}
	if c.KeyRef == "" {
		out.DefaultChain = true
		return out, nil
	}
	ref, err := outbound.ParseSecretRef(c.KeyRef)
	if err != nil {
		return nil, fmt.Errorf("%w: key_ref", ErrInvalid)
	}
	secret, err := ref.Resolve()
	if err != nil {
		return nil, fmt.Errorf("connections: %s: secret access key: %w", c.ID, err)
	}
	out.AccessKeyID, out.SecretAccessKey = c.AccessKeyID, secret
	return out, nil
}

// gcpTokenSource builds a KindGCP connection's token source, from the service account key at
// KeyRef or from Application Default Credentials.
func gcpTokenSource(c Connection) (oauth2.TokenSource, error) {
	ctx := context.Background()
	if c.KeyRef == "" {
		creds, err := google.FindDefaultCredentials(ctx, GCSReadScope)
		if err != nil {
			return nil, fmt.Errorf("connections: %s: application default credentials: %w", c.ID, err)
		}
		return creds.TokenSource, nil
	}
	ref, err := outbound.ParseSecretRef(c.KeyRef)
	if err != nil {
		return nil, fmt.Errorf("%w: key_ref", ErrInvalid)
	}
	key, err := ref.Resolve()
	if err != nil {
		return nil, fmt.Errorf("connections: %s: service account key: %w", c.ID, err)
	}
	creds, err := google.CredentialsFromJSONWithType(ctx, []byte(key), google.ServiceAccount, GCSReadScope)
	if err != nil {
		return nil, fmt.Errorf("connections: %s: service account key: %w", c.ID, err)
	}
	return creds.TokenSource, nil
}
