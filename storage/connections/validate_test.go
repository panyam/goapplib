package connections

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateAcceptsEachKind(t *testing.T) {
	for _, c := range []Connection{
		{ID: "acme", Kind: KindGitHubApp, Scope: ScopeServer, AppID: "12345", InstallationID: 1, KeyRef: "file:/run/secrets/gh-app.pem"},
		{ID: "ci-pat", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "env:GITHUB_TOKEN"},
		{ID: "alice-pat", Kind: KindGitHubToken, Scope: ScopePerson, Owner: "alice", TokenRef: "env:ALICE_TOKEN"},
		{ID: "alice-gh", Kind: KindGitHubUser, Scope: ScopePerson, Owner: "alice"},
		{ID: "alice-paste", Kind: KindGitHubUser, Scope: ScopePerson, Owner: "alice", Pasted: true},
		{ID: "s3-keys", Kind: KindAWS, Scope: ScopeServer, AccessKeyID: "AKIAEXAMPLE", KeyRef: "env:AWS_SECRET", Region: "us-east-1"},
		{ID: "s3-chain", Kind: KindAWS, Scope: ScopeServer, RoleARN: "arn:aws:iam::123456789012:role/reader"},
		{ID: "gcs-key", Kind: KindGCP, Scope: ScopeServer, KeyRef: "file:/run/secrets/sa.json"},
		{ID: "gcs-adc", Kind: KindGCP, Scope: ScopeServer},
	} {
		if err := c.Validate(); err != nil {
			t.Errorf("%s: %v", c.ID, err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	const raw = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"
	for name, c := range map[string]Connection{
		"bad id":                 {ID: "Acme", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "env:T"},
		"unknown kind":           {ID: "x", Kind: "gitlab", Scope: ScopeServer},
		"unknown scope":          {ID: "x", Kind: KindGitHubToken, TokenRef: "env:T"},
		"server with owner":      {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, Owner: "alice", TokenRef: "env:T"},
		"person without owner":   {ID: "x", Kind: KindGitHubUser, Scope: ScopePerson},
		"app for a person":       {ID: "x", Kind: KindGitHubApp, Scope: ScopePerson, Owner: "alice", AppID: "1", InstallationID: 1, KeyRef: "env:K"},
		"app without install":    {ID: "x", Kind: KindGitHubApp, Scope: ScopeServer, AppID: "1", KeyRef: "env:K"},
		"app without key":        {ID: "x", Kind: KindGitHubApp, Scope: ScopeServer, AppID: "1", InstallationID: 1},
		"pasted on a token kind": {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "env:T", Pasted: true},
		"raw token in token_ref": {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: raw},
		"relative file ref":      {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "file:secrets/t"},
		"user with token_ref":    {ID: "x", Kind: KindGitHubUser, Scope: ScopePerson, Owner: "alice", TokenRef: "env:T"},
		"server user connection": {ID: "x", Kind: KindGitHubUser, Scope: ScopeServer},
		"aws for a person":       {ID: "x", Kind: KindAWS, Scope: ScopePerson, Owner: "alice"},
		"aws key without secret": {ID: "x", Kind: KindAWS, Scope: ScopeServer, AccessKeyID: "AKIAEXAMPLE"},
		"aws secret without key": {ID: "x", Kind: KindAWS, Scope: ScopeServer, KeyRef: "env:S"},
		"aws raw secret":         {ID: "x", Kind: KindAWS, Scope: ScopeServer, AccessKeyID: "AKIAEXAMPLE", KeyRef: raw},
		"aws role not an arn":    {ID: "x", Kind: KindAWS, Scope: ScopeServer, RoleARN: "reader"},
		"aws with base url":      {ID: "x", Kind: KindAWS, Scope: ScopeServer, BaseURL: "https://s3.example"},
		"aws with token_ref":     {ID: "x", Kind: KindAWS, Scope: ScopeServer, TokenRef: "env:T"},
		"gcp for a person":       {ID: "x", Kind: KindGCP, Scope: ScopePerson, Owner: "alice"},
		"gcp raw key":            {ID: "x", Kind: KindGCP, Scope: ScopeServer, KeyRef: raw},
		"gcp with a role":        {ID: "x", Kind: KindGCP, Scope: ScopeServer, RoleARN: "arn:aws:iam::1:role/r"},
		"github with a region":   {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "env:T", Region: "us-east-1"},
	} {
		err := c.Validate()
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Validate() = %v, want ErrInvalid", name, err)
			continue
		}
		if strings.Contains(err.Error(), raw) {
			t.Errorf("%s: the error repeats the secret: %v", name, err)
		}
	}
}

func TestViewer(t *testing.T) {
	ctx := context.Background()
	if v := Viewer(ctx); v != "" {
		t.Errorf("Viewer of a bare context = %q, want empty", v)
	}
	if v := Viewer(WithViewer(ctx, "alice")); v != "alice" {
		t.Errorf("Viewer = %q, want alice", v)
	}
}

func TestValidateSealed(t *testing.T) {
	for _, c := range []Connection{
		{ID: "pat", Kind: KindGitHubToken, Scope: ScopeServer, Sealed: true},
		{ID: "app", Kind: KindGitHubApp, Scope: ScopeServer, AppID: "1", InstallationID: 1, Sealed: true},
		{ID: "s3", Kind: KindAWS, Scope: ScopeServer, AccessKeyID: "AKIAEXAMPLE", Sealed: true},
		{ID: "gcs", Kind: KindGCP, Scope: ScopeServer, Sealed: true},
		{ID: "graph", Kind: KindEntra, Scope: ScopeServer, TenantID: "t", ClientID: "c", EntraAuth: EntraSecret, Sealed: true},
		{ID: "graph-cert", Kind: KindEntra, Scope: ScopeServer, TenantID: "t", ClientID: "c", EntraAuth: EntraCertificate,
			CertRef: "file:/run/secrets/cert.pem", Sealed: true},
	} {
		if err := c.Validate(); err != nil {
			t.Errorf("%s: %v", c.ID, err)
		}
	}
	for name, c := range map[string]Connection{
		"person token":         {ID: "x", Kind: KindGitHubToken, Scope: ScopePerson, Owner: "alice", Sealed: true},
		"user connection":      {ID: "x", Kind: KindGitHubUser, Scope: ScopePerson, Owner: "alice", Sealed: true},
		"token with token_ref": {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "env:T", Sealed: true},
		"app with key_ref":     {ID: "x", Kind: KindGitHubApp, Scope: ScopeServer, AppID: "1", InstallationID: 1, KeyRef: "env:K", Sealed: true},
		"aws without key id":   {ID: "x", Kind: KindAWS, Scope: ScopeServer, Sealed: true},
		"gcs with key_ref":     {ID: "x", Kind: KindGCP, Scope: ScopeServer, KeyRef: "env:K", Sealed: true},
		"managed identity":     {ID: "x", Kind: KindEntra, Scope: ScopeServer, EntraAuth: EntraManagedIdentity, Sealed: true},
		"workload identity": {ID: "x", Kind: KindEntra, Scope: ScopeServer, TenantID: "t", ClientID: "c", EntraAuth: EntraWorkloadIdentity,
			KeyRef: "file:/var/run/token", Sealed: true},
		"cert without cert_ref": {ID: "x", Kind: KindEntra, Scope: ScopeServer, TenantID: "t", ClientID: "c", EntraAuth: EntraCertificate, Sealed: true},
	} {
		if err := c.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Validate = %v, want ErrInvalid", name, err)
		}
	}
}
