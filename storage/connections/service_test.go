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
	"github.com/panyam/goapplib/storage/mounts"
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

// token returns a func that takes Credentials' two results and fetches a token, failing t on
// either error, so a call reads token(t)(svc.Credentials(ctx, owner, id)).
func token(t *testing.T) func(mounts.Credentials, error) *oauth2.Token {
	return func(ts mounts.Credentials, err error) *oauth2.Token {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		src, err := mounts.TokenSourceOf(ts)
		if err != nil {
			t.Fatal(err)
		}
		tok, err := src.Token()
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

	a := token(t)(f.svc.Credentials(ctx, "", "acme"))
	if !f.gh.ValidInstallationToken(a.AccessToken) {
		t.Fatalf("the fake doesn't accept the installation token")
	}
	b := token(t)(f.svc.Credentials(ctx, "", "acme"))
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
	token(t)(f.svc.Credentials(ctx, "", "acme"))

	moved := filepath.Join(filepath.Dir(f.keyPath), "rotated.pem")
	if err := os.Rename(f.keyPath, moved); err != nil {
		t.Fatal(err)
	}
	c := f.app()
	c.KeyRef = "file:" + moved
	f.put(t, c)
	token(t)(f.svc.Credentials(ctx, "", "acme"))
	if n := f.gh.InstallationCalls(); n != 2 {
		t.Errorf("installation token requests = %d, want 2 after the record changed", n)
	}
}

func TestStaticTokenFromEnv(t *testing.T) {
	f := newFixture(t)
	t.Setenv("GOAPPLIB_TEST_PAT", "pat-123")
	f.put(t, connections.Connection{ID: "ci", Kind: connections.KindGitHubToken, Scope: connections.ScopeServer, TokenRef: "env:GOAPPLIB_TEST_PAT"})
	if tok := token(t)(f.svc.Credentials(context.Background(), "", "ci")); tok.AccessToken != "pat-123" {
		t.Errorf("AccessToken = %q, want pat-123", tok.AccessToken)
	}
}

func TestUserTokenRefreshes(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")

	issued := f.gh.IssueUserToken("alice")
	issued.Expiry = time.Now().Add(-time.Minute)
	if err := f.svc.SaveUserToken(ctx, "alice", "alice-gh", issued); err != nil {
		t.Fatal(err)
	}
	got := token(t)(f.svc.Credentials(ctx, "alice", "alice-gh"))
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
		if _, err := f.svc.Credentials(ctx, "alice", "alice-gh"); !errors.Is(err, connections.ErrNotOwner) {
			t.Errorf("%s: TokenSource = %v, want ErrNotOwner", name, err)
		}
	}
}

func TestDeadRefreshMarksReconnect(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	if err := f.svc.SaveUserToken(ctx, "alice", "alice-gh", &oauth2.Token{
		AccessToken: "gho_stale", RefreshToken: "ghr_dead", TokenType: "bearer", Expiry: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	ts, err := f.svc.Credentials(ctx, "alice", "alice-gh")
	if err != nil {
		t.Fatal(err)
	}
	src, _ := mounts.TokenSourceOf(ts)
	_, err = src.Token()
	if !errors.Is(err, connections.ErrReconnect) || !errors.Is(err, outbound.ErrReconnect) {
		t.Fatalf("Token = %v, want ErrReconnect wrapping outbound.ErrReconnect", err)
	}
	c, err := f.store.GetConnection(context.Background(), "alice", "alice-gh")
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
	if err := f.svc.SaveUserToken(ctx, "alice", "alice-gh", f.gh.IssueUserToken("alice")); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.GetConnection(ctx, "alice", "alice-gh")
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
	if err := f.svc.SaveUserToken(ctx, "alice", "alice-gh", issued); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Disconnect(ctx, "alice", "alice-gh"); err != nil {
		t.Fatal(err)
	}
	if n := f.gh.RevokeCalls(); n != 1 {
		t.Errorf("revoke calls = %d, want 1", n)
	}
	if _, ok := f.gh.ValidUserToken(issued.AccessToken); ok {
		t.Error("the token still works at the fake after Disconnect")
	}
	if _, err := f.store.GetConnection(ctx, "alice", "alice-gh"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("record after Disconnect: %v, want ErrNotFound", err)
	}
	if err := f.svc.Disconnect(ctx, "alice", "alice-gh"); err != nil {
		t.Errorf("a second Disconnect = %v, want nil", err)
	}
}

func TestDisconnectKeepsBothWhenRevokeFails(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	if err := f.svc.SaveUserToken(ctx, "alice", "alice-gh", &oauth2.Token{
		AccessToken: "gho_unknown_to_github", TokenType: "bearer", Expiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Disconnect(ctx, "alice", "alice-gh"); err == nil {
		t.Fatal("Disconnect succeeded though GitHub refused the revoke")
	}
	if _, err := f.store.GetConnection(ctx, "alice", "alice-gh"); err != nil {
		t.Errorf("the record was deleted after a failed revoke: %v", err)
	}
	if tok := token(t)(f.svc.Credentials(ctx, "alice", "alice-gh")); tok.AccessToken != "gho_unknown_to_github" {
		t.Errorf("the stored token was deleted after a failed revoke")
	}
}

// A person pasted a token the OAuth app never issued, so GitHub would refuse to revoke it
// through the app (agni#1772).
func TestDisconnectDeletesAPastedTokenWithoutRevoking(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	if err := f.svc.SavePastedToken(ctx, "alice", "alice-gh", &oauth2.Token{AccessToken: "ghp_pasted", TokenType: "bearer"}); err != nil {
		t.Fatal(err)
	}
	if c, err := f.store.GetConnection(ctx, "alice", "alice-gh"); err != nil || !c.Pasted {
		t.Fatalf("after SavePastedToken, Pasted = %v (err %v), want true", c.Pasted, err)
	}
	if tok := token(t)(f.svc.Credentials(ctx, "alice", "alice-gh")); tok.AccessToken != "ghp_pasted" {
		t.Errorf("token = %q, want the pasted one", tok.AccessToken)
	}
	if err := f.svc.Disconnect(ctx, "alice", "alice-gh"); err != nil {
		t.Fatal(err)
	}
	if n := f.gh.RevokeCalls(); n != 0 {
		t.Errorf("revoke calls = %d, want 0", n)
	}
	if _, err := f.store.GetConnection(ctx, "alice", "alice-gh"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("record after Disconnect: %v, want ErrNotFound", err)
	}
	f.put(t, alice())
	ts, err := f.svc.Credentials(ctx, "alice", "alice-gh")
	if err != nil {
		t.Fatal(err)
	}
	src, _ := mounts.TokenSourceOf(ts)
	if _, err := src.Token(); !errors.Is(err, outbound.ErrUserTokenNotFound) {
		t.Errorf("token after Disconnect: %v, want ErrUserTokenNotFound", err)
	}
}

func TestSaveUserTokenAfterAPasteRevokesAgain(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	if err := f.svc.SavePastedToken(ctx, "alice", "alice-gh", &oauth2.Token{AccessToken: "ghp_pasted", TokenType: "bearer"}); err != nil {
		t.Fatal(err)
	}
	issued := f.gh.IssueUserToken("alice")
	if err := f.svc.SaveUserToken(ctx, "alice", "alice-gh", issued); err != nil {
		t.Fatal(err)
	}
	if c, err := f.store.GetConnection(ctx, "alice", "alice-gh"); err != nil || c.Pasted {
		t.Fatalf("after SaveUserToken, Pasted = %v (err %v), want false", c.Pasted, err)
	}
	if err := f.svc.Disconnect(ctx, "alice", "alice-gh"); err != nil {
		t.Fatal(err)
	}
	if n := f.gh.RevokeCalls(); n != 1 {
		t.Errorf("revoke calls = %d, want 1", n)
	}
	if _, ok := f.gh.ValidUserToken(issued.AccessToken); ok {
		t.Error("the app's token still works at the fake after Disconnect")
	}
}

// failingPuts fails every PutConnection while fail is set.
type failingPuts struct {
	connections.Store
	fail bool
}

func (s *failingPuts) PutConnection(ctx context.Context, c connections.Connection) error {
	if s.fail {
		return errors.New("disk full")
	}
	return s.Store.PutConnection(ctx, c)
}

// When the record write after the token write fails, the record still says the app issued the
// token, so Disconnect tries the revoke and keeps everything rather than skip it.
func TestSavePastedTokenFailedRecordWriteStillRevokes(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	store := &failingPuts{Store: f.store}
	svc := connections.NewService(store,
		connections.WithUserTokens(oneauthfs.NewFSUserTokenStore(f.dirs[1]), "env:GOAPPLIB_TEST_CONN_KEY", f.gh.OAuthConfig()))
	ctx := connections.WithViewer(context.Background(), "alice")
	store.fail = true
	if err := svc.SavePastedToken(ctx, "alice", "alice-gh", &oauth2.Token{AccessToken: "ghp_pasted", TokenType: "bearer"}); err == nil {
		t.Fatal("SavePastedToken succeeded though the record write failed")
	}
	store.fail = false
	if err := svc.Disconnect(ctx, "alice", "alice-gh"); err == nil {
		t.Fatal("Disconnect succeeded without the Pasted flag, so it skipped the revoke")
	}
	if n := f.gh.RevokeCalls(); n != 1 {
		t.Errorf("revoke calls = %d, want 1", n)
	}
	if _, err := f.store.GetConnection(ctx, "alice", "alice-gh"); err != nil {
		t.Errorf("the record was deleted after a failed revoke: %v", err)
	}
}

func TestSavePastedTokenChecksKindAndOwner(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	f.put(t, f.app())
	tok := &oauth2.Token{AccessToken: "ghp_pasted", TokenType: "bearer"}
	if err := f.svc.SavePastedToken(connections.WithViewer(context.Background(), "bob"), "alice", "alice-gh", tok); !errors.Is(err, connections.ErrNotOwner) {
		t.Errorf("another viewer: %v, want ErrNotOwner", err)
	}
	if err := f.svc.SavePastedToken(connections.WithViewer(context.Background(), "alice"), "", "acme", tok); !errors.Is(err, connections.ErrNotOwner) {
		t.Errorf("a server connection: %v, want ErrNotOwner", err)
	}
	f.put(t, connections.Connection{ID: "alice-pat", Kind: connections.KindGitHubToken, Scope: connections.ScopePerson, Owner: "alice", TokenRef: "env:ALICE_PAT"})
	if err := f.svc.SavePastedToken(connections.WithViewer(context.Background(), "alice"), "alice", "alice-pat", tok); !errors.Is(err, connections.ErrInvalid) {
		t.Errorf("a token connection: %v, want ErrInvalid", err)
	}
}

func TestNoTokenInAnyStoredFile(t *testing.T) {
	f := newFixture(t)
	f.put(t, f.app())
	f.put(t, alice())
	ctx := connections.WithViewer(context.Background(), "alice")
	issued := f.gh.IssueUserToken("alice")
	if err := f.svc.SaveUserToken(ctx, "alice", "alice-gh", issued); err != nil {
		t.Fatal(err)
	}
	app := token(t)(f.svc.Credentials(ctx, "", "acme"))
	user := token(t)(f.svc.Credentials(ctx, "alice", "alice-gh"))
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
		c, err := f.store.GetConnection(ctx, "", "ci")
		if err != nil {
			t.Fatal(err)
		}
		return c.LastUsed
	}

	start := f.now
	token(t)(f.svc.Credentials(ctx, "", "ci"))
	if got := lastUsed(); !got.Equal(start) {
		t.Fatalf("LastUsed = %v, want %v", got, start)
	}
	f.now = start.Add(10 * time.Minute)
	token(t)(f.svc.Credentials(ctx, "", "ci"))
	if got := lastUsed(); !got.Equal(start) {
		t.Errorf("LastUsed moved within the hour: %v", got)
	}
	f.now = start.Add(61 * time.Minute)
	token(t)(f.svc.Credentials(ctx, "", "ci"))
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
	if _, err := svc.Credentials(ctx, "alice", "alice-gh"); !errors.Is(err, connections.ErrNotConfigured) {
		t.Errorf("TokenSource = %v, want ErrNotConfigured", err)
	}
}

func TestMissingConnection(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Credentials(context.Background(), "", "nope"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("TokenSource(missing) = %v, want ErrNotFound", err)
	}
}

func TestASuccessAfterAFailureClearsTheStatus(t *testing.T) {
	f := newFixture(t)
	c := f.app()
	c.Status, c.LastError = connections.StatusFailed, "github: 502"
	f.put(t, c)
	ctx := context.Background()
	token(t)(f.svc.Credentials(ctx, "", "acme"))
	got, err := f.store.GetConnection(ctx, "", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != connections.StatusOK || got.LastError != "" {
		t.Errorf("after a good token, status = %q, last error = %q", got.Status, got.LastError)
	}
}

func person(id, owner string) connections.Connection {
	return connections.Connection{ID: id, Kind: connections.KindGitHubUser, Scope: connections.ScopePerson, Owner: owner}
}

// paste gives owner's connection id a pasted token named after them.
func (f *fixture) paste(t *testing.T, owner, id string) {
	t.Helper()
	ctx := connections.WithViewer(context.Background(), owner)
	if err := f.svc.SavePastedToken(ctx, owner, id, &oauth2.Token{AccessToken: "ghp_" + owner, TokenType: "bearer"}); err != nil {
		t.Fatal(err)
	}
}

func TestTwoPeopleEachReadThroughTheirOwnConnectionWithOneID(t *testing.T) {
	f := newFixture(t)
	f.put(t, person("gh", "alice"))
	f.put(t, person("gh", "bob"))
	f.paste(t, "alice", "gh")
	f.paste(t, "bob", "gh")
	for _, owner := range []string{"alice", "bob", "alice"} {
		ctx := connections.WithViewer(context.Background(), owner)
		if tok := token(t)(f.svc.Credentials(ctx, owner, "gh")); tok.AccessToken != "ghp_"+owner {
			t.Errorf("%s's gh gave %q, want ghp_%s", owner, tok.AccessToken, owner)
		}
	}
}

func TestAnotherOwnersConnectionIsRefusedWithoutLookingItUp(t *testing.T) {
	f := newFixture(t)
	f.put(t, alice())
	bob := connections.WithViewer(context.Background(), "bob")
	// alice-gh exists and nope doesn't, and bob gets the same error for both.
	for _, id := range []string{"alice-gh", "nope"} {
		if _, err := f.svc.Credentials(bob, "alice", id); !errors.Is(err, connections.ErrNotOwner) {
			t.Errorf("bob asking for alice's %s: %v, want ErrNotOwner", id, err)
		}
	}
}

func TestServerAndPersonConnectionsWithOneIDStayApart(t *testing.T) {
	f := newFixture(t)
	t.Setenv("CI_TOKEN", "pat-123")
	f.put(t, connections.Connection{ID: "ci", Kind: connections.KindGitHubToken, Scope: connections.ScopeServer, TokenRef: "env:CI_TOKEN"})
	f.put(t, person("ci", "alice"))
	f.paste(t, "alice", "ci")
	ctx := connections.WithViewer(context.Background(), "alice")
	if tok := token(t)(f.svc.Credentials(ctx, "", "ci")); tok.AccessToken != "pat-123" {
		t.Errorf("the server's ci gave %q, want pat-123", tok.AccessToken)
	}
	if tok := token(t)(f.svc.Credentials(ctx, "alice", "ci")); tok.AccessToken != "ghp_alice" {
		t.Errorf("alice's ci gave %q, want ghp_alice", tok.AccessToken)
	}
}

func TestDisconnectLeavesAnotherOwnersConnectionWithTheSameID(t *testing.T) {
	f := newFixture(t)
	f.put(t, person("gh", "alice"))
	f.put(t, person("gh", "bob"))
	f.paste(t, "alice", "gh")
	f.paste(t, "bob", "gh")
	if err := f.svc.Disconnect(context.Background(), "bob", "gh"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.GetConnection(context.Background(), "bob", "gh"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("bob's gh after Disconnect: %v, want ErrNotFound", err)
	}
	ctx := connections.WithViewer(context.Background(), "alice")
	if tok := token(t)(f.svc.Credentials(ctx, "alice", "gh")); tok.AccessToken != "ghp_alice" {
		t.Errorf("alice's gh after bob's Disconnect gave %q, want ghp_alice", tok.AccessToken)
	}
}
