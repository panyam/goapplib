package gormstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	gormstore "github.com/panyam/goapplib/storage/backends/gorm"
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

// oldRow is a row of the connection table as goapplib 0.7.22 and before made it, keyed by ID.
type oldRow struct {
	ID       string    `gorm:"column:id;primaryKey;size:64"`
	Kind     string    `gorm:"column:kind;size:32"`
	Scope    string    `gorm:"column:scope;size:16"`
	Owner    string    `gorm:"column:owner;size:256;index"`
	Name     string    `gorm:"column:name"`
	TokenRef string    `gorm:"column:token_ref"`
	Status   string    `gorm:"column:status;size:16"`
	Created  time.Time `gorm:"column:created"`
}

func (oldRow) TableName() string { return gormstore.OldConnectionTable }

func TestRecordsFromTheOldTableCopyAcrossOnce(t *testing.T) {
	db := open(t, filepath.Join(t.TempDir(), "old.db"))
	if err := db.AutoMigrate(&oldRow{}); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if err := db.Create([]oldRow{
		{ID: "ci", Kind: "github-token", Scope: "server", Name: "CI", TokenRef: "env:CI", Status: "ok", Created: created},
		{ID: "gh", Kind: "github-user", Scope: "person", Owner: "alice", Name: "old", Status: "ok", Created: created},
	}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := gormstore.NewConnectionStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := s.GetConnection(ctx, "", "ci"); err != nil || c.TokenRef != "env:CI" || !c.Created.Equal(created) {
		t.Errorf("the server's ci = %+v, %v", c, err)
	}
	if c, err := s.GetConnection(ctx, "alice", "gh"); err != nil || c.Name != "old" {
		t.Errorf("alice's gh = %+v, %v", c, err)
	}
	// bob can now make his own gh, and opening the store again doesn't copy the old rows over it.
	if err := s.PutConnection(ctx, connectionstest.Person("gh", "bob")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConnection(ctx, "", "ci"); err != nil {
		t.Fatal(err)
	}
	s2, err := gormstore.NewConnectionStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.GetConnection(ctx, "", "ci"); !errors.Is(err, connections.ErrNotFound) {
		t.Errorf("the deleted ci came back on reopening: %v", err)
	}
	if _, err := s2.GetConnection(ctx, "bob", "gh"); err != nil {
		t.Errorf("bob's gh after reopening: %v", err)
	}
}
