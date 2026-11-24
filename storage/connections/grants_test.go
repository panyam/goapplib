package connections_test

import (
	"errors"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/github"
)

// granted rebuilds a's service with grants beside the GitHub read check.
func (a *access) granted(grants ...connections.SourceGrant) {
	drv := github.New(nil, github.WithBaseURL(a.gh.URL))
	a.svc = connections.NewService(a.store,
		connections.WithReadCheck(drv.CanRead),
		connections.WithClock(func() time.Time { return a.now }),
		connections.WithSourceGrants(grants...),
	)
}

// wantGrant checks that viewer reads source through a grant, without a request to GitHub.
func (a *access) wantGrant(t *testing.T, viewer, source string) {
	t.Helper()
	n := len(a.gh.Requests())
	if id, err := a.svc.CanRead(a.as(viewer), source); err != nil || id != connections.GrantedBySource {
		t.Errorf("CanRead(%q, %q) = %q, %v; want GrantedBySource", viewer, source, id, err)
	}
	if got := len(a.gh.Requests()); got != n {
		t.Errorf("a granted CanRead(%q) made %d requests to GitHub, want 0", viewer, got-n)
	}
}

// wantNoAccess checks that viewer falls through the grants to the connection check, which
// finds no connection that can read source.
func (a *access) wantNoAccess(t *testing.T, viewer, source string) {
	t.Helper()
	if id, err := a.svc.CanRead(a.as(viewer), source); !errors.Is(err, connections.ErrNoAccess) || id != "" {
		t.Errorf("CanRead(%q, %q) = %q, %v; want ErrNoAccess", viewer, source, id, err)
	}
}

func TestGrantStarAdmitsAnyViewer(t *testing.T) {
	a := newAccess(t)
	a.granted(connections.SourceGrant{Source: a.src, Viewers: []string{"*"}})
	a.wantGrant(t, "alice", a.src)
	a.wantGrant(t, "local:carol", a.src)
	if id, err := a.svc.CanRead(a.ctx, a.src); !errors.Is(err, connections.ErrNotOwner) || id != "" {
		t.Errorf("CanRead with no viewer under a * grant = %q, %v; want ErrNotOwner", id, err)
	}
}

func TestGrantPrefixAdmitsOnlyItsViewers(t *testing.T) {
	a := newAccess(t)
	a.granted(connections.SourceGrant{Source: a.src, Viewers: []string{"local:*"}})
	a.wantGrant(t, "local:carol", a.src)
	a.wantNoAccess(t, "github:bob", a.src)
	a.wantNoAccess(t, "local:", a.src)
	a.wantNoAccess(t, "xlocal:carol", a.src)
}

func TestGrantExactViewerMatchesOnlyItself(t *testing.T) {
	a := newAccess(t)
	a.granted(connections.SourceGrant{Source: a.src, Viewers: []string{"local:carol"}})
	a.wantGrant(t, "local:carol", a.src)
	a.wantNoAccess(t, "local:caro", a.src)
	a.wantNoAccess(t, "local:carol2", a.src)
	a.wantNoAccess(t, "local:dave", a.src)
}

func TestGrantCoversOnlyItsSource(t *testing.T) {
	a := newAccess(t)
	a.granted(connections.SourceGrant{Source: a.src, Viewers: []string{"*"}})
	a.wantNoAccess(t, "alice", "github://acme/secret")
	// The match is exact, so another spelling of the repo goes to the connection check.
	a.wantNoAccess(t, "alice", "github://ACME/board")
	a.wantNoAccess(t, "alice", a.src+"/")
}

func TestGrantsKeepTheConnectionCheck(t *testing.T) {
	a := newAccess(t)
	a.put(t, "alice-gh", "alice", "ALICE_TOKEN", a.gh.URL)
	a.granted(
		connections.SourceGrant{Source: "github://acme/other", Viewers: []string{"*"}},
		connections.SourceGrant{Source: a.src, Viewers: []string{"local:*"}},
	)
	if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "alice-gh" {
		t.Errorf("CanRead with no matching grant = %q, %v; want alice-gh", id, err)
	}
	a.granted()
	if id, err := a.svc.CanRead(a.as("alice"), a.src); err != nil || id != "alice-gh" {
		t.Errorf("CanRead with no grants = %q, %v; want alice-gh", id, err)
	}
}

func TestGrantComesBeforeTheViewersConnections(t *testing.T) {
	a := newAccess(t)
	a.put(t, "alice-gh", "alice", "ALICE_TOKEN", a.gh.URL)
	a.granted(connections.SourceGrant{Source: a.src, Viewers: []string{"alice"}})
	a.wantGrant(t, "alice", a.src)
}

func TestGrantNeedsNoReadCheck(t *testing.T) {
	a := newAccess(t)
	a.svc = connections.NewService(a.store, connections.WithSourceGrants(
		connections.SourceGrant{Source: a.src, Viewers: []string{"local:*"}}))
	a.wantGrant(t, "local:carol", a.src)
	if _, err := a.svc.CanRead(a.as("github:bob"), a.src); !errors.Is(err, connections.ErrNotConfigured) {
		t.Errorf("CanRead outside the grant with no read check = %v, want ErrNotConfigured", err)
	}
}

func TestGrantedBySourceIsNoConnectionID(t *testing.T) {
	if mounts.ValidName(connections.GrantedBySource) == nil {
		t.Errorf("GrantedBySource %q is a valid connection ID, so a caller can't tell a grant from a connection",
			connections.GrantedBySource)
	}
}

func TestSourceGrantValidate(t *testing.T) {
	src := "github://acme/board"
	for _, g := range []connections.SourceGrant{
		{Source: src, Viewers: []string{"*"}},
		{Source: src, Viewers: []string{"local:*", "github:bob", "oidc:group:eng*"}},
		{Source: "msgraph://contoso/sites/eng", Viewers: []string{"local:carol"}},
	} {
		if err := g.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v, want nil", g, err)
		}
	}
	for name, g := range map[string]connections.SourceGrant{
		"a star inside":      {Source: src, Viewers: []string{"lo*cal:*"}},
		"an empty viewer":    {Source: src, Viewers: []string{""}},
		"two stars":          {Source: src, Viewers: []string{"**"}},
		"a leading star":     {Source: src, Viewers: []string{"*:carol"}},
		"one bad of two":     {Source: src, Viewers: []string{"local:*", "*bob"}},
		"no viewers":         {Source: src},
		"no source":          {Viewers: []string{"*"}},
		"no scheme":          {Source: "acme/board", Viewers: []string{"*"}},
		"nothing after ://":  {Source: "github://", Viewers: []string{"*"}},
		"a star in a source": {Source: "github://acme/*", Viewers: []string{"*"}},
	} {
		if err := g.Validate(); !errors.Is(err, connections.ErrInvalidGrant) {
			t.Errorf("Validate with %s = %v, want ErrInvalidGrant", name, err)
		}
	}
}

func TestAnInvalidGrantAdmitsNobody(t *testing.T) {
	a := newAccess(t)
	a.granted(connections.SourceGrant{Source: a.src, Viewers: []string{"lo*cal:*", "", "**", "*carol"}})
	for _, viewer := range []string{"local:carol", "lo*cal:x", "lo*cal:*", "**", "*carol", "carol", "xcarol"} {
		a.wantNoAccess(t, viewer, a.src)
	}
}
