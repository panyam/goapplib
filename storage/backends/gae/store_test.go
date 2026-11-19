package gaestore_test

import (
	"context"
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
