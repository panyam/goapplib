// Package connectionstest is the contract suite for connections.Store. Every backend runs it, so
// an app can swap one store for another and keep the same behavior:
//
//	func TestStore(t *testing.T) {
//		connectionstest.RunAll(t, func(t *testing.T) connections.Store {
//			s, err := fsstore.New(t.TempDir())
//			if err != nil {
//				t.Fatal(err)
//			}
//			return s
//		})
//	}
package connectionstest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/connections"
)

// Factory returns a new, empty store for one subtest.
type Factory func(t *testing.T) connections.Store

// RunAll runs every contract test against stores from factory, each as a subtest with a store of
// its own.
func RunAll(t *testing.T, factory Factory) {
	tests := []struct {
		name string
		run  func(t *testing.T, s connections.Store)
	}{
		{"GetMissing", testGetMissing},
		{"PutThenGet", testPutThenGet},
		{"PutReplaces", testPutReplaces},
		{"PutValidates", testPutValidates},
		{"ListByOwner", testListByOwner},
		{"DeleteIsIdempotent", testDeleteIsIdempotent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, factory(t)) })
	}
}

// Server is a valid server-scoped token connection with the given ID.
func Server(id string) connections.Connection {
	return connections.Connection{
		ID: id, Kind: connections.KindGitHubToken, Scope: connections.ScopeServer,
		Name: "CI token", TokenRef: "env:GITHUB_TOKEN", Status: connections.StatusOK,
		Created: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
	}
}

// Person is a valid person-scoped GitHub user connection with the given ID and owner.
func Person(id, owner string) connections.Connection {
	return connections.Connection{
		ID: id, Kind: connections.KindGitHubUser, Scope: connections.ScopePerson, Owner: owner,
		Name: owner + "'s GitHub", Status: connections.StatusOK,
		Created: time.Date(2026, 11, 18, 11, 0, 0, 0, time.UTC),
	}
}

func testGetMissing(t *testing.T, s connections.Store) {
	if _, err := s.GetConnection(context.Background(), "nope"); !errors.Is(err, connections.ErrNotFound) {
		t.Fatalf("GetConnection(missing) = %v, want ErrNotFound", err)
	}
}

func testPutThenGet(t *testing.T, s connections.Store) {
	ctx := context.Background()
	want := connections.Connection{
		ID: "acme", Kind: connections.KindGitHubApp, Scope: connections.ScopeServer,
		Name: "acme org", BaseURL: "https://ghe.example/api/v3", AppID: "12345", InstallationID: 7,
		KeyRef: "file:/run/secrets/app.pem", Status: connections.StatusFailed, LastError: "boom",
		Created:  time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC),
		LastUsed: time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC),
	}
	if err := s.PutConnection(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetConnection(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created.Equal(want.Created) || !got.LastUsed.Equal(want.LastUsed) {
		t.Errorf("times = %v, %v; want %v, %v", got.Created, got.LastUsed, want.Created, want.LastUsed)
	}
	got.Created, got.LastUsed = want.Created, want.LastUsed
	if got != want {
		t.Errorf("GetConnection = %+v, want %+v", got, want)
	}
}

func testPutReplaces(t *testing.T, s connections.Store) {
	ctx := context.Background()
	c := Server("ci")
	if err := s.PutConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.Status, c.LastError = connections.StatusFailed, "rate limited"
	if err := s.PutConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetConnection(ctx, "ci")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != connections.StatusFailed || got.LastError != "rate limited" {
		t.Errorf("after replace = %+v", got)
	}
}

func testPutValidates(t *testing.T, s connections.Store) {
	ctx := context.Background()
	c := Server("ci")
	c.TokenRef = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"
	if err := s.PutConnection(ctx, c); !errors.Is(err, connections.ErrInvalid) {
		t.Fatalf("PutConnection(raw token) = %v, want ErrInvalid", err)
	}
	if _, err := s.GetConnection(ctx, "ci"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("an invalid record was stored: GetConnection = %v", err)
	}
}

func testListByOwner(t *testing.T, s connections.Store) {
	ctx := context.Background()
	for _, c := range []connections.Connection{
		Person("bob-gh", "bob"), Server("ci"), Person("alice-gh", "alice"),
		Person("alice-ghe", "alice"), Server("acme"),
	} {
		if err := s.PutConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	for owner, want := range map[string][]string{
		"alice": {"alice-gh", "alice-ghe"},
		"bob":   {"bob-gh"},
		"":      {"acme", "ci"},
		"carol": nil,
	} {
		got, err := s.ListConnections(ctx, owner)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, c := range got {
			ids = append(ids, c.ID)
		}
		if len(ids) != len(want) {
			t.Errorf("ListConnections(%q) = %v, want %v", owner, ids, want)
			continue
		}
		for i := range ids {
			if ids[i] != want[i] {
				t.Errorf("ListConnections(%q) = %v, want %v", owner, ids, want)
				break
			}
		}
	}
}

func testDeleteIsIdempotent(t *testing.T, s connections.Store) {
	ctx := context.Background()
	if err := s.PutConnection(ctx, Server("ci")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.DeleteConnection(ctx, "ci"); err != nil {
			t.Fatalf("DeleteConnection #%d = %v", i+1, err)
		}
	}
	if _, err := s.GetConnection(ctx, "ci"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("after delete, GetConnection = %v, want ErrNotFound", err)
	}
}
