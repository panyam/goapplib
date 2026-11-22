package msgraph_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/entratest"
	"github.com/panyam/goapplib/storage/connections/fsstore"
	"github.com/panyam/goapplib/storage/mounts"
	oneauthfs "github.com/panyam/oneauth/stores/fs"
	"golang.org/x/oauth2"
)

func bearer(tok string) mounts.Credentials {
	return mounts.Bearer(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tok}))
}

func TestCanReadAsksForTheFolder(t *testing.T) {
	g, d := fake(t, true)
	g.AddToken("alice")
	g.AddToken("bob")
	d.Put("Boards/README.md", "the jetson baseboard")
	d.Grant("tok", "alice")
	drv := newDriver(t, g)
	ctx := context.Background()

	for name, c := range map[string]struct {
		source, tok string
		want        bool
	}{
		"a granted token on the site's folder":  {source, "alice", true},
		"a granted token on the drive's folder": {"msgraph://drives/" + driveID + "/root/Boards", "alice", true},
		"a granted token on the drive's root":   {"msgraph://drives/" + driveID + "/root", "alice", true},
		"me/drive":                              {"msgraph://me/drive/root/Boards", "alice", true},
		"a token the drive doesn't grant":       {source, "bob", false},
		"a folder that doesn't exist":           {"msgraph://drives/" + driveID + "/root/Nope", "alice", false},
		"a drive that doesn't exist":            {"msgraph://drives/b!nope/root/Boards", "alice", false},
	} {
		got, err := drv.CanRead(ctx, "", c.source, bearer(c.tok))
		if err != nil || got != c.want {
			t.Errorf("%s: CanRead = %v, %v; want %v", name, got, err, c.want)
		}
	}
	if _, err := drv.CanRead(ctx, "", source, bearer("rejected")); !errors.Is(err, mounts.ErrPermissionDenied) {
		t.Errorf("CanRead with a token Graph rejects = %v, want an ErrPermissionDenied error, since it says nothing about access", err)
	}
	for _, bad := range []string{"github://acme/board", "msgraph://groups/x/root", "msgraph://drives/" + driveID} {
		if _, err := drv.CanRead(ctx, "", bad, bearer("alice")); !errors.Is(err, mounts.ErrInvalidArgument) {
			t.Errorf("CanRead(%q) = %v, want ErrInvalidArgument", bad, err)
		}
	}
}

// Two people sign in to SharePoint through the app. Only alice can open the library, so CanRead
// names her connection and tells bob no (agni#1729).
func TestCanReadThroughEachPersonsEntraSignIn(t *testing.T) {
	const tenant, clientID = "contoso.onmicrosoft.com", "app-1"
	ent := entratest.New(t)
	ent.AddSecretApp(tenant, clientID, "s3cret")
	g, d := fake(t, true)
	g.AcceptToken(ent.ValidToken)
	d.Put("Boards/README.md", "the jetson baseboard")
	d.Allow(func(tok string) bool { who, _ := ent.SubjectOf(tok); return who == "alice" })
	drv := newDriver(t, g)

	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOAPPLIB_TEST_GRAPH_SECRET", "s3cret")
	t.Setenv("GOAPPLIB_TEST_CONN_KEY", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	svc := connections.NewService(store,
		connections.WithUserTokens(oneauthfs.NewFSUserTokenStore(t.TempDir()), "env:GOAPPLIB_TEST_CONN_KEY", nil),
		connections.WithReadCheckFor("msgraph", drv.CanRead, connections.KindEntraUser))
	ctx := context.Background()
	for _, who := range []string{"alice", "bob"} {
		if err := store.PutConnection(ctx, connections.Connection{ID: who + "-sp", Kind: connections.KindEntraUser, Scope: connections.ScopePerson,
			Owner: who, BaseURL: ent.URL, TenantID: tenant, ClientID: clientID, KeyRef: "env:GOAPPLIB_TEST_GRAPH_SECRET"}); err != nil {
			t.Fatal(err)
		}
		signed := ent.SignIn(tenant, clientID, who)
		// Expired already, so the first use refreshes it at entratest.
		if err := svc.SaveUserToken(connections.WithViewer(ctx, who), who, who+"-sp",
			&oauth2.Token{AccessToken: signed.AccessToken, RefreshToken: signed.RefreshToken, Expiry: time.Now().Add(-time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}

	if id, err := svc.CanRead(connections.WithViewer(ctx, "alice"), source); err != nil || id != "alice-sp" {
		t.Errorf("alice: CanRead = %q, %v; want her connection alice-sp", id, err)
	}
	if id, err := svc.CanRead(connections.WithViewer(ctx, "bob"), source); !errors.Is(err, connections.ErrNoAccess) {
		t.Errorf("bob: CanRead = %q, %v; want ErrNoAccess", id, err)
	}
	refreshed := 0
	for _, r := range ent.Requests() {
		if r.GrantType == "refresh_token" {
			refreshed++
		}
	}
	if refreshed != 2 {
		t.Errorf("entratest saw %d refreshes, want one per person", refreshed)
	}
}
