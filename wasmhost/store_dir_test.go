//go:build !(js && wasm)

package wasmhost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDirStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "made-on-first-put")
	testStore(t, DirStore(dir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != "" || e.Name()[0] == '.' {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
	if got, err := DirStore(dir).Get(context.Background(), "big"); err != nil || len(got) != 3<<20 {
		t.Fatalf("a second DirStore over the same directory: %d bytes, %v", len(got), err)
	}
}
