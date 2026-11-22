package connections

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- Entra names a certificate by its SHA-1 thumbprint in the x5t header
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/panyam/oneauth/outbound"
	"golang.org/x/oauth2"
)

const (
	// DefaultEntraAuthority is the Entra authority host for public Azure. A KindEntra or
	// KindEntraUser connection's BaseURL overrides it, for a national cloud or a test's fake.
	DefaultEntraAuthority = "https://login.microsoftonline.com"

	// GraphScope is the scope a KindEntra connection asks for: every Microsoft Graph application
	// permission an admin granted the app registration.
	GraphScope = "https://graph.microsoft.com/.default"

	// GraphUserScope is the scope a KindEntraUser refresh asks for: the Graph permissions the
	// person consented to, and a new refresh token.
	GraphUserScope = "https://graph.microsoft.com/.default offline_access"

	// GraphResource is the resource a managed identity token is for.
	GraphResource = "https://graph.microsoft.com"

	// DefaultManagedIdentityURL is the Azure Instance Metadata Service's token endpoint, which a
	// VM, a scale set or an AKS node answers for its managed identity. WithManagedIdentityURL
	// overrides it.
	DefaultManagedIdentityURL = "http://169.254.169.254/metadata/identity/oauth2/token"

	clientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

	// assertionLife is how long a client assertion is good for. Entra refuses one that lives
	// longer than ten minutes.
	assertionLife = 5 * time.Minute
)

// WithManagedIdentityURL points KindEntra EntraManagedIdentity connections at another token
// endpoint than DefaultManagedIdentityURL, such as a test's fake. It belongs to the service, not
// the record, so a stored record can never send the host's identity token somewhere else.
func WithManagedIdentityURL(u string) Option {
	return func(s *Service) { s.miURL = u }
}

// WithIdentityEndpoint points KindEntra EntraManagedIdentity connections at an App Service or
// Container Apps identity endpoint, in place of IMDS. Such a host sets the endpoint in
// IDENTITY_ENDPOINT and the secret header in IDENTITY_HEADER, and the app passes both:
//
//	connections.WithIdentityEndpoint(os.Getenv("IDENTITY_ENDPOINT"), os.Getenv("IDENTITY_HEADER"))
//
// The service sends the header as X-IDENTITY-HEADER on each token request. Like
// WithManagedIdentityURL, it belongs to the service, so a stored record can't name another
// endpoint. An empty endpoint leaves IMDS in place.
func WithIdentityEndpoint(endpoint, header string) Option {
	return func(s *Service) { s.idURL, s.idHeader = endpoint, header }
}

func entraAuthority(c Connection) string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return DefaultEntraAuthority
}

// entraTokenURL is the v2 token endpoint of c's tenant.
func entraTokenURL(c Connection) string {
	return entraAuthority(c) + "/" + url.PathEscape(c.TenantID) + "/oauth2/v2.0/token"
}

// entraEndpoint is c's tenant's v2 OAuth endpoint, for an app's sign-in and for refreshes.
func entraEndpoint(c Connection) oauth2.Endpoint {
	base := entraAuthority(c) + "/" + url.PathEscape(c.TenantID) + "/oauth2/v2.0"
	return oauth2.Endpoint{AuthURL: base + "/authorize", TokenURL: base + "/token", AuthStyle: oauth2.AuthStyleInParams}
}

// EntraEndpoint returns the OAuth endpoint of c's tenant, which an app's sign-in for a
// KindEntraUser connection uses, so its tokens come from the same authority the service refreshes
// them at. c must be a KindEntraUser connection.
func EntraEndpoint(c Connection) oauth2.Endpoint { return entraEndpoint(c) }

// entraTokenSource builds a KindEntra connection's token source. Secrets are read once here, and
// the service rebuilds the source when the record's sign-in fields change.
func (s *Service) entraTokenSource(c Connection, secret func(field, ref string) (string, error)) (oauth2.TokenSource, error) {
	es := &entraSource{c: c, client: s.client}
	switch c.EntraAuth {
	case EntraSecret:
		v, err := secret("key_ref", c.KeyRef)
		if err != nil {
			return nil, err
		}
		es.secret = v
	case EntraCertificate:
		keyPEM, err := secret("key_ref", c.KeyRef)
		if err != nil {
			return nil, err
		}
		certPEM, err := secret("cert_ref", c.CertRef)
		if err != nil {
			return nil, err
		}
		if es.key, es.x5t, err = parseCertKey([]byte(keyPEM), []byte(certPEM)); err != nil {
			return nil, fmt.Errorf("connections: %s: %w", c.ID, err)
		}
	case EntraManagedIdentity:
		es.miURL = s.miURL
		if s.idURL != "" {
			es.miURL, es.idHeader = s.idURL, s.idHeader
		}
		if es.miURL == "" {
			es.miURL = DefaultManagedIdentityURL
		}
	case EntraWorkloadIdentity:
		r, err := outbound.ParseSecretRef(c.KeyRef)
		if err != nil {
			return nil, fmt.Errorf("%w: key_ref", ErrInvalid)
		}
		es.federated = r
	default:
		return nil, fmt.Errorf("%w: unknown entra_auth %q", ErrInvalid, c.EntraAuth)
	}
	return oauth2.ReuseTokenSource(nil, es), nil
}

func resolve(c Connection, field, ref string) (string, error) {
	r, err := outbound.ParseSecretRef(ref)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalid, field)
	}
	v, err := r.Resolve()
	if err != nil {
		return "", fmt.Errorf("connections: %s: %s: %w", c.ID, field, err)
	}
	return v, nil
}

// parseCertKey reads a PEM private key and a PEM certificate, checks that they belong together,
// and returns the key with the certificate's x5t: the unpadded base64url SHA-1 of its DER.
func parseCertKey(keyPEM, certPEM []byte) (crypto.Signer, string, error) {
	cb, _ := pem.Decode(certPEM)
	if cb == nil || cb.Type != "CERTIFICATE" {
		return nil, "", fmt.Errorf("%w: cert_ref holds no PEM certificate", ErrInvalid)
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("%w: cert_ref: %v", ErrInvalid, err)
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil {
		return nil, "", fmt.Errorf("%w: key_ref holds no PEM key", ErrInvalid)
	}
	var key any
	switch kb.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(kb.Bytes)
	default:
		key, err = x509.ParsePKCS8PrivateKey(kb.Bytes)
	}
	if err != nil {
		return nil, "", fmt.Errorf("%w: key_ref: %v", ErrInvalid, err)
	}
	// Entra checks certificate assertions with RS256.
	signer, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, "", fmt.Errorf("%w: key_ref must be an RSA key", ErrInvalid)
	}
	if !signer.PublicKey.Equal(cert.PublicKey) {
		return nil, "", fmt.Errorf("%w: the key at key_ref doesn't match the certificate at cert_ref", ErrInvalid)
	}
	sum := sha1.Sum(cert.Raw) // #nosec G401 -- the thumbprint format Entra's x5t header uses
	return signer, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// entraSource fetches one token per call. oauth2.ReuseTokenSource in front of it keeps the token
// until it expires.
type entraSource struct {
	c      Connection
	client *http.Client

	secret string
	key    crypto.Signer
	x5t    string
	miURL  string

	// idHeader is set when miURL is an App Service identity endpoint.
	idHeader string

	// federated is a workload identity's token file, read again for each request since the
	// cluster rotates it.
	federated outbound.SecretRef
}

func (e *entraSource) httpClient() *http.Client {
	if e.client != nil {
		return e.client
	}
	return http.DefaultClient
}

func (e *entraSource) Token() (*oauth2.Token, error) {
	if e.miURL != "" {
		return e.managedIdentity()
	}
	form := url.Values{
		"grant_type": {"client_credentials"},
		"client_id":  {e.c.ClientID},
		"scope":      {GraphScope},
	}
	tokenURL := entraTokenURL(e.c)
	switch {
	case !e.federated.IsZero():
		a, err := e.federated.Resolve()
		if err != nil {
			return nil, fmt.Errorf("connections: %s: key_ref: %w", e.c.ID, err)
		}
		form.Set("client_assertion_type", clientAssertionType)
		form.Set("client_assertion", strings.TrimSpace(a))
	case e.key != nil:
		a, err := e.assertion(tokenURL)
		if err != nil {
			return nil, err
		}
		form.Set("client_assertion_type", clientAssertionType)
		form.Set("client_assertion", a)
	default:
		form.Set("client_secret", e.secret)
	}
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return e.do(req)
}

// assertion signs a client assertion for aud, the token endpoint, as RFC 7523 and Entra's
// certificate credential docs describe: iss and sub are the client ID, jti is fresh each time,
// and the x5t header names the certificate.
func (e *entraSource) assertion(aud string) (string, error) {
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    e.c.ClientID,
		Subject:   e.c.ClientID,
		Audience:  jwt.ClaimStrings{aud},
		ID:        hex.EncodeToString(jti),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(assertionLife)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["x5t"] = e.x5t
	return tok.SignedString(e.key)
}

// managedIdentity asks the host's identity endpoint for a Graph token. IMDS answers only a
// request with "Metadata: true", which a forwarded browser request can't carry. An App Service
// identity endpoint wants its secret header and a newer api-version instead.
func (e *entraSource) managedIdentity() (*oauth2.Token, error) {
	q := url.Values{"api-version": {"2018-02-01"}, "resource": {GraphResource}}
	if e.idHeader != "" {
		q.Set("api-version", "2019-08-01")
	}
	if e.c.ClientID != "" {
		q.Set("client_id", e.c.ClientID)
	}
	req, err := http.NewRequest(http.MethodGet, e.miURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if e.idHeader != "" {
		req.Header.Set("X-IDENTITY-HEADER", e.idHeader)
	} else {
		req.Header.Set("Metadata", "true")
	}
	return e.do(req)
}

// do sends a token request and reads an Entra, IMDS or App Service token answer. IMDS sends
// expires_in as a string, and the v2 endpoint as a number, so both are read. App Service sends
// only expires_on, an epoch time, which is read when expires_in is missing.
func (e *entraSource) do(req *http.Request) (*oauth2.Token, error) {
	resp, err := e.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("connections: %s: entra token: %w", e.c.ID, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("connections: %s: entra token: %w", e.c.ID, err)
	}
	var out struct {
		AccessToken string          `json:"access_token"`
		TokenType   string          `json:"token_type"`
		ExpiresIn   json.RawMessage `json:"expires_in"`
		ExpiresOn   json.RawMessage `json:"expires_on"`
		Error       string          `json:"error"`
		Description string          `json:"error_description"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("connections: %s: entra token: status %d, not JSON", e.c.ID, resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 || out.AccessToken == "" {
		return nil, fmt.Errorf("connections: %s: entra token: status %d: %s %s", e.c.ID, resp.StatusCode, out.Error, firstLine(out.Description))
	}
	tok := &oauth2.Token{AccessToken: out.AccessToken, TokenType: out.TokenType}
	secs, err := strconv.ParseInt(strings.Trim(string(out.ExpiresIn), `"`), 10, 64)
	if err == nil && secs > 0 {
		tok.Expiry = time.Now().Add(time.Duration(secs) * time.Second)
	} else if on, err := strconv.ParseInt(strings.Trim(string(out.ExpiresOn), `"`), 10, 64); err == nil && on > 0 {
		tok.Expiry = time.Unix(on, 0)
	}
	return tok, nil
}

// firstLine keeps an Entra error description to its first line, which names the error code
// without the trace and correlation IDs that follow.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// entraUserConfig is the OAuth config oneauth refreshes a KindEntraUser person's token with: the
// tenant's token endpoint, the client ID, and the client secret at KeyRef for a confidential
// client.
func entraUserConfig(c Connection) (*oauth2.Config, error) {
	cfg := &oauth2.Config{ClientID: c.ClientID, Endpoint: entraEndpoint(c), Scopes: strings.Fields(GraphUserScope)}
	if c.KeyRef != "" {
		secret, err := resolve(c, "key_ref", c.KeyRef)
		if err != nil {
			return nil, err
		}
		cfg.ClientSecret = secret
	}
	return cfg, nil
}

// scopeOnRefresh adds GraphUserScope to each refresh-token request to tokenURL. x/oauth2 sends no
// scope on a refresh, and Entra's v2 endpoint expects one.
type scopeOnRefresh struct {
	next     http.RoundTripper
	tokenURL string
}

func (s scopeOnRefresh) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodPost || r.URL.String() != s.tokenURL || r.Body == nil {
		return s.next.RoundTrip(r)
	}
	b, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(b))
	if err == nil && form.Get("grant_type") == "refresh_token" && form.Get("scope") == "" {
		form.Set("scope", GraphUserScope)
		b = []byte(form.Encode())
	}
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(strings.NewReader(string(b)))
	r2.ContentLength = int64(len(b))
	return s.next.RoundTrip(r2)
}

// entraUserClient is the HTTP client a KindEntraUser connection refreshes through.
func entraUserClient(base *http.Client, c Connection) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	next := base.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	out := *base
	out.Transport = scopeOnRefresh{next: next, tokenURL: entraEndpoint(c).TokenURL}
	return &out
}
