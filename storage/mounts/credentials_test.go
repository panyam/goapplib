package mounts_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

func TestBearerOfNilIsNil(t *testing.T) {
	if c := mounts.Bearer(nil); c != nil {
		t.Fatalf("Bearer(nil) = %v, want nil so a public source sends no token", c)
	}
	if tok, err := mounts.BearerToken(nil); tok != "" || err != nil {
		t.Fatalf("BearerToken(nil) = %q, %v; want no token and no error", tok, err)
	}
}

func TestBearerTokenReadsTheSource(t *testing.T) {
	c := mounts.Bearer(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "ghs_1"}))
	if tok, err := mounts.BearerToken(c); tok != "ghs_1" || err != nil {
		t.Fatalf("BearerToken = %q, %v; want ghs_1", tok, err)
	}
	if c.CredentialKind() != "bearer" {
		t.Errorf("CredentialKind = %q, want bearer", c.CredentialKind())
	}
}

func TestAWSCredentialsCantSignABearerRequest(t *testing.T) {
	_, err := mounts.TokenSourceOf(mounts.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "s3cr3t"})
	if !errors.Is(err, mounts.ErrInvalidArgument) || !strings.Contains(err.Error(), "aws") {
		t.Fatalf("TokenSourceOf(aws) = %v, want ErrInvalidArgument naming aws", err)
	}
}

func TestAWSCredentialsHideTheSecret(t *testing.T) {
	c := mounts.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "s3cr3t", SessionToken: "sess10n"}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(verb, c); strings.Contains(out, "s3cr3t") || strings.Contains(out, "sess10n") {
			t.Errorf("%s printed %q, which holds a secret", verb, out)
		}
	}
}
