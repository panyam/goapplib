package gaestore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/panyam/goapplib/storage/backends/gae"
	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/connectionstest"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

// client connects to Datastore, or to the emulator when DATASTORE_EMULATOR_HOST is set. The tests
// skip unless DATASTORE_PROJECT_ID is set; make test-datastore starts the emulator and sets both.
func client(t *testing.T) *datastore.Client {
	t.Helper()
	project := os.Getenv("DATASTORE_PROJECT_ID")
	if project == "" {
		t.Skip("DATASTORE_PROJECT_ID not set; make test-datastore runs these against the emulator")
	}
	c, err := datastore.NewClient(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

var seq atomic.Int64

// namespace gives each test data of its own, so every factory call starts from an empty store.
func namespace(t *testing.T) string {
	name := strings.NewReplacer("/", "-", "_", "-").Replace(t.Name())
	if len(name) > 60 {
		name = name[len(name)-60:]
	}
	return fmt.Sprintf("t%d-%d-%s", time.Now().UnixNano(), seq.Add(1), name)
}

func TestMountStoreContract(t *testing.T) {
	c := client(t)
	mountstest.RunRecordStore(t, func(t *testing.T) mounts.RecordStore {
		return gaestore.NewMountStore(c, namespace(t))
	})
}

func TestMountStoreShared(t *testing.T) {
	c := client(t)
	mountstest.RunShared(t, func(t *testing.T) (a, b mounts.RecordStore) {
		ns := namespace(t)
		return gaestore.NewMountStore(c, ns), gaestore.NewMountStore(c, ns)
	})
}

func TestConnectionStoreContract(t *testing.T) {
	c := client(t)
	connectionstest.RunAll(t, func(t *testing.T) connections.Store {
		return gaestore.NewConnectionStore(c, namespace(t))
	})
}

// oldEntity is a person's connection as goapplib 0.7.22 and before stored it, with no parent key.
type oldEntity struct {
	Kind    string    `datastore:"kind,noindex"`
	Scope   string    `datastore:"scope,noindex"`
	Owner   string    `datastore:"owner"`
	Name    string    `datastore:"name,noindex"`
	Status  string    `datastore:"status,noindex"`
	Created time.Time `datastore:"created,noindex"`
}

func TestOldPersonEntityReadsBackAndMovesOnWrite(t *testing.T) {
	c, ns := client(t), namespace(t)
	ctx := context.Background()
	k := datastore.NameKey("GoapplibConnection", "gh", nil)
	k.Namespace = ns
	if _, err := c.Put(ctx, k, &oldEntity{Kind: "github-user", Scope: "person", Owner: "alice", Name: "old", Status: "ok",
		Created: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	s := gaestore.NewConnectionStore(c, ns)
	if got, err := s.GetConnection(ctx, "alice", "gh"); err != nil || got.Name != "old" {
		t.Fatalf("alice's old gh = %+v, %v", got, err)
	}
	if _, err := s.GetConnection(ctx, "", "gh"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("the server's gh = %v, want ErrNotFound", err)
	}
	if list, err := s.ListConnections(ctx, "alice"); err != nil || len(list) != 1 {
		t.Errorf("ListConnections(alice) = %+v, %v; want the old record", list, err)
	}
	// A server connection named gh would overwrite the old entity, so it has to move first.
	if err := s.PutConnection(ctx, connectionstest.Server("gh")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetConnection(ctx, "alice", "gh"); err != nil || got.Name != "old" {
		t.Errorf("alice's gh after a server put = %+v, %v", got, err)
	}
	if got, err := s.GetConnection(ctx, "", "gh"); err != nil || got.Scope != connections.ScopeServer {
		t.Errorf("the server's gh = %+v, %v", got, err)
	}
	if err := s.DeleteConnection(ctx, "", "gh"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetConnection(ctx, "alice", "gh"); err != nil {
		t.Errorf("deleting the server's gh removed alice's: %v", err)
	}
	if list, err := s.ListConnections(ctx, "alice"); err != nil || len(list) != 1 {
		t.Errorf("ListConnections(alice) after the move = %+v, %v; want one", list, err)
	}
}
