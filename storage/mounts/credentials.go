package mounts

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
)

// Credentials is how a driver signs its requests to a source. The registry gets them from the
// mount's connection (WithCredentials) and hands them to the driver unchanged. A nil Credentials
// means the source is public or local, and the driver sends no sign-in.
//
// Each service signs in its own way, so a driver type-switches on the value and takes the kinds
// it understands:
//
//   - BearerCredentials carries an OAuth bearer token, as GitHub and Google Cloud Storage take.
//   - AWSCredentials carries what an AWS SDK needs to sign with SigV4, which has no bearer token.
//
// A driver given a kind it can't use returns ErrInvalidArgument, naming the kind. A package can
// add its own kind by implementing CredentialKind, so this package never imports a cloud SDK.
type Credentials interface {
	// CredentialKind names the kind, such as "bearer" or "aws", for error messages.
	CredentialKind() string
}

// BearerCredentials signs each request with a token from Source, as "Authorization: Bearer".
type BearerCredentials struct {
	Source oauth2.TokenSource
}

// CredentialKind returns "bearer".
func (BearerCredentials) CredentialKind() string { return "bearer" }

// Bearer wraps ts as Credentials. A nil ts returns nil, so a public source stays public.
func Bearer(ts oauth2.TokenSource) Credentials {
	if ts == nil {
		return nil
	}
	return BearerCredentials{Source: ts}
}

// TokenSourceOf returns the token source of bearer credentials. Nil credentials return a nil
// source and no error, which means "send no token". Any other kind returns ErrInvalidArgument,
// since a bearer-token driver can't sign with it.
func TokenSourceOf(c Credentials) (oauth2.TokenSource, error) {
	switch c := c.(type) {
	case nil:
		return nil, nil
	case BearerCredentials:
		return c.Source, nil
	case *BearerCredentials:
		return c.Source, nil
	default:
		return nil, fmt.Errorf("%w: %s credentials can't sign a bearer-token request", ErrInvalidArgument, c.CredentialKind())
	}
}

// BearerToken returns the access token c gives, or "" for nil credentials. It is
// TokenSourceOf followed by Token, for a driver that sets the header itself.
func BearerToken(c Credentials) (string, error) {
	ts, err := TokenSourceOf(c)
	if err != nil || ts == nil {
		return "", err
	}
	tok, err := ts.Token()
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// AWSCredentials is what an S3 driver needs to sign requests with SigV4. The connections service
// resolves the secret before building it, so the value holds a secret and must not be logged or
// stored.
//
// With DefaultChain set, the driver ignores the key pair and uses the AWS SDK's default chain:
// environment variables, the shared config files, an EC2 instance role, or an IRSA role on
// Kubernetes. Otherwise AccessKeyID and SecretAccessKey must both be set. A non-empty RoleARN
// makes the driver assume that role with the base credentials first. An empty Region means the
// driver's own default.
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	DefaultChain    bool
	RoleARN         string
	Region          string
}

// CredentialKind returns "aws".
func (AWSCredentials) CredentialKind() string { return "aws" }

// String hides the secret, so a stray %v never prints it.
func (c AWSCredentials) String() string {
	return fmt.Sprintf("aws{key=%s chain=%t role=%s region=%s}", c.AccessKeyID, c.DefaultChain, c.RoleARN, c.Region)
}

// CredentialsFunc returns the credentials of owner's connection, as WithCredentials takes it. The
// connections service's Credentials method has this signature.
type CredentialsFunc func(ctx context.Context, owner, connection string) (Credentials, error)

// GoString hides the secret from %#v as well.
func (c AWSCredentials) GoString() string { return c.String() }
