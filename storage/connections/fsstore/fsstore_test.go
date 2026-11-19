package fsstore_test

import (
	"context"
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
	if _, err := b.GetConnection(ctx, "ci"); err != nil {
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
	if _, err := s.GetConnection(ctx, "other"); err == nil {
		t.Error("GetConnection read a record filed under another ID")
	}
	if got, err := s.ListConnections(ctx, ""); err == nil && len(got) != 0 {
		t.Errorf("ListConnections listed a record filed under another ID: %+v", got)
	}
}
