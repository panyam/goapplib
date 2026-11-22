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
func entraFixture(t *testing.T, opts ...func(*entratest.Server) connections.Option) (*entratest.Server, *fsstore.Store, *connections.Service) {
	t.Helper()
	ent := entratest.New(t)
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOAPPLIB_TEST_CONN_KEY", masterKey)
	all := []connections.Option{
		connections.WithUserTokens(oneauthfs.NewFSUserTokenStore(t.TempDir()), "env:GOAPPLIB_TEST_CONN_KEY", nil),
		connections.WithManagedIdentityURL(ent.ManagedIdentityURL())}
	for _, o := range opts {
		all = append(all, o(ent))
	}
	svc := connections.NewService(store, all...)
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

func TestEntraManagedIdentityUsesTheAppServiceEndpoint(t *testing.T) {
	ent, store, svc := entraFixture(t, func(e *entratest.Server) connections.Option {
		return connections.WithIdentityEndpoint(e.IdentityEndpoint())
	})
	ent.AddManagedIdentity("")
	putConn(t, store, connections.Connection{ID: "mi", Kind: connections.KindEntra, Scope: connections.ScopeServer,
		EntraAuth: connections.EntraManagedIdentity})
	creds, err := svc.Credentials(context.Background(), "", "mi")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := mounts.TokenSourceOf(creds)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if !ent.ValidToken(tok.AccessToken) {
		t.Errorf("token %q wasn't issued by the fake", tok.AccessToken)
	}
	// App Service sends expires_on, an epoch time, and no expires_in.
	if left := time.Until(tok.Expiry); left < 50*time.Minute || left > 61*time.Minute {
		t.Errorf("token expires in %v; want about an hour, from expires_on", left)
	}
	_, header := ent.IdentityEndpoint()
	reqs := ent.Requests()
	if len(reqs) != 1 || reqs[0].Path != "/msi/token" || reqs[0].IdentityHeader != header || reqs[0].Scope != connections.GraphResource {
		t.Errorf("requests = %+v; want one identity endpoint request for Graph with the host's header", reqs)
	}
}

func TestEntraIdentityEndpointNeedsTheHostsHeader(t *testing.T) {
	ent, store, svc := entraFixture(t, func(e *entratest.Server) connections.Option {
		endpoint, _ := e.IdentityEndpoint()
		return connections.WithIdentityEndpoint(endpoint, "not-the-header")
	})
	ent.AddManagedIdentity("")
	putConn(t, store, connections.Connection{ID: "mi", Kind: connections.KindEntra, Scope: connections.ScopeServer,
		EntraAuth: connections.EntraManagedIdentity})
	creds, err := svc.Credentials(context.Background(), "", "mi")
	if err == nil {
		_, err = mounts.BearerToken(creds)
	}
	if err == nil || !strings.Contains(err.Error(), "X-IDENTITY-HEADER") {
		t.Errorf("token with the wrong header: %v; want the endpoint's refusal", err)
	}
}

func TestEntraWorkloadIdentitySendsTheFederatedToken(t *testing.T) {
	ent, store, svc := entraFixture(t)
	const sa = "system:serviceaccount:agni:web"
	ent.AddFederatedApp(tenant, clientID, sa)
	tokenFile := filepath.Join(t.TempDir(), "azure-identity-token")
	writeFile(t, os.WriteFile(tokenFile, []byte(ent.FederatedToken(sa, time.Now().Add(time.Hour))+"\n"), 0o600))
	putConn(t, store, connections.Connection{ID: "wi", Kind: connections.KindEntra, Scope: connections.ScopeServer, BaseURL: ent.URL,
		TenantID: tenant, ClientID: clientID, EntraAuth: connections.EntraWorkloadIdentity, KeyRef: "file:" + tokenFile})
	if tok := entraToken(t, svc, context.Background(), "", "wi"); !ent.ValidToken(tok) {
		t.Errorf("token %q wasn't issued by the fake", tok)
	}
	reqs := ent.Requests()
	if len(reqs) != 1 || reqs[0].GrantType != "client_credentials" || !reqs[0].Assertion || reqs[0].Scope != connections.GraphScope {
		t.Errorf("requests = %+v; want one client-credentials request with a client assertion", reqs)
	}
}

func TestEntraWorkloadIdentityReadsTheRotatedFile(t *testing.T) {
	ent, store, svc := entraFixture(t)
	const sa = "system:serviceaccount:agni:web"
	ent.AddFederatedApp(tenant, clientID, sa)
	tokenFile := filepath.Join(t.TempDir(), "azure-identity-token")
	writeFile(t, os.WriteFile(tokenFile, []byte(ent.FederatedToken(sa, time.Now().Add(-time.Minute))), 0o600))
	putConn(t, store, connections.Connection{ID: "wi", Kind: connections.KindEntra, Scope: connections.ScopeServer, BaseURL: ent.URL,
		TenantID: tenant, ClientID: clientID, EntraAuth: connections.EntraWorkloadIdentity, KeyRef: "file:" + tokenFile})
	token := func() (string, error) {
		creds, err := svc.Credentials(context.Background(), "", "wi")
		if err != nil {
			return "", err
		}
		return mounts.BearerToken(creds)
	}
	if _, err := token(); err == nil || !strings.Contains(err.Error(), "AADSTS70021") {
		t.Errorf("token from an expired federated token: %v; want Entra's AADSTS70021 refusal", err)
	}
	// The cluster rotates the file. The same record reads the new token.
	writeFile(t, os.WriteFile(tokenFile, []byte(ent.FederatedToken(sa, time.Now().Add(time.Hour))), 0o600))
	if tok, err := token(); err != nil || !ent.ValidToken(tok) {
		t.Errorf("token after the file rotated = %q, %v; want one from the fake", tok, err)
	}
	// A token for another service account doesn't match the federated credential.
	writeFile(t, os.WriteFile(tokenFile, []byte(ent.FederatedToken("system:serviceaccount:agni:other", time.Now().Add(time.Hour))), 0o600))
	putConn(t, store, connections.Connection{ID: "wi2", Kind: connections.KindEntra, Scope: connections.ScopeServer, BaseURL: ent.URL,
		TenantID: tenant, ClientID: clientID, EntraAuth: connections.EntraWorkloadIdentity, KeyRef: "file:" + tokenFile})
	creds, err := svc.Credentials(context.Background(), "", "wi2")
	if err == nil {
		_, err = mounts.BearerToken(creds)
	}
	if err == nil || !strings.Contains(err.Error(), "AADSTS70021") {
		t.Errorf("token for another service account: %v; want Entra's AADSTS70021 refusal", err)
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
		"wi with env ref":  func(c *connections.Connection) { c.EntraAuth = connections.EntraWorkloadIdentity },
		"wi without key": func(c *connections.Connection) {
			c.EntraAuth, c.KeyRef = connections.EntraWorkloadIdentity, ""
		},
		"wi with cert": func(c *connections.Connection) {
			c.EntraAuth, c.KeyRef, c.CertRef = connections.EntraWorkloadIdentity, "file:/t", "file:/c"
		},
		"wi without client": func(c *connections.Connection) {
			c.EntraAuth, c.KeyRef, c.ClientID = connections.EntraWorkloadIdentity, "file:/t", ""
		},
		"aws field":  func(c *connections.Connection) { c.Region = "eu-west-1" },
		"plain http": func(c *connections.Connection) { c.BaseURL = "http://login.example.com" },
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
	wi := base
	wi.EntraAuth, wi.KeyRef = connections.EntraWorkloadIdentity, "file:/var/run/secrets/azure/tokens/azure-identity-token"
	if err := wi.Validate(); err != nil {
		t.Errorf("Validate(workload identity) = %v", err)
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
