// Package entratest is a fake Microsoft Entra ID token service for tests of KindEntra and
// KindEntraUser connections. It serves a tenant's v2 token endpoint at
// "/{tenant}/oauth2/v2.0/token" for the client-credentials grant, with a client secret or a
// certificate's client assertion, and for the refresh-token grant. It also serves an Azure
// Instance Metadata Service managed identity endpoint at ManagedIdentityURL.
//
// It checks what Entra checks: a client assertion must be an RS256 JWT signed by the registered
// certificate's key, with that certificate's x5t thumbprint in its header, the token endpoint as
// its aud, the client ID as iss and sub, a fresh jti, and a lifetime of at most ten minutes. Every
// access token it issues is recorded, so a fake Graph can trust them with ValidToken.
package entratest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Server is the fake. Its URL is the authority a connection's BaseURL names.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	apps     map[string]*app // by tenant and client ID
	mi       map[string]bool // accepted managed identity client IDs, "" for system-assigned
	refresh  map[string]grant
	issued   map[string]string // access token -> who it was issued to
	jtis     map[string]bool
	requests []Request
	seq      int
}

type app struct {
	secret string
	cert   *x509.Certificate
}

type grant struct {
	tenant, client, subject string
}

// Request is one token request the fake answered.
type Request struct {
	Path      string
	GrantType string
	ClientID  string
	Scope     string

	// Assertion is set when the request carried a client assertion.
	Assertion bool

	// Metadata is the "Metadata" header, which the managed identity endpoint needs.
	Metadata string
}

// New starts a fake and stops it when the test ends.
func New(t testing.TB) *Server {
	s := &Server{apps: map[string]*app{}, mi: map[string]bool{}, refresh: map[string]grant{},
		issued: map[string]string{}, jtis: map[string]bool{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// ManagedIdentityURL is the fake's managed identity endpoint, for
// connections.WithManagedIdentityURL.
func (s *Server) ManagedIdentityURL() string { return s.URL + "/metadata/identity/oauth2/token" }

// TokenURL is tenant's token endpoint, which a client assertion's aud must name.
func (s *Server) TokenURL(tenant string) string { return s.URL + "/" + tenant + "/oauth2/v2.0/token" }

// AddSecretApp registers an app in tenant that signs in with secret.
func (s *Server) AddSecretApp(tenant, clientID, secret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apps[tenant+"/"+clientID] = &app{secret: secret}
}

// AddCertApp registers an app in tenant that signs in with a client assertion signed by cert's
// key.
func (s *Server) AddCertApp(tenant, clientID string, cert *x509.Certificate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apps[tenant+"/"+clientID] = &app{cert: cert}
}

// AddManagedIdentity makes the managed identity endpoint answer for clientID, or for the
// system-assigned identity when clientID is "".
func (s *Server) AddManagedIdentity(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mi[clientID] = true
}

// SignIn stands in for a person signing in to the app clientID in tenant, and returns the token
// the app's OAuth callback would get. The app must be registered first.
func (s *Server) SignIn(tenant, clientID, subject string) *Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.issueLocked(grant{tenant, clientID, subject}, true)
}

// Token is what the fake issues.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// ValidToken reports whether tok is an access token the fake issued.
func (s *Server) ValidToken(tok string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.issued[tok]
	return ok
}

// Requests returns every request answered so far, oldest first.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func (s *Server) issueLocked(g grant, withRefresh bool) *Token {
	s.seq++
	tok := &Token{AccessToken: "entra-at-" + hex.EncodeToString([]byte{byte(s.seq)}) + "-" + randHex(), TokenType: "Bearer", ExpiresIn: 3600}
	s.issued[tok.AccessToken] = g.tenant + "/" + g.client + "/" + g.subject
	if withRefresh {
		tok.RefreshToken = "entra-rt-" + randHex()
		s.refresh[tok.RefreshToken] = g
	}
	return tok
}

func randHex() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/metadata/identity/oauth2/token" {
		s.managedIdentity(w, r)
		return
	}
	tenant, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/oauth2/v2.0/token")
	if !ok || strings.Contains(tenant, "/") || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "bad form")
		return
	}
	f := r.PostForm
	s.mu.Lock()
	s.requests = append(s.requests, Request{Path: r.URL.Path, GrantType: f.Get("grant_type"), ClientID: f.Get("client_id"),
		Scope: f.Get("scope"), Assertion: f.Get("client_assertion") != ""})
	a := s.apps[tenant+"/"+f.Get("client_id")]
	s.mu.Unlock()
	if a == nil {
		oauthError(w, http.StatusBadRequest, "unauthorized_client", "AADSTS700016: application not found in the directory")
		return
	}
	if f.Get("scope") == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "AADSTS90014: the request body must contain the parameter 'scope'")
		return
	}
	if msg := s.checkClient(a, f.Get("client_secret"), f.Get("client_assertion_type"), f.Get("client_assertion"), s.TokenURL(tenant), f.Get("client_id")); msg != "" {
		oauthError(w, http.StatusUnauthorized, "invalid_client", msg)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch f.Get("grant_type") {
	case "client_credentials":
		writeJSON(w, http.StatusOK, s.issueLocked(grant{tenant, f.Get("client_id"), ""}, false))
	case "refresh_token":
		g, ok := s.refresh[f.Get("refresh_token")]
		if !ok || g.tenant != tenant || g.client != f.Get("client_id") {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "AADSTS70000: the refresh token is invalid")
			return
		}
		delete(s.refresh, f.Get("refresh_token"))
		writeJSON(w, http.StatusOK, s.issueLocked(g, true))
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "")
	}
}

// checkClient returns "" when the request authenticates as a, or Entra's reason otherwise. An app
// registered with neither a secret nor a certificate is a public client.
func (s *Server) checkClient(a *app, secret, assertionType, assertion, aud, clientID string) string {
	switch {
	case a.cert != nil:
		if assertionType != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" || assertion == "" {
			return "AADSTS7000218: the request body must contain client_assertion"
		}
		return s.checkAssertion(a.cert, assertion, aud, clientID)
	case a.secret != "":
		if secret != a.secret {
			return "AADSTS7000215: invalid client secret provided"
		}
	}
	return ""
}

func (s *Server) checkAssertion(cert *x509.Certificate, assertion, aud, clientID string) string {
	var claims jwt.RegisteredClaims
	tok, err := jwt.ParseWithClaims(assertion, &claims, func(t *jwt.Token) (any, error) {
		return cert.PublicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithAudience(aud), jwt.WithIssuer(clientID), jwt.WithSubject(clientID), jwt.WithExpirationRequired())
	if err != nil {
		return "AADSTS700027: client assertion failed signature or claim validation: " + err.Error()
	}
	if x5t, _ := tok.Header["x5t"].(string); x5t != Thumbprint(cert) {
		return "AADSTS700027: the x5t header doesn't name a registered certificate"
	}
	if claims.ID == "" || claims.ExpiresAt == nil || claims.NotBefore == nil || claims.ExpiresAt.Sub(claims.NotBefore.Time) > 10*time.Minute {
		return "AADSTS700024: client assertion needs a jti and a lifetime of at most ten minutes"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jtis[claims.ID] {
		return "AADSTS700021: client assertion jti was already used"
	}
	s.jtis[claims.ID] = true
	return ""
}

func (s *Server) managedIdentity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.mu.Lock()
	s.requests = append(s.requests, Request{Path: r.URL.Path, ClientID: q.Get("client_id"), Scope: q.Get("resource"), Metadata: r.Header.Get("Metadata")})
	ok := s.mi[q.Get("client_id")]
	s.mu.Unlock()
	if r.Header.Get("Metadata") != "true" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Required metadata header not specified")
		return
	}
	if !ok || q.Get("resource") == "" || q.Get("api-version") == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Identity not found")
		return
	}
	s.mu.Lock()
	tok := s.issueLocked(grant{"imds", q.Get("client_id"), ""}, false)
	s.mu.Unlock()
	// IMDS sends expires_in as a string.
	writeJSON(w, http.StatusOK, map[string]string{"access_token": tok.AccessToken, "token_type": "Bearer",
		"expires_in": "3599", "resource": q.Get("resource")})
}

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc + "\r\nTrace ID: 0000\r\nCorrelation ID: 0000"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Thumbprint is cert's x5t: the unpadded base64url SHA-1 of its DER.
func Thumbprint(cert *x509.Certificate) string {
	sum := sha1.Sum(cert.Raw) // #nosec G401 -- the thumbprint format Entra's x5t header uses
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// NewCertificate makes a self-signed RSA certificate for an app registration, and returns its
// private key and certificate as PEM, as they would sit in the secret references, and the parsed
// certificate for AddCertApp.
func NewCertificate(t testing.TB) (keyPEM, certPEM []byte, cert *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "goapplib test app"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return keyPEM, certPEM, cert
}
