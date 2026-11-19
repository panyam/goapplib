package connections_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/fsstore"
	"github.com/panyam/oneauth/outbound"
	"github.com/panyam/oneauth/outbound/githubtest"
	oneauthfs "github.com/panyam/oneauth/stores/fs"
	"golang.org/x/oauth2"
)

const masterKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

type fixture struct {
	gh      *githubtest.Server
	store   *fsstore.Store
	svc     *connections.Service
	dirs    []string
	keyPath string
	now     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{gh: githubtest.New(t), now: time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)}
	connDir, tokDir, secretDir := t.TempDir(), t.TempDir(), t.TempDir()
	f.dirs = []string{connDir, tokDir}
	var err error
	if f.store, err = fsstore.New(connDir); err != nil {
		t.Fatal(err)
	}
	f.keyPath = filepath.Join(secretDir, "app.pem")
	if err := os.WriteFile(f.keyPath, f.gh.AppKeyPEM(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOAPPLIB_TEST_CONN_KEY", masterKey)
	f.svc = connections.NewService(f.store,
		connections.WithUserTokens(oneauthfs.NewFSUserTokenStore(tokDir), "env:GOAPPLIB_TEST_CONN_KEY", f.gh.OAuthConfig()),
		connections.WithClock(func() time.Time { return f.now }),
	)
	return f
}

func (f *fixture) put(t *testing.T, c connections.Connection) {
	t.Helper()
	if c.BaseURL == "" {
		c.BaseURL = f.gh.URL
	}
	if err := f.store.PutConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) app() connections.Connection {
	return connections.Connection{ID: "acme", Kind: connections.KindGitHubApp, Scope: connections.ScopeServer,
		AppID: f.gh.AppID, InstallationID: f.gh.InstallationID, KeyRef: "file:" + f.keyPath}
}

func alice() connections.Connection {
	return connections.Connection{ID: "alice-gh", Kind: connections.KindGitHubUser, Scope: connections.ScopePerson, Owner: "alice"}
}

// token returns a func that takes TokenSource's two results and fetches a token, failing t on
// either error, so a call reads token(t)(svc.TokenSource(ctx, id)).
func token(t *testing.T) func(oauth2.TokenSource, error) *oauth2.Token {
	return func(ts oauth2.TokenSource, err error) *oauth2.Token {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		tok, err := ts.Token()
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
}

func TestAppTokenIsMintedOnceAndShared(t *testing.T) {
	f := newFixture(t)
	f.put(t, f.app())
	ctx := context.Background()

	a := token(t)(f.svc.TokenSource(ctx, "acme"))
	if !f.gh.ValidInstallationToken(a.AccessToken) {
		t.Fatalf("the fake doesn't accept the installation token")
	}
	b := token(t)(f.svc.TokenSource(ctx, "acme"))
	if b.AccessToken != a.AccessToken {
		t.Errorf("a second mount on the connection got a different token")
	}
	if n := f.gh.InstallationCalls(); n != 1 {
		t.Errorf("installation token requests = %d, want 1", n)
	}
}

func TestChangedRecordDropsTheCachedSource(t *testing.T) {
	f := newFixture(t)
	f.put(t, f.app())
	ctx := context.Background()
	token(t)(f.svc.TokenSource(ctx, "acme"))

	moved := filepath.Join(filepath.Dir(f.keyPath), "rotated.pem")
	if err := os.Rename(f.keyPath, moved); err != nil {
		t.Fatal(err)
	}
	c := f.app()
	c.KeyRef = "file:" + moved
	f.put(t, c)
	token(t)(f.svc.TokenSource(ctx, "acme"))
	if n := f.gh.InstallationCalls(); n != 2 {
		t.Errorf("installation token requests = %d, want 2 after the record changed", n)
	}
}

func TestStaticTokenFromEnv(t *testing.T) {
	f := newFixture(t)
	t.Setenv("GOAPPLIB_TEST_PAT", "pat-123")
	f.put(t, connections.Connection{ID: "ci", Kind: connections.KindGitHubToken, Scope: connections.ScopeServer, TokenRef: "env:GOAPPLIB_TEST_PAT"})
	if tok := token(t)(f.svc.TokenSource(context.Background(), "ci")); tok.AccessToken != "pat-123" {
		t.Errorf("AccessToken = %q, want pat-123", tok.AccessToken)
	}
}

func TestUserTokenRefreshes(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")

	issued := f.gh.IssueUserToken("alice")
	issued.Expiry = time.Now().Add(-time.Minute)
	if err := f.svc.SaveUserToken(ctx, "alice-gh", issued); err != nil {
		t.Fatal(err)
	}
	got := token(t)(f.svc.TokenSource(ctx, "alice-gh"))
	if got.AccessToken == issued.AccessToken {
		t.Fatalf("the expired token was handed out without a refresh")
	}
	if subject, ok := f.gh.ValidUserToken(got.AccessToken); !ok || subject != "alice" {
		t.Errorf("the refreshed token isn't alice's at the fake: %q %v", subject, ok)
	}
	if n := f.gh.RefreshCalls(); n != 1 {
		t.Errorf("refresh calls = %d, want 1", n)
	}
}

func TestPersonConnectionRefusesOtherViewers(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	for name, ctx := range map[string]context.Context{
		"bob":       connections.WithViewer(context.Background(), "bob"),
		"no viewer": context.Background(),
	} {
		if _, err := f.svc.TokenSource(ctx, "alice-gh"); !errors.Is(err, connections.ErrNotOwner) {
			t.Errorf("%s: TokenSource = %v, want ErrNotOwner", name, err)
		}
	}
}

func TestDeadRefreshMarksReconnect(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	if err := f.svc.SaveUserToken(ctx, "alice-gh", &oauth2.Token{
		AccessToken: "gho_stale", RefreshToken: "ghr_dead", TokenType: "bearer", Expiry: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	ts, err := f.svc.TokenSource(ctx, "alice-gh")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ts.Token()
	if !errors.Is(err, connections.ErrReconnect) || !errors.Is(err, outbound.ErrReconnect) {
		t.Fatalf("Token = %v, want ErrReconnect wrapping outbound.ErrReconnect", err)
	}
	c, err := f.store.GetConnection(context.Background(), "alice-gh")
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != connections.StatusReconnect || c.LastError == "" {
		t.Errorf("record status = %q, last error = %q; want reconnect and a message", c.Status, c.LastError)
	}
	if strings.Contains(c.LastError, "ghr_dead") || strings.Contains(c.LastError, "gho_stale") {
		t.Errorf("LastError repeats a token: %q", c.LastError)
	}
}

func TestSavingATokenClearsReconnect(t *testing.T) {
	f := newFixture(t)
	c := alice()
	c.Status, c.LastError = connections.StatusReconnect, "provider refused the refresh token"
	f.put(t, c)
	ctx := connections.WithViewer(context.Background(), "alice")
	if err := f.svc.SaveUserToken(ctx, "alice-gh", f.gh.IssueUserToken("alice")); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.GetConnection(ctx, "alice-gh")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != connections.StatusOK || got.LastError != "" {
		t.Errorf("after a new connect, status = %q, last error = %q", got.Status, got.LastError)
	}
}

func TestDisconnectRevokesThenDeletes(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	issued := f.gh.IssueUserToken("alice")
	if err := f.svc.SaveUserToken(ctx, "alice-gh", issued); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Disconnect(ctx, "alice-gh"); err != nil {
		t.Fatal(err)
	}
	if n := f.gh.RevokeCalls(); n != 1 {
		t.Errorf("revoke calls = %d, want 1", n)
	}
	if _, ok := f.gh.ValidUserToken(issued.AccessToken); ok {
		t.Error("the token still works at the fake after Disconnect")
	}
	if _, err := f.store.GetConnection(ctx, "alice-gh"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("record after Disconnect: %v, want ErrNotFound", err)
	}
	if err := f.svc.Disconnect(ctx, "alice-gh"); err != nil {
		t.Errorf("a second Disconnect = %v, want nil", err)
	}
}

func TestDisconnectKeepsBothWhenRevokeFails(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	if err := f.svc.SaveUserToken(ctx, "alice-gh", &oauth2.Token{
		AccessToken: "gho_unknown_to_github", TokenType: "bearer", Expiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Disconnect(ctx, "alice-gh"); err == nil {
		t.Fatal("Disconnect succeeded though GitHub refused the revoke")
	}
	if _, err := f.store.GetConnection(ctx, "alice-gh"); err != nil {
		t.Errorf("the record was deleted after a failed revoke: %v", err)
	}
	if tok := token(t)(f.svc.TokenSource(ctx, "alice-gh")); tok.AccessToken != "gho_unknown_to_github" {
		t.Errorf("the stored token was deleted after a failed revoke")
	}
}

func TestNoTokenInAnyStoredFile(t *testing.T) {
	f := newFixture(t)
	f.put(t, f.app())
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	issued := f.gh.IssueUserToken("alice")
	if err := f.svc.SaveUserToken(ctx, "alice-gh", issued); err != nil {
		t.Fatal(err)
	}
	app := token(t)(f.svc.TokenSource(ctx, "acme"))
	user := token(t)(f.svc.TokenSource(ctx, "alice-gh"))
	secrets := []string{app.AccessToken, user.AccessToken, issued.AccessToken, issued.RefreshToken}

	files := 0
	for _, dir := range f.dirs {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			files++
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, s := range secrets {
				if s != "" && strings.Contains(string(b), s) {
					t.Errorf("%s holds a token in the clear", p)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if files < 3 {
		t.Errorf("checked %d files, want the two records and the sealed token", files)
	}
}

func TestLastUsedIsWrittenAtMostHourly(t *testing.T) {
	f := newFixture(t)
	t.Setenv("GOAPPLIB_TEST_PAT", "pat-123")
	f.put(t, connections.Connection{ID: "ci", Kind: connections.KindGitHubToken, Scope: connections.ScopeServer, TokenRef: "env:GOAPPLIB_TEST_PAT"})
	ctx := context.Background()
	lastUsed := func() time.Time {
		c, err := f.store.GetConnection(ctx, "ci")
		if err != nil {
			t.Fatal(err)
		}
		return c.LastUsed
	}

	start := f.now
	token(t)(f.svc.TokenSource(ctx, "ci"))
	if got := lastUsed(); !got.Equal(start) {
		t.Fatalf("LastUsed = %v, want %v", got, start)
	}
	f.now = start.Add(10 * time.Minute)
	token(t)(f.svc.TokenSource(ctx, "ci"))
	if got := lastUsed(); !got.Equal(start) {
		t.Errorf("LastUsed moved within the hour: %v", got)
	}
	f.now = start.Add(61 * time.Minute)
	token(t)(f.svc.TokenSource(ctx, "ci"))
	if got := lastUsed(); !got.Equal(f.now) {
		t.Errorf("LastUsed = %v, want %v after an hour", got, f.now)
	}
}

func TestUserConnectionNeedsUserTokens(t *testing.T) {
	gh := githubtest.New(t)
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := alice()
	c.BaseURL = gh.URL
	if err := store.PutConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	svc := connections.NewService(store)
	ctx := connections.WithViewer(context.Background(), "alice")
	if _, err := svc.TokenSource(ctx, "alice-gh"); !errors.Is(err, connections.ErrNotConfigured) {
		t.Errorf("TokenSource = %v, want ErrNotConfigured", err)
	}
}

func TestMissingConnection(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.TokenSource(context.Background(), "nope"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("TokenSource(missing) = %v, want ErrNotFound", err)
	}
}

func TestASuccessAfterAFailureClearsTheStatus(t *testing.T) {
	f := newFixture(t)
	c := f.app()
	c.Status, c.LastError = connections.StatusFailed, "github: 502"
	f.put(t, c)
	ctx := context.Background()
	token(t)(f.svc.TokenSource(ctx, "acme"))
	got, err := f.store.GetConnection(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != connections.StatusOK || got.LastError != "" {
		t.Errorf("after a good token, status = %q, last error = %q", got.Status, got.LastError)
	}
}
