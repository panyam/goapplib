package blobs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/panyam/goapplib/storage/blobs"
)

func TestAliasesNameBlobsAndSurviveASweep(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	d, err := blobs.NewDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := d.Alias(ctx, "git", "abc"); err != nil || ok {
		t.Errorf("an unset alias = %v, %v; want none", ok, err)
	}
	h := blobs.HashOf([]byte("x"))
	if err := d.SetAlias(ctx, "git", "abc", h); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := d.Alias(ctx, "git", "abc"); err != nil || !ok || got != h {
		t.Errorf("Alias = %q, %v, %v; want %q", got, ok, err, h)
	}
	other := blobs.HashOf([]byte("y"))
	if err := d.SetAlias(ctx, "git", "abc", other); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := d.Alias(ctx, "git", "abc"); got != other {
		t.Errorf("a replaced alias = %q, want %q", got, other)
	}
	if _, err := d.Sweep(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := d.Alias(ctx, "git", "abc"); !ok {
		t.Errorf("a sweep removed an alias; it should walk the blobs alone")
	}
	for _, bad := range [][2]string{{"..", "x"}, {"git", "../x"}, {"git", ""}, {"a/b", "x"}} {
		if err := d.SetAlias(ctx, bad[0], bad[1], h); !errors.Is(err, blobs.ErrInvalidAlias) {
			t.Errorf("SetAlias(%q, %q) = %v, want ErrInvalidAlias", bad[0], bad[1], err)
		}
	}
	if err := d.SetAlias(ctx, "git", "def", "sha256:nope"); !errors.Is(err, blobs.ErrInvalidHash) {
		t.Errorf("SetAlias to a bad hash = %v, want ErrInvalidHash", err)
	}
	if err := os.WriteFile(filepath.Join(root, "aliases", "git", "junk"), []byte("not a hash"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := d.Alias(ctx, "git", "junk"); err != nil || ok {
		t.Errorf("an alias file holding no hash = %v, %v; want none", ok, err)
	}
}

func TestCachedKeepsAliasesInItsLocalDir(t *testing.T) {
	ctx := context.Background()
	remote, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	local, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var c blobs.Aliases = blobs.NewCached(remote, local)
	h := blobs.HashOf([]byte("x"))
	if err := c.SetAlias(ctx, "bucket", "abc", h); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := c.Alias(ctx, "bucket", "abc"); err != nil || !ok || got != h {
		t.Errorf("Alias = %s, %v, %v; want %s", got, ok, err, h)
	}
	if got, ok, err := local.Alias(ctx, "bucket", "abc"); err != nil || !ok || got != h {
		t.Errorf("the local Dir's alias = %s, %v, %v; want %s", got, ok, err, h)
	}
	if err := c.SetAlias(ctx, "bucket", "../up", h); !errors.Is(err, blobs.ErrInvalidAlias) {
		t.Errorf("SetAlias with a bad id = %v, want ErrInvalidAlias", err)
	}
}
