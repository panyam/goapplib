package connections

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/oneauth/outbound"
	"golang.org/x/oauth2"
)

// LastUsedEvery is how often the service writes a connection's LastUsed. A token request inside
// that window since the last write doesn't touch the record, so reading a token doesn't cost a
// store write on every request.
const LastUsedEvery = time.Hour

// Service hands out credentials for connections, and connects and disconnects people. It reads
// records from a Store and builds each connection's token source from oneauth's outbound package:
//
//   - KindGitHubApp: one outbound.GitHubAppInstallation per connection, kept while the record is
//     unchanged, so every mount on the connection shares its cached installation token.
//   - KindGitHubToken: outbound.NewStatic over TokenRef.
//   - KindGitHubUser: oneauth's outbound.UserTokens under UserTokenProvider(ID) and the owner's
//     subject. It needs WithUserTokens.
//   - KindGCP: a Google token source from the service account key at KeyRef, or Application
//     Default Credentials when KeyRef is empty.
//   - KindAWS: an AWS key pair, or the AWS default chain, which the S3 driver signs with.
//   - KindEntra: a Microsoft Graph token from the tenant's token endpoint, for a client secret, a
//     certificate or a workload identity's federated token, or from the host's managed identity
//     endpoint.
//   - KindEntraUser: oneauth's outbound.UserTokens, as for KindGitHubUser, refreshed at the
//     tenant's token endpoint. It needs WithUserTokens for the store and the master key.
//
// A Service is safe for concurrent use. Several processes can share one Store and one user-token
// store, since oneauth refreshes a person's token with compare-and-swap.
type Service struct {
	store Store
	users outbound.UserTokenStore
	// keyRef is the master key's secret reference, resolved on first use.
	keyRef string
	oauth  *oauth2.Config
	client *http.Client
	now    func() time.Time
	miURL  string
	// idURL and idHeader are an App Service identity endpoint and its secret header.
	idURL, idHeader string

	check     ReadCheck
	accessTTL time.Duration

	mu sync.Mutex
	// entries is keyed by connKey, since two owners can each have a connection with one ID.
	entries map[string]*entry
	key     string
	access  map[string]answer
	// gen counts each viewer's forgets, so a CanRead that began before one doesn't keep its
	// answer.
	gen map[string]uint64
}

// entry is one connection's built token source, kept while its record's sign-in fields match.
type entry struct {
	conn  Connection
	ts    oauth2.TokenSource
	users *outbound.UserTokens

	// failed is set when this process recorded a failure on the record, so the next success
	// clears it without a store read on every token.
	failed atomic.Bool
}

// Option configures a Service.
type Option func(*Service)

// NewService returns a service over store. Without WithUserTokens, a KindGitHubUser or
// KindEntraUser connection returns ErrNotConfigured.
func NewService(store Store, opts ...Option) *Service {
	s := &Service{store: store, now: time.Now, entries: map[string]*entry{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

// WithUserTokens lets the service hold people's own tokens. store keeps them sealed with a key
// derived from the master key, which masterKeyRef names as a secret reference to 64 hex
// characters ("env:NAME" or "file:/abs/path"). oauth is the GitHub OAuth app's config, which
// refreshes a KindGitHubUser token and whose client ID and secret revoke one. A KindEntraUser
// connection builds its own config from its record, so it uses only store and the master key, and
// oauth may be nil when the service holds no GitHub users.
func WithUserTokens(store outbound.UserTokenStore, masterKeyRef string, oauth *oauth2.Config) Option {
	return func(s *Service) { s.users, s.keyRef, s.oauth = store, masterKeyRef, oauth }
}

// WithHTTPClient sets the client for calls to GitHub: installation tokens, refreshes and revokes.
// Nil means http.DefaultClient.
func WithHTTPClient(c *http.Client) Option {
	return func(s *Service) { s.client = c }
}

// WithClock sets the service's clock, which stamps LastUsed. Tests use it.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// Credentials returns how owner's connection id signs in, as mounts.Credentials. An empty owner
// names a server connection. The signature fits the mounts registry's WithCredentials option.
//
// The GitHub kinds, KindGCP and the Entra kinds return mounts.BearerCredentials. KindAWS returns
// mounts.AWSCredentials with its secret resolved from KeyRef, or with DefaultChain set when the
// record names no key.
//
// A person's connection gives credentials only when Viewer(ctx) is owner. Otherwise Credentials
// returns ErrNotOwner without reading the store, so the error says nothing about whether owner
// has a connection with that ID. A request with no viewer gets ErrNotOwner too. A missing
// connection returns ErrNotFound. A user connection without WithUserTokens returns
// ErrNotConfigured.
//
// A bearer source's Token returns ErrReconnect, wrapping outbound.ErrReconnect, when a
// person's token can't be refreshed, and sets the record's Status to StatusReconnect. Any other
// failure sets StatusFailed. LastError gets the message, which never holds a token. The next
// token that works sets Status back to StatusOK.
//
// Credentials also stamps the record's LastUsed, at most once per LastUsedEvery.
func (s *Service) Credentials(ctx context.Context, owner, id string) (mounts.Credentials, error) {
	if owner != "" && Viewer(ctx) != owner {
		return nil, fmt.Errorf("%w: %q", ErrNotOwner, id)
	}
	c, err := s.store.GetConnection(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	e, err := s.entry(c)
	if err != nil {
		return nil, err
	}
	if s.now().Sub(c.LastUsed) >= LastUsedEvery {
		c.LastUsed = s.now()
		if err := s.store.PutConnection(ctx, c); err != nil {
			return nil, err
		}
	}
	if c.Kind == KindAWS {
		return awsCredentials(c)
	}
	if c.Status != StatusOK {
		e.failed.Store(true)
	}
	return mounts.Bearer(&statusSource{svc: s, owner: owner, id: id, e: e}), nil
}

// SaveUserToken keeps tok as the token of owner's KindGitHubUser or KindEntraUser connection id,
// sealed, and sets
// the record's status back to StatusOK. The app calls it from its OAuth callback after a person
// connects, with the viewer set to owner. Any other viewer gets ErrNotOwner. The token must come from the app's own OAuth
// flow, since Disconnect revokes it through the app. A pasted token goes through SavePastedToken.
// Once the token is saved, CanRead forgets the owner's kept answers, so a no from before the
// person connected doesn't outlive the connection.
func (s *Service) SaveUserToken(ctx context.Context, owner, id string, tok *oauth2.Token) error {
	return s.saveToken(ctx, owner, id, tok, false)
}

// SavePastedToken keeps tok as the token of owner's KindGitHubUser connection id, as SaveUserToken does,
// for a token the person pasted rather than one the app's OAuth flow issued. It sets the record's
// Pasted, so Disconnect deletes the token without asking GitHub to revoke it. GitHub refuses that
// revoke for a token the app didn't issue. A later SaveUserToken clears Pasted.
func (s *Service) SavePastedToken(ctx context.Context, owner, id string, tok *oauth2.Token) error {
	return s.saveToken(ctx, owner, id, tok, true)
}

// saveToken writes the sealed token and the record's Pasted in an order that leaves the record
// saying the app issued the token when the second write fails. Disconnect then tries the revoke
// and returns its error, rather than skip the revoke of a live app token.
func (s *Service) saveToken(ctx context.Context, owner, id string, tok *oauth2.Token, pasted bool) error {
	if owner == "" || Viewer(ctx) != owner {
		return fmt.Errorf("%w: %q", ErrNotOwner, id)
	}
	c, err := s.store.GetConnection(ctx, owner, id)
	if err != nil {
		return err
	}
	if !c.Kind.holdsUserToken() {
		return fmt.Errorf("%w: %q is a %s connection, which has no stored user token", ErrInvalid, id, c.Kind)
	}
	e, err := s.entry(c)
	if err != nil {
		return err
	}
	if c.Pasted && !pasted {
		c.Pasted = false
		if err := s.store.PutConnection(ctx, c); err != nil {
			return err
		}
	}
	if err := e.users.Save(ctx, c.Owner, tok); err != nil {
		return err
	}
	s.ForgetAccess(c.Owner)
	if c.Status != StatusOK || c.LastError != "" || c.Pasted != pasted {
		c.Status, c.LastError, c.Pasted = StatusOK, "", pasted
		return s.store.PutConnection(ctx, c)
	}
	return nil
}

// Disconnect removes owner's connection id, where an empty owner names a server connection. It
// doesn't check the viewer, so the app decides who may disconnect what. For a KindGitHubUser connection it first revokes the token at
// GitHub and deletes it from the user-token store, through oneauth's UserTokens.Revoke. When the
// revoke fails it returns the error and keeps both the token and the record, so the app can
// retry. When the record's Pasted is set, it deletes the token with no call to GitHub, since the
// app didn't issue it and can't revoke it. Other kinds have nothing to revoke here: an App
// installation is removed on GitHub, and a fixed token by whoever issued it. A KindEntraUser token
// is deleted with no call to Entra, since Entra has no endpoint that revokes one token; the person
// removes the app's consent in their account to end it there. A missing connection
// is not an error. Once the record is gone, CanRead forgets the owner's kept answers.
func (s *Service) Disconnect(ctx context.Context, owner, id string) error {
	c, err := s.store.GetConnection(ctx, owner, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if c.Kind.holdsUserToken() {
		e, err := s.entry(c)
		if err != nil {
			return err
		}
		revoke := e.users.Revoke
		if c.Pasted || e.users.Revoker == nil {
			revoke = e.users.Delete
		}
		if err := revoke(ctx, c.Owner); err != nil {
			return err
		}
	}
	s.mu.Lock()
	delete(s.entries, connKey(owner, id))
	s.mu.Unlock()
	if err := s.store.DeleteConnection(ctx, owner, id); err != nil {
		return err
	}
	s.ForgetAccess(c.Owner)
	return nil
}

// entry returns c's built token source, building it when there is none or c's sign-in fields
// changed since it was built.
func (s *Service) entry(c Connection) (*entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[connKey(c.Owner, c.ID)]; ok && sameSignIn(e.conn, c) {
		return e, nil
	}
	e := &entry{conn: c}
	switch c.Kind {
	case KindGitHubApp:
		ref, err := outbound.ParseSecretRef(c.KeyRef)
		if err != nil {
			return nil, fmt.Errorf("%w: key_ref", ErrInvalid)
		}
		pem, err := ref.Resolve()
		if err != nil {
			return nil, fmt.Errorf("connections: %s: app key: %w", c.ID, err)
		}
		app, err := outbound.NewGitHubAppInstallation(c.AppID, c.InstallationID, []byte(pem))
		if err != nil {
			return nil, fmt.Errorf("connections: %s: %w", c.ID, err)
		}
		app.BaseURL, app.HTTPClient = c.BaseURL, s.client
		e.ts = app
	case KindGitHubToken:
		st, err := outbound.NewStatic(c.TokenRef)
		if err != nil {
			return nil, fmt.Errorf("connections: %s: %w", c.ID, err)
		}
		e.ts = st
	case KindGitHubUser:
		if s.users == nil || s.oauth == nil {
			return nil, fmt.Errorf("%w: %q needs WithUserTokens", ErrNotConfigured, c.ID)
		}
		key, err := s.masterKey()
		if err != nil {
			return nil, err
		}
		u, err := outbound.NewUserTokens(UserTokenProvider(c.ID), s.users, key, s.oauth)
		if err != nil {
			return nil, fmt.Errorf("connections: %s: %w", c.ID, err)
		}
		u.HTTPClient = s.client
		u.Revoker = &outbound.GitHubRevoker{ClientID: s.oauth.ClientID, ClientSecret: s.oauth.ClientSecret,
			BaseURL: c.BaseURL, HTTPClient: s.client}
		e.users, e.ts = u, u.TokenSource(c.Owner)
	case KindGCP:
		ts, err := gcpTokenSource(c)
		if err != nil {
			return nil, err
		}
		e.ts = ts
	case KindAWS:
		// Credentials builds an AWS connection's value on each call, from the record.
	case KindEntra:
		ts, err := s.entraTokenSource(c)
		if err != nil {
			return nil, err
		}
		e.ts = ts
	case KindEntraUser:
		if s.users == nil {
			return nil, fmt.Errorf("%w: %q needs WithUserTokens", ErrNotConfigured, c.ID)
		}
		key, err := s.masterKey()
		if err != nil {
			return nil, err
		}
		cfg, err := entraUserConfig(c)
		if err != nil {
			return nil, err
		}
		u, err := outbound.NewUserTokens(UserTokenProvider(c.ID), s.users, key, cfg)
		if err != nil {
			return nil, fmt.Errorf("connections: %s: %w", c.ID, err)
		}
		u.HTTPClient = entraUserClient(s.client, c)
		e.users, e.ts = u, u.TokenSource(c.Owner)
	default:
		return nil, fmt.Errorf("%w: unknown kind %q", ErrInvalid, c.Kind)
	}
	s.entries[connKey(c.Owner, c.ID)] = e
	return e, nil
}

// connKey is a connection's key in maps: its owner and ID, which can't contain a NUL.
func connKey(owner, id string) string {
	return owner + "\x00" + id
}

// masterKey resolves the master key's reference once. Callers hold s.mu.
func (s *Service) masterKey() (string, error) {
	if s.key != "" {
		return s.key, nil
	}
	ref, err := outbound.ParseSecretRef(s.keyRef)
	if err != nil {
		return "", fmt.Errorf("%w: the master key must be a secret reference", ErrNotConfigured)
	}
	key, err := ref.Resolve()
	if err != nil {
		return "", fmt.Errorf("connections: master key: %w", err)
	}
	s.key = key
	return key, nil
}

// sameSignIn reports whether a and b sign in the same way, ignoring the fields that change with
// use: Name, Status, LastError and the times.
func sameSignIn(a, b Connection) bool {
	return a.Kind == b.Kind && a.Scope == b.Scope && a.Owner == b.Owner && a.BaseURL == b.BaseURL &&
		a.AppID == b.AppID && a.InstallationID == b.InstallationID && a.KeyRef == b.KeyRef && a.TokenRef == b.TokenRef &&
		a.AccessKeyID == b.AccessKeyID && a.RoleARN == b.RoleARN && a.Region == b.Region &&
		a.TenantID == b.TenantID && a.ClientID == b.ClientID && a.EntraAuth == b.EntraAuth && a.CertRef == b.CertRef
}

// statusSource keeps the connection's record in step with its token requests. A failure sets
// Status and LastError. The next success after one sets Status back to StatusOK. A failed record
// write doesn't fail the token request.
type statusSource struct {
	svc       *Service
	owner, id string
	e         *entry
}

func (t *statusSource) Token() (*oauth2.Token, error) {
	tok, err := t.e.ts.Token()
	status, msg := StatusOK, ""
	if err != nil {
		status, msg = StatusFailed, err.Error()
		if errors.Is(err, outbound.ErrReconnect) {
			err = fmt.Errorf("%w: %w", ErrReconnect, err)
			status, msg = StatusReconnect, err.Error()
		}
		t.e.failed.Store(true)
	} else if !t.e.failed.Swap(false) {
		return tok, nil
	}
	ctx := context.Background()
	if c, gerr := t.svc.store.GetConnection(ctx, t.owner, t.id); gerr == nil && (c.Status != status || c.LastError != msg) {
		c.Status, c.LastError = status, msg
		_ = t.svc.store.PutConnection(ctx, c)
	}
	return tok, err
}
