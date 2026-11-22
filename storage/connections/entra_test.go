package connections_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/entratest"
	"github.com/panyam/goapplib/storage/connections/fsstore"
	"github.com/panyam/goapplib/storage/mounts"
	oneauthfs "github.com/panyam/oneauth/stores/fs"
	"golang.org/x/oauth2"
)

const (
	tenant   = "contoso.onmicrosoft.com"
	clientID = "11111111-2222-3333-4444-555555555555"
)

// entraFixture is a connections service over a folder store, with user tokens, pointed at a fake
// Entra for both the authority and the managed identity endpoint.
func entraFixture(t *testing.T) (*entratest.Server, *fsstore.Store, *connections.Service) {
	t.Helper()
	ent := entratest.New(t)
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOAPPLIB_TEST_CONN_KEY", masterKey)
	svc := connections.NewService(store,
		connections.WithUserTokens(oneauthfs.NewFSUserTokenStore(t.TempDir()), "env:GOAPPLIB_TEST_CONN_KEY", nil),
		connections.WithManagedIdentityURL(ent.ManagedIdentityURL()))
	return ent, store, svc
}

func entraToken(t *testing.T, svc *connections.Service, ctx context.Context, owner, id string) string {
	t.Helper()
	creds, err := svc.Credentials(ctx, owner, id)
	if err != nil {
		t.Fatalf("Credentials(%s): %v", id, err)
	}
	tok, err := mounts.BearerToken(creds)
	if err != nil {
		t.Fatalf("Token(%s): %v", id, err)
	}
	return tok
}

func putConn(t *testing.T, store *fsstore.Store, c connections.Connection) {
	t.Helper()
	if err := store.PutConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func TestEntraClientSecretGetsAGraphToken(t *testing.T) {
	ent, store, svc := entraFixture(t)
	ent.AddSecretApp(tenant, clientID, "s3cret")
	t.Setenv("GOAPPLIB_TEST_ENTRA_SECRET", "s3cret")
	putConn(t, store, connections.Connection{ID: "sp", Kind: connections.KindEntra, Scope: connections.ScopeServer, BaseURL: ent.URL,
		TenantID: tenant, ClientID: clientID, EntraAuth: connections.EntraSecret, KeyRef: "env:GOAPPLIB_TEST_ENTRA_SECRET"})
	tok := entraToken(t, svc, context.Background(), "", "sp")
	if !ent.ValidToken(tok) {
		t.Errorf("token %q wasn't issued by the fake", tok)
	}
	reqs := ent.Requests()
	if len(reqs) != 1 || reqs[0].GrantType != "client_credentials" || reqs[0].Scope != connections.GraphScope || reqs[0].Assertion {
		t.Errorf("requests = %+v; want one client_credentials request for the Graph scope with a secret", reqs)
	}
	entraToken(t, svc, context.Background(), "", "sp")
	if n := len(ent.Requests()); n != 1 {
		t.Errorf("%d token requests after a second Credentials, want the cached token", n)
	}
}

func TestEntraCertificateSignsAClientAssertion(t *testing.T) {
	ent, store, svc := entraFixture(t)
	keyPEM, certPEM, cert := entratest.NewCertificate(t)
	ent.AddCertApp(tenant, clientID, cert)
	dir := t.TempDir()
	keyPath, certPath := filepath.Join(dir, "graph.key"), filepath.Join(dir, "graph.crt")
	writeFile(t, os.WriteFile(keyPath, keyPEM, 0o600))
	writeFile(t, os.WriteFile(certPath, certPEM, 0o600))
	putConn(t, store, connections.Connection{ID: "sp", Kind: connections.KindEntra, Scope: connections.ScopeServer, BaseURL: ent.URL,
		TenantID: tenant, ClientID: clientID, EntraAuth: connections.EntraCertificate, KeyRef: "file:" + keyPath, CertRef: "file:" + certPath})
	if tok := entraToken(t, svc, context.Background(), "", "sp"); !ent.ValidToken(tok) {
		t.Errorf("token %q wasn't issued by the fake", tok)
	}
	if reqs := ent.Requests(); len(reqs) != 1 || !reqs[0].Assertion {
		t.Errorf("requests = %+v; want one with a client assertion", reqs)
	}

	// A key from another certificate is refused before any request.
	otherKey, _, _ := entratest.NewCertificate(t)
	writeFile(t, os.WriteFile(keyPath, otherKey, 0o600))
	putConn(t, store, connections.Connection{ID: "sp2", Kind: connections.KindEntra, Scope: connections.ScopeServer, BaseURL: ent.URL,
		TenantID: tenant, ClientID: clientID, EntraAuth: connections.EntraCertificate, KeyRef: "file:" + keyPath, CertRef: "file:" + certPath})
	if _, err := svc.Credentials(context.Background(), "", "sp2"); !errors.Is(err, connections.ErrInvalid) || !strings.Contains(err.Error(), "doesn't match") {
		t.Errorf("Credentials with a mismatched key = %v, want ErrInvalid saying it doesn't match", err)
	}
}

func TestEntraManagedIdentityAsksTheHostEndpoint(t *testing.T) {
	ent, store, svc := entraFixture(t)
	ent.AddManagedIdentity("user-assigned-id")
	putConn(t, store, connections.Connection{ID: "mi", Kind: connections.KindEntra, Scope: connections.ScopeServer,
		ClientID: "user-assigned-id", EntraAuth: connections.EntraManagedIdentity})
	if tok := entraToken(t, svc, context.Background(), "", "mi"); !ent.ValidToken(tok) {
		t.Errorf("token %q wasn't issued by the fake", tok)
	}
	reqs := ent.Requests()
	if len(reqs) != 1 || reqs[0].Metadata != "true" || reqs[0].Scope != connections.GraphResource || reqs[0].ClientID != "user-assigned-id" {
		t.Errorf("requests = %+v; want one IMDS request for Graph with Metadata: true and the client ID", reqs)
	}
}

func TestEntraUserTokenIsRefreshedWithTheGraphScope(t *testing.T) {
	ent, store, svc := entraFixture(t)
	ent.AddSecretApp(tenant, clientID, "s3cret")
	t.Setenv("GOAPPLIB_TEST_ENTRA_SECRET", "s3cret")
	putConn(t, store, connections.Connection{ID: "od", Kind: connections.KindEntraUser, Scope: connections.ScopePerson, Owner: "alice",
		BaseURL: ent.URL, TenantID: tenant, ClientID: clientID, KeyRef: "env:GOAPPLIB_TEST_ENTRA_SECRET"})
	ctx := connections.WithViewer(context.Background(), "alice")
	signed := ent.SignIn(tenant, clientID, "alice")
	// The access token from sign-in is already expired, so the first use refreshes it.
	if err := svc.SaveUserToken(ctx, "alice", "od", &oauth2.Token{AccessToken: signed.AccessToken, RefreshToken: signed.RefreshToken, Expiry: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	tok := entraToken(t, svc, ctx, "alice", "od")
	if tok == signed.AccessToken || !ent.ValidToken(tok) {
		t.Errorf("token = %q; want a refreshed token from the fake", tok)
	}
	reqs := ent.Requests()
	if len(reqs) != 1 || reqs[0].GrantType != "refresh_token" || reqs[0].Scope != connections.GraphUserScope {
		t.Errorf("requests = %+v; want one refresh with the Graph user scope", reqs)
	}
	if err := svc.Disconnect(ctx, "alice", "od"); err != nil {
		t.Errorf("Disconnect = %v; an Entra user token is deleted, with nothing to revoke", err)
	}
	if _, err := store.GetConnection(ctx, "alice", "od"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("record after Disconnect: %v, want ErrNotFound", err)
	}
}

func TestEntraValidate(t *testing.T) {
	base := connections.Connection{ID: "sp", Kind: connections.KindEntra, Scope: connections.ScopeServer,
		TenantID: tenant, ClientID: clientID, EntraAuth: connections.EntraSecret, KeyRef: "env:X"}
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate(secret) = %v", err)
	}
	for name, mod := range map[string]func(*connections.Connection){
		"no auth":          func(c *connections.Connection) { c.EntraAuth = "" },
		"raw secret":       func(c *connections.Connection) { c.KeyRef = "hunter2" },
		"no tenant":        func(c *connections.Connection) { c.TenantID = "" },
		"person scope":     func(c *connections.Connection) { c.Scope, c.Owner = connections.ScopePerson, "alice" },
		"cert without ref": func(c *connections.Connection) { c.EntraAuth = connections.EntraCertificate },
		"mi with a key":    func(c *connections.Connection) { c.EntraAuth = connections.EntraManagedIdentity },
		"aws field":        func(c *connections.Connection) { c.Region = "eu-west-1" },
		"plain http":       func(c *connections.Connection) { c.BaseURL = "http://login.example.com" },
		"user with auth": func(c *connections.Connection) {
			c.Kind, c.Scope, c.Owner = connections.KindEntraUser, connections.ScopePerson, "alice"
		},
		"github with tenant": func(c *connections.Connection) {
			c.Kind, c.EntraAuth, c.KeyRef, c.TokenRef = connections.KindGitHubToken, "", "", "env:T"
		},
	} {
		c := base
		mod(&c)
		if err := c.Validate(); !errors.Is(err, connections.ErrInvalid) {
			t.Errorf("%s: Validate = %v, want ErrInvalid", name, err)
		}
	}
	mi := connections.Connection{ID: "mi", Kind: connections.KindEntra, Scope: connections.ScopeServer, EntraAuth: connections.EntraManagedIdentity}
	if err := mi.Validate(); err != nil {
		t.Errorf("Validate(system-assigned managed identity) = %v", err)
	}
	user := connections.Connection{ID: "od", Kind: connections.KindEntraUser, Scope: connections.ScopePerson, Owner: "alice", TenantID: tenant, ClientID: clientID}
	if err := user.Validate(); err != nil {
		t.Errorf("Validate(public-client user) = %v", err)
	}
}

func writeFile(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
