package connections_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/fsstore"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/oneauth/outbound"
	"github.com/panyam/oneauth/outbound/githubtest"
	oneauthfs "github.com/panyam/oneauth/stores/fs"
)

func sealedToken() connections.Connection {
	return connections.Connection{ID: "hw", Kind: connections.KindGitHubToken, Scope: connections.ScopeServer, Sealed: true}
}

// replica is a second service over f's stores, as another server process would run.
func (f *fixture) replica(t *testing.T) *connections.Service {
	t.Helper()
	store, err := fsstore.New(f.dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	return connections.NewService(store, connections.WithUserTokens(oneauthfs.NewFSUserTokenStore(f.dirs[1]), "env:GOAPPLIB_TEST_CONN_KEY", nil))
}

func TestASealedServerTokenIsSavedUsedRotatedAndDeleted(t *testing.T) {
	f := newFixture(t)
	f.put(t, sealedToken())
	ctx := context.Background()

	if _, err := f.svc.Credentials(ctx, "", "hw"); !errors.Is(err, connections.ErrNoSecret) {
		t.Fatalf("Credentials before a save = %v, want ErrNoSecret", err)
	}
	if err := f.svc.SaveServerSecret(ctx, "hw", "ghp_first"); err != nil {
		t.Fatal(err)
	}
	if tok := token(t)(f.svc.Credentials(ctx, "", "hw")); tok.AccessToken != "ghp_first" {
		t.Errorf("token = %q, want the saved ghp_first", tok.AccessToken)
	}

	other := f.replica(t)
	if tok := token(t)(other.Credentials(ctx, "", "hw")); tok.AccessToken != "ghp_first" {
		t.Errorf("another process's token = %q, want ghp_first", tok.AccessToken)
	}
	if err := f.svc.SaveServerSecret(ctx, "hw", "ghp_second"); err != nil {
		t.Fatal(err)
	}
	if tok := token(t)(other.Credentials(ctx, "", "hw")); tok.AccessToken != "ghp_second" {
		t.Errorf("another process's token after a rotation = %q, want ghp_second", tok.AccessToken)
	}

	if err := f.svc.Disconnect(ctx, "", "hw"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.GetConnection(ctx, "", "hw"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("the record after Disconnect: %v, want ErrNotFound", err)
	}
	f.put(t, sealedToken())
	if _, err := f.svc.Credentials(ctx, "", "hw"); !errors.Is(err, connections.ErrNoSecret) {
		t.Errorf("Credentials for a new record with the old ID = %v, want ErrNoSecret, since Disconnect deleted the secret", err)
	}
}

func TestASealedAppKeyMintsInstallationTokens(t *testing.T) {
	f := newFixture(t)
	c := f.app()
	c.KeyRef, c.Sealed = "", true
	f.put(t, c)
	ctx := context.Background()
	if err := f.svc.SaveServerSecret(ctx, "acme", string(f.gh.AppKeyPEM())); err != nil {
		t.Fatal(err)
	}
	if tok := token(t)(f.svc.Credentials(ctx, "", "acme")); !f.gh.ValidInstallationToken(tok.AccessToken) {
		t.Errorf("the fake doesn't accept the installation token from a sealed key")
	}

	stranger := githubtest.New(t)
	if err := f.svc.SaveServerSecret(ctx, "acme", string(stranger.AppKeyPEM())); err != nil {
		t.Fatal(err)
	}
	creds, err := f.svc.Credentials(ctx, "", "acme")
	if err != nil {
		t.Fatal(err)
	}
	src, err := mounts.TokenSourceOf(creds)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Token(); err == nil {
		t.Error("a token signed with a rotated-in key the App doesn't have = nil error, want the fake's refusal")
	}
}

func TestASealedAWSKeyComesBackBesideItsKeyID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.store.PutConnection(ctx, connections.Connection{ID: "s3", Kind: connections.KindAWS, Scope: connections.ScopeServer,
		AccessKeyID: "AKIAEXAMPLE", Sealed: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SaveServerSecret(ctx, "s3", "aws-secret"); err != nil {
		t.Fatal(err)
	}
	creds, err := f.svc.Credentials(ctx, "", "s3")
	if err != nil {
		t.Fatal(err)
	}
	aws, ok := creds.(mounts.AWSCredentials)
	if !ok || aws.AccessKeyID != "AKIAEXAMPLE" || aws.SecretAccessKey != "aws-secret" || aws.DefaultChain {
		t.Errorf("credentials = %#v, want the key ID and the sealed secret", creds)
	}
}

func TestSaveServerSecretRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	t.Setenv("GOAPPLIB_TEST_PAT", "pat")
	f.put(t, connections.Connection{ID: "byref", Kind: connections.KindGitHubToken, Scope: connections.ScopeServer, TokenRef: "env:GOAPPLIB_TEST_PAT"})
	f.put(t, sealedToken())

	for name, c := range map[string]struct {
		svc    *connections.Service
		id     string
		secret string
		want   error
	}{
		"a record without Sealed": {f.svc, "byref", "s", connections.ErrInvalid},
		"an empty secret":         {f.svc, "hw", "", connections.ErrInvalid},
		"a missing connection":    {f.svc, "nope", "s", connections.ErrNotFound},
		"no WithUserTokens":       {connections.NewService(f.store), "hw", "s", connections.ErrNotConfigured},
	} {
		if err := c.svc.SaveServerSecret(ctx, c.id, c.secret); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
}

func TestASealedSecretIsInNoStoredFileInTheClear(t *testing.T) {
	f := newFixture(t)
	f.put(t, sealedToken())
	ctx := context.Background()
	const secret = "ghp_sealed_0123456789abcdefghijklmnop"
	if err := f.svc.SaveServerSecret(ctx, "hw", secret); err != nil {
		t.Fatal(err)
	}
	for _, dir := range f.dirs {
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			if err == nil && strings.Contains(string(b), secret) {
				t.Errorf("%s holds the secret in the clear", p)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err := f.store.GetConnection(ctx, "", "hw")
	if err != nil || !c.Sealed || c.TokenRef != "" {
		t.Errorf("the record = %+v (err %v), want Sealed and no reference", c, err)
	}
}

func TestASealedSecretDoesNotOpenUnderAnotherID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.put(t, sealedToken())
	other := sealedToken()
	other.ID = "copy"
	f.put(t, other)
	if err := f.svc.SaveServerSecret(ctx, "hw", "ghp_hw"); err != nil {
		t.Fatal(err)
	}
	users := oneauthfs.NewFSUserTokenStore(f.dirs[1])
	got, err := users.GetUserToken(ctx, &outbound.GetUserTokenRequest{Provider: "goapplib-server-secret:hw", Subject: "server"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.PutUserToken(ctx, &outbound.PutUserTokenRequest{Provider: "goapplib-server-secret:copy", Subject: "server", Sealed: got.Record.Sealed}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Credentials(ctx, "", "copy"); err == nil || errors.Is(err, connections.ErrNoSecret) {
		t.Errorf("a sealed secret copied under another ID = %v, want an error opening it", err)
	}
}
