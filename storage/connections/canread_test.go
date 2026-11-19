package connections_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/fsstore"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/github"
	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
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
