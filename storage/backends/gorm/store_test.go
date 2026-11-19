package gormstore_test

import (
	"path/filepath"
	"testing"

	"github.com/panyam/goapplib/storage/backends/gorm"
	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/connectionstest"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// open returns a SQLite database in a file, since each connection to ":memory:" is a separate
// database and the concurrency checks need one database shared by many connections.
// _txlock=immediate takes the write lock at BEGIN, so concurrent transactions wait on the busy
// timeout rather than failing when one upgrades from a read.
func open(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+path+"?_busy_timeout=10000&_txlock=immediate"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func mountStore(t *testing.T, path string) *gormstore.MountStore {
	t.Helper()
	s, err := gormstore.NewMountStore(open(t, path))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMountStoreContract(t *testing.T) {
	mountstest.RunRecordStore(t, func(t *testing.T) mounts.RecordStore {
		return mountStore(t, filepath.Join(t.TempDir(), "db.sqlite"))
	})
}

func TestMountStoreShared(t *testing.T) {
	mountstest.RunShared(t, func(t *testing.T) (a, b mounts.RecordStore) {
		path := filepath.Join(t.TempDir(), "db.sqlite")
		return mountStore(t, path), mountStore(t, path)
	})
}

func TestConnectionStoreContract(t *testing.T) {
	connectionstest.RunAll(t, func(t *testing.T) connections.Store {
		s, err := gormstore.NewConnectionStore(open(t, filepath.Join(t.TempDir(), "db.sqlite")))
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}
