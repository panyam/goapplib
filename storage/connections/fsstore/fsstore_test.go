package fsstore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/connectionstest"
	"github.com/panyam/goapplib/storage/connections/fsstore"
)

func TestContract(t *testing.T) {
	connectionstest.RunAll(t, func(t *testing.T) connections.Store {
		s, err := fsstore.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestTwoStoresShareOneFolder(t *testing.T) {
	dir := t.TempDir()
	a, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.PutConnection(ctx, connectionstest.Server("ci")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetConnection(ctx, "", "ci"); err != nil {
		t.Errorf("the second store doesn't see the first's record: %v", err)
	}
}

func TestOneYAMLFilePerConnection(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutConnection(context.Background(), connectionstest.Server("ci")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Error("ci.yaml is empty")
	}
}

func TestRecordWhoseNameDisagreesIsRefused(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.PutConnection(ctx, connectionstest.Server("ci")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "ci.yaml"), filepath.Join(dir, "other.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetConnection(ctx, "", "other"); err == nil {
		t.Error("GetConnection read a record filed under another ID")
	}
	if got, err := s.ListConnections(ctx, ""); err == nil && len(got) != 0 {
		t.Errorf("ListConnections listed a record filed under another ID: %+v", got)
	}
}

// oldPerson writes a person's record where goapplib 0.7.22 and before kept it, as <id>.yaml at
// the top of the folder.
func oldPerson(t *testing.T, dir, id, owner string) {
	t.Helper()
	b := "id: " + id + "\nkind: github-user\nscope: person\nowner: " + owner + "\nname: old\nstatus: ok\ncreated: 2026-11-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestOldPersonRecordReadsBackAsItsOwners(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldPerson(t, dir, "gh", "alice")
	ctx := context.Background()
	got, err := s.GetConnection(ctx, "alice", "gh")
	if err != nil || got.Owner != "alice" || got.Name != "old" {
		t.Fatalf("GetConnection(alice, gh) = %+v, %v; want the old record", got, err)
	}
	for _, owner := range []string{"", "bob"} {
		if _, err := s.GetConnection(ctx, owner, "gh"); !errors.Is(err, connections.ErrNotFound) {
			t.Errorf("GetConnection(%q, gh) = %v, want ErrNotFound", owner, err)
		}
	}
	if list, err := s.ListConnections(ctx, "alice"); err != nil || len(list) != 1 {
		t.Errorf("ListConnections(alice) = %+v, %v; want the old record", list, err)
	}
	if list, err := s.ListConnections(ctx, ""); err != nil || len(list) != 0 {
		t.Errorf("ListConnections(\"\") = %+v, %v; want none", list, err)
	}
}

func TestWritingAnOldRecordsIDMovesIt(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldPerson(t, dir, "gh", "alice")
	ctx := context.Background()
	// A server connection named gh would overwrite gh.yaml, so alice's record has to move first.
	if err := s.PutConnection(ctx, connectionstest.Server("gh")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetConnection(ctx, "alice", "gh"); err != nil || got.Name != "old" {
		t.Errorf("alice's gh after a server put = %+v, %v; want the old record", got, err)
	}
	if got, err := s.GetConnection(ctx, "", "gh"); err != nil || got.Scope != connections.ScopeServer {
		t.Errorf("the server's gh = %+v, %v", got, err)
	}
	moved, err := filepath.Glob(filepath.Join(dir, "people", "*", "gh.yaml"))
	if err != nil || len(moved) != 1 {
		t.Errorf("people/*/gh.yaml = %v, want alice's record moved there", moved)
	}
}

func TestDeletingAServerIDKeepsAnOldPersonRecord(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldPerson(t, dir, "gh", "alice")
	ctx := context.Background()
	if err := s.DeleteConnection(ctx, "", "gh"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetConnection(ctx, "alice", "gh"); err != nil {
		t.Errorf("deleting the server's gh removed alice's: %v", err)
	}
	if err := s.DeleteConnection(ctx, "alice", "gh"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetConnection(ctx, "alice", "gh"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("alice's gh after deleting it = %v, want ErrNotFound", err)
	}
}

func TestAnOwnerCantNameAPath(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, owner := range []string{"../x", "a/b", "..", "github|42"} {
		if err := s.PutConnection(ctx, connectionstest.Person("gh", owner)); err != nil {
			t.Fatalf("PutConnection(owner %q) = %v", owner, err)
		}
		if got, err := s.GetConnection(ctx, owner, "gh"); err != nil || got.Owner != owner {
			t.Errorf("GetConnection(%q, gh) = %+v, %v", owner, got, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "x" {
			t.Errorf("an owner of ../x wrote outside the store: %s", e.Name())
		}
	}
	people, _ := os.ReadDir(filepath.Join(dir, "people"))
	if len(people) != 4 {
		t.Errorf("people/ has %d folders, want one per owner (4)", len(people))
	}
}
