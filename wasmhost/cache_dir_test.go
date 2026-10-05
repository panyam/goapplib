//go:build !(js && wasm)

package wasmhost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDirCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "made-on-first-put")
	testCache(t, DirCache(dir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != "" || e.Name()[0] == '.' {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
	if got, err := DirCache(dir).Get(context.Background(), "big"); err != nil || len(got) != 3<<20 {
		t.Fatalf("a second DirCache over the same directory: %d bytes, %v", len(got), err)
	}
}
