package connections_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/fsstore"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/github"
	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
	oneauthfs "github.com/panyam/oneauth/stores/fs"
	"golang.org/x/oauth2"
)

type access struct {
	ctx   context.Context
	gh    *githubtest.Server
	src   string
	store *fsstore.Store
	svc   *connections.Service
	now   time.Time
}

func newAccess(t *testing.T) *access {
	t.Helper()
	a := &access{ctx: context.Background(), gh: githubtest.New(t), now: time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)}
	a.src = a.gh.AddRepo("acme", "board", true, map[string]string{"README.md": "private"}).Source()
	a.gh.AddToken("ghp_alice")
	t.Setenv("ALICE_TOKEN", "ghp_alice")
	t.Setenv("ALICE_OLD_TOKEN", "ghp_revoked")
	t.Setenv("BOB_TOKEN", "ghp_bob")
	a.gh.AddToken("ghp_bob")
	var err error
	if a.store, err = fsstore.New(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	drv := github.New(nil, github.WithBaseURL(a.gh.URL))
	a.svc = connections.NewService(a.store,
		connections.WithReadCheck(drv.CanRead),
		connections.WithClock(func() time.Time { return a.now }),
	)
	return a
}

func (a *access) put(t *testing.T, id, owner, env, baseURL string) {
	t.Helper()
	c := connections.Connection{ID: id, Kind: connections.KindGitHubToken, Scope: connections.ScopePerson,
		Owner: owner, TokenRef: "env:" + env, BaseURL: baseURL, Status: connections.StatusOK}
	if err := a.store.PutConnection(a.ctx, c); err != nil {
		t.Fatal(err)
	}
}

func (a *access) as(viewer string) context.Context {
	return connections.WithViewer(a.ctx, viewer)
}

func TestCanReadYes(t *testing.T) {
	a := newAccess(t)
	a.put(t, "alice-gh", "alice", "ALICE_TOKEN", a.gh.URL)
	id, err := a.svc.CanRead(a.as("alice"), a.src)
	if err != nil || id != "alice-gh" {
		t.Fatalf("CanRead = %q, %v; want alice-gh", id, err)
	}
}

func TestCanReadNoWhenTheRepoIsHidden(t *testing.T) {
	a := newAccess(t)
	a.put(t, "alice-gh", "alice", "ALICE_TOKEN", a.gh.URL)
	if id, err := a.svc.CanRead(a.as("alice"), "github://acme/secret"); !errors.Is(err, connections.ErrNoAccess) || id != "" {
		t.Errorf("CanRead of a repo GitHub hides = %q, %v; want ErrNoAccess", id, err)
	}
}

func TestCanReadNoConnection(t *testing.T) {
	a := newAccess(t)
	a.put(t, "bob-gh", "bob", "BOB_TOKEN", a.gh.URL)
	if id, err := a.svc.CanRead(a.as("alice"), a.src); !errors.Is(err, connections.ErrNoAccess) || id != "" {
		t.Errorf("CanRead with no connection of alice's = %q, %v; want ErrNoAccess", id, err)
	}
	for _, r := range a.gh.Requests() {
		if r.Token == "ghp_bob" {
			t.Errorf("alice's check used bob's token on %s %s", r.Method, r.Path)
		}
	}
	if _, err := a.svc.CanRead(a.ctx, a.src); !errors.Is(err, connections.ErrNotOwner) {
		t.Errorf("CanRead with no viewer = %v, want ErrNotOwner", err)
	}
}

func TestCanReadSkipsAnotherHost(t *testing.T) {
	a := newAccess(t)
	a.put(t, "alice-ghe", "alice", "ALICE_TOKEN", "https://ghe.example.com/api/v3")
	if _, err := a.svc.CanRead(a.as("alice"), a.src); !errors.Is(err, connections.ErrNoAccess) {
		t.Errorf("a connection to another host = %v, want ErrNoAccess", err)
	}
	if n := len(a.gh.Requests()); n != 0 {
		t.Errorf("a connection to another host sent %d requests to this one", n)
	}
}

func TestCanReadTriesEachConnection(t *testing.T) {
	a := newAccess(t)
	a.put(t, "a-old", "alice", "ALICE_OLD_TOKEN", a.gh.URL)
	a.put(t, "b-good", "alice", "ALICE_TOKEN", a.gh.URL)
	if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "b-good" {
		t.Errorf("CanRead past a rejected token = %q, %v; want b-good", id, err)
	}
}

func TestCanReadReportsAFailedCheck(t *testing.T) {
	a := newAccess(t)
	a.put(t, "a-old", "alice", "ALICE_OLD_TOKEN", a.gh.URL)
	_, err := a.svc.CanRead(a.as("alice"), a.src)
	if err == nil || errors.Is(err, connections.ErrNoAccess) || !errors.Is(err, mounts.ErrUnavailable) {
		t.Fatalf("CanRead with only a rejected token = %v; want the check's ErrUnavailable, not ErrNoAccess", err)
	}
	a.put(t, "a-old", "alice", "ALICE_TOKEN", a.gh.URL)
	if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "a-old" {
		t.Errorf("after the connection was fixed, CanRead = %q, %v; a failed check must not be cached", id, err)
	}
}

func TestCanReadCaches(t *testing.T) {
	a := newAccess(t)
	a.put(t, "alice-gh", "alice", "ALICE_TOKEN", a.gh.URL)
	if _, err := a.svc.CanRead(a.as("alice"), a.src); err != nil {
		t.Fatal(err)
	}
	if _, err := a.svc.CanRead(a.as("alice"), "github://acme/secret"); !errors.Is(err, connections.ErrNoAccess) {
		t.Fatal(err)
	}
	n := len(a.gh.Requests())
	a.now = a.now.Add(connections.DefaultAccessTTL - time.Second)
	if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "alice-gh" {
		t.Errorf("cached yes = %q, %v", id, err)
	}
	if _, err := a.svc.CanRead(a.as("alice"), "github://acme/secret"); !errors.Is(err, connections.ErrNoAccess) {
		t.Errorf("cached no = %v", err)
	}
	if got := len(a.gh.Requests()); got != n {
		t.Errorf("calls within the TTL made %d requests, want 0", got-n)
	}
	if _, err := a.svc.CanRead(a.as("bob"), a.src); !errors.Is(err, connections.ErrNoAccess) {
		t.Errorf("bob got alice's cached answer: %v", err)
	}
	a.now = a.now.Add(2 * time.Second)
	a.svc.CanRead(a.as("alice"), a.src)
	if got := len(a.gh.Requests()); got == n {
		t.Error("a call after the TTL made no request")
	}
}

func TestCanReadNeedsACheck(t *testing.T) {
	a := newAccess(t)
	svc := connections.NewService(a.store)
	if _, err := svc.CanRead(a.as("alice"), a.src); !errors.Is(err, connections.ErrNotConfigured) {
		t.Errorf("CanRead without WithReadCheck = %v, want ErrNotConfigured", err)
	}
}

// newUserAccess is newAccess with user tokens, so alice can connect with SaveUserToken or
// SavePastedToken. Her token is ghu_alice, which reads the repo. She has no connection yet.
func newUserAccess(t *testing.T) *access {
	t.Helper()
	a := newAccess(t)
	a.gh.AddToken("ghu_alice")
	t.Setenv("GOAPPLIB_TEST_CONN_KEY", masterKey)
	drv := github.New(nil, github.WithBaseURL(a.gh.URL))
	a.svc = connections.NewService(a.store,
		connections.WithReadCheck(drv.CanRead),
		connections.WithClock(func() time.Time { return a.now }),
		connections.WithUserTokens(oneauthfs.NewFSUserTokenStore(t.TempDir()), "env:GOAPPLIB_TEST_CONN_KEY",
			&oauth2.Config{ClientID: "cid", ClientSecret: "cs", Endpoint: oauth2.Endpoint{TokenURL: a.gh.URL + "/login/oauth/access_token"}}),
	)
	return a
}

func TestCanReadForgetsANoWhenTheViewerSavesAToken(t *testing.T) {
	for name, save := range map[string]func(*connections.Service, context.Context, string, string, *oauth2.Token) error{
		"SaveUserToken":   (*connections.Service).SaveUserToken,
		"SavePastedToken": (*connections.Service).SavePastedToken,
	} {
		t.Run(name, func(t *testing.T) {
			a := newUserAccess(t)
			if _, err := a.svc.CanRead(a.as("alice"), a.src); !errors.Is(err, connections.ErrNoAccess) {
				t.Fatalf("CanRead before alice connects = %v, want ErrNoAccess", err)
			}
			// The app makes the record, then saves the token from the OAuth callback.
			c := alice()
			c.BaseURL = a.gh.URL
			if err := a.store.PutConnection(a.ctx, c); err != nil {
				t.Fatal(err)
			}
			tok := &oauth2.Token{AccessToken: "ghu_alice", Expiry: time.Now().Add(time.Hour)}
			if err := save(a.svc, a.as("alice"), "alice", "alice-gh", tok); err != nil {
				t.Fatal(err)
			}
			if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "alice-gh" {
				t.Errorf("CanRead right after %s = %q, %v; want alice-gh with no wait", name, id, err)
			}
		})
	}
}

func TestCanReadForgetsAYesWhenTheViewerDisconnects(t *testing.T) {
	a := newAccess(t)
	a.put(t, "alice-gh", "alice", "ALICE_TOKEN", a.gh.URL)
	if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "alice-gh" {
		t.Fatalf("CanRead = %q, %v; want alice-gh", id, err)
	}
	if err := a.svc.Disconnect(a.ctx, "alice", "alice-gh"); err != nil {
		t.Fatal(err)
	}
	if id, err := a.svc.CanRead(a.as("alice"), a.src); !errors.Is(err, connections.ErrNoAccess) {
		t.Errorf("CanRead right after Disconnect = %q, %v; want ErrNoAccess with no wait", id, err)
	}
}

func TestForgetAccessDropsOnlyThatViewersAnswers(t *testing.T) {
	a := newAccess(t)
	a.put(t, "bob-gh", "bob", "BOB_TOKEN", a.gh.URL)
	if _, err := a.svc.CanRead(a.as("alice"), a.src); !errors.Is(err, connections.ErrNoAccess) {
		t.Fatalf("alice with no connection = %v, want ErrNoAccess", err)
	}
	if _, err := a.svc.CanRead(a.as("bob"), a.src); err != nil {
		t.Fatal(err)
	}

	// The app adds alice's connection through the store, which the service can't see.
	a.put(t, "alice-gh", "alice", "ALICE_TOKEN", a.gh.URL)
	if _, err := a.svc.CanRead(a.as("alice"), a.src); !errors.Is(err, connections.ErrNoAccess) {
		t.Fatalf("before ForgetAccess, alice's kept no = %v, want ErrNoAccess", err)
	}
	a.svc.ForgetAccess("alice")
	n := len(a.gh.Requests())
	if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "alice-gh" {
		t.Errorf("after ForgetAccess, CanRead = %q, %v; want alice-gh", id, err)
	}
	if _, err := a.svc.CanRead(a.as("bob"), a.src); err != nil {
		t.Fatal(err)
	}
	for _, r := range a.gh.Requests()[n:] {
		if r.Token == "ghp_bob" {
			t.Errorf("ForgetAccess(alice) dropped bob's kept answer: %s %s", r.Method, r.Path)
		}
	}
}

func TestCanReadDoesNotKeepAnAnswerFromBeforeAForget(t *testing.T) {
	a := newAccess(t)
	a.put(t, "alice-gh", "alice", "ALICE_TOKEN", a.gh.URL)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	a.svc = connections.NewService(a.store,
		connections.WithClock(func() time.Time { return a.now }),
		connections.WithReadCheck(func(ctx context.Context, baseURL, source string, ts mounts.Credentials) (bool, error) {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
				return false, nil
			}
			return true, nil
		}))

	done := make(chan error)
	go func() {
		_, err := a.svc.CanRead(a.as("alice"), a.src)
		done <- err
	}()
	<-entered
	a.svc.ForgetAccess("alice")
	close(release)
	if err := <-done; !errors.Is(err, connections.ErrNoAccess) {
		t.Fatalf("the call in flight = %v, want its own ErrNoAccess", err)
	}
	if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "alice-gh" {
		t.Errorf("CanRead after the forget = %q, %v; the no from before it was kept", id, err)
	}
}

// recordingCheck answers yes to want and records each token it was handed.
func recordingCheck(t *testing.T, seen *[]string, want string) connections.ReadCheck {
	return func(ctx context.Context, baseURL, source string, creds mounts.Credentials) (bool, error) {
		tok, err := mounts.BearerToken(creds)
		if err != nil {
			t.Fatal(err)
		}
		*seen = append(*seen, tok)
		return tok == want, nil
	}
}

// TestReadChecksGoOnlyToTheirKinds gives alice a GitHub token connection and an Entra sign-in. A
// check sees only the connections of its kinds, so her Entra token never reaches GitHub and her
// GitHub token never reaches Graph.
func TestReadChecksGoOnlyToTheirKinds(t *testing.T) {
	ctx := context.Background()
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOAPPLIB_TEST_GH_TOKEN", "gh-token")
	t.Setenv("GOAPPLIB_TEST_CONN_KEY", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	for _, c := range []connections.Connection{
		{ID: "a-gh", Kind: connections.KindGitHubToken, Scope: connections.ScopePerson, Owner: "alice", TokenRef: "env:GOAPPLIB_TEST_GH_TOKEN"},
		{ID: "b-sp", Kind: connections.KindEntraUser, Scope: connections.ScopePerson, Owner: "alice", TenantID: "contoso", ClientID: "app-1"},
	} {
		if err := store.PutConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	var ghSeen, graphSeen, otherSeen []string
	users := oneauthfs.NewFSUserTokenStore(t.TempDir())
	svc := connections.NewService(store,
		connections.WithUserTokens(users, "env:GOAPPLIB_TEST_CONN_KEY", nil),
		connections.WithReadCheckFor("github", recordingCheck(t, &ghSeen, "gh-token"), connections.KindGitHubUser, connections.KindGitHubToken),
		connections.WithReadCheckFor("msgraph", recordingCheck(t, &graphSeen, "entra-token"), connections.KindEntraUser),
		connections.WithReadCheck(recordingCheck(t, &otherSeen, "nobody")))
	as := connections.WithViewer(ctx, "alice")
	if err := svc.SaveUserToken(as, "alice", "b-sp", &oauth2.Token{AccessToken: "entra-token", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	if id, err := svc.CanRead(as, "github://acme/board"); err != nil || id != "a-gh" {
		t.Errorf("github source: CanRead = %q, %v; want a-gh", id, err)
	}
	if id, err := svc.CanRead(as, "msgraph://drives/b!x/root/Boards"); err != nil || id != "b-sp" {
		t.Errorf("msgraph source: CanRead = %q, %v; want b-sp", id, err)
	}
	if len(ghSeen) != 1 || ghSeen[0] != "gh-token" {
		t.Errorf("the GitHub check saw tokens %q, want only gh-token", ghSeen)
	}
	if len(graphSeen) != 1 || graphSeen[0] != "entra-token" {
		t.Errorf("the Graph check saw tokens %q, want only entra-token", graphSeen)
	}

	// A scheme with no check of its own falls back to WithReadCheck, which tries every kind.
	if _, err := svc.CanRead(as, "s3://designs/boards"); !errors.Is(err, connections.ErrNoAccess) {
		t.Errorf("s3 source: CanRead = %v, want ErrNoAccess from the fallback", err)
	}
	if len(otherSeen) != 2 {
		t.Errorf("the fallback check saw %d connections, want both", len(otherSeen))
	}
}

func TestAViewerWithNoConnectionOfTheKindGetsNoAccess(t *testing.T) {
	ctx := context.Background()
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOAPPLIB_TEST_GH_TOKEN", "gh-token")
	if err := store.PutConnection(ctx, connections.Connection{ID: "a-gh", Kind: connections.KindGitHubToken, Scope: connections.ScopePerson,
		Owner: "alice", TokenRef: "env:GOAPPLIB_TEST_GH_TOKEN"}); err != nil {
		t.Fatal(err)
	}
	called := false
	svc := connections.NewService(store, connections.WithReadCheckFor("msgraph",
		func(context.Context, string, string, mounts.Credentials) (bool, error) {
			called = true
			return false, errors.New("Graph rejected a GitHub token")
		}, connections.KindEntraUser))
	as := connections.WithViewer(ctx, "alice")
	if _, err := svc.CanRead(as, "msgraph://drives/b!x/root/Boards"); !errors.Is(err, connections.ErrNoAccess) || called {
		t.Errorf("CanRead = %v with the check called = %v; want ErrNoAccess and no call", err, called)
	}
	if _, err := svc.CanRead(as, "github://acme/board"); !errors.Is(err, connections.ErrNotConfigured) {
		t.Errorf("CanRead of a scheme with no check and no fallback = %v, want ErrNotConfigured", err)
	}
}
