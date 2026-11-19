package fsstore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

func newStore(t *testing.T) *fsstore.Store {
	t.Helper()
	s, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestContract(t *testing.T) {
	mountstest.RunRecordStore(t, func(t *testing.T) mounts.RecordStore { return newStore(t) })
}

func TestShared(t *testing.T) {
	mountstest.RunShared(t, func(t *testing.T) (a, b mounts.RecordStore) {
		dir := t.TempDir()
		sa, err := fsstore.New(dir)
		if err != nil {
			t.Fatal(err)
		}
		sb, err := fsstore.New(dir)
		if err != nil {
			t.Fatal(err)
		}
		return sa, sb
	})
}

func TestOneYAMLFilePerMount(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateMount(context.Background(), mountstest.Sample("jetson")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "jetson.yaml" {
		t.Fatalf("folder holds %v, want only jetson.yaml", entries)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "jetson.yaml"))
	if !strings.Contains(string(b), "source: github://acme/jetson") {
		t.Errorf("jetson.yaml doesn't read as YAML:\n%s", b)
	}
}

func TestListRefusesARecordWhoseNameDisagreesWithItsFile(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.CreateMount(ctx, mountstest.Sample("a")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.yaml"))
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListMounts(ctx); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("ListMounts = %v, want ErrInvalidArgument for b.yaml naming a", err)
	}
}
