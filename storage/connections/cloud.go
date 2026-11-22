package connections

import (
	"context"
	"fmt"

	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GCSReadScope is the OAuth scope a KindGCP connection's tokens carry. Mounts only read, so the
// token can't change a bucket.
const GCSReadScope = "https://www.googleapis.com/auth/devstorage.read_only"

// awsCredentials builds a KindAWS connection's credentials, resolving the secret key on each call
// so a rotated file or a newly sealed secret is picked up.
func awsCredentials(c Connection, secret func(field, ref string) (string, error)) (mounts.Credentials, error) {
	out := mounts.AWSCredentials{RoleARN: c.RoleARN, Region: c.Region}
	if c.KeyRef == "" && !c.Sealed {
		out.DefaultChain = true
		return out, nil
	}
	key, err := secret("key_ref", c.KeyRef)
	if err != nil {
		return nil, err
	}
	out.AccessKeyID, out.SecretAccessKey = c.AccessKeyID, key
	return out, nil
}

// gcpTokenSource builds a KindGCP connection's token source, from the service account key at
// KeyRef or sealed, or from Application Default Credentials.
func gcpTokenSource(c Connection, secret func(field, ref string) (string, error)) (oauth2.TokenSource, error) {
	ctx := context.Background()
	if c.KeyRef == "" && !c.Sealed {
		creds, err := google.FindDefaultCredentials(ctx, GCSReadScope)
		if err != nil {
			return nil, fmt.Errorf("connections: %s: application default credentials: %w", c.ID, err)
		}
		return creds.TokenSource, nil
	}
	key, err := secret("key_ref", c.KeyRef)
	if err != nil {
		return nil, err
	}
	creds, err := google.CredentialsFromJSONWithType(ctx, []byte(key), google.ServiceAccount, GCSReadScope)
	if err != nil {
		return nil, fmt.Errorf("connections: %s: service account key: %w", c.ID, err)
	}
	return creds.TokenSource, nil
}
