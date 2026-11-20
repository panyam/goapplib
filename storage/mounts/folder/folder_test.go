package folder_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/folder"
)

// tree makes a folder with a file, a subfolder, and two links: one to a file inside, one to a
// folder outside.
func tree(t *testing.T) mounts.Mount {
	t.Helper()
	root, outside := t.TempDir(), t.TempDir()
	must(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0o644))
	must(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("beta"), 0o644))
	must(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("nope"), 0o644))
	must(t, os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, "inner-link")))
	must(t, os.Symlink(outside, filepath.Join(root, "outer-link")))
	return mounts.Mount{Name: "notes", Kind: mounts.KindFolder, Source: root, Mode: mounts.ModeLive}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	d, ctx := folder.New(), context.Background()
	m := tree(t)
	if v, err := d.Resolve(ctx, m.Source, "", nil); err != nil || !strings.HasPrefix(v, "mtime:") || len(v) != len("mtime:")+64 {
		t.Errorf("Resolve(dir) = %q, %v; want mtime: and a SHA-256", v, err)
	}
	if _, err := d.Resolve(ctx, "relative/dir", "", nil); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("Resolve(relative) = %v, want ErrInvalidArgument", err)
	}
	if _, err := d.Resolve(ctx, filepath.Join(m.Source, "missing"), "", nil); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Resolve(missing) = %v, want ErrNotFound", err)
	}
	if _, err := d.Resolve(ctx, filepath.Join(m.Source, "a.txt"), "", nil); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("Resolve(file) = %v, want ErrInvalidArgument", err)
	}
	if _, err := d.Resolve(ctx, m.Source, "main", nil); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("Resolve with a ref = %v, want ErrInvalidArgument", err)
	}
}

func TestOpenHidesAndRefusesLinks(t *testing.T) {
	d, ctx := folder.New(), context.Background()
	m := tree(t)
	fsys, err := d.Open(ctx, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := fs.ReadFile(fsys, "sub/b.txt"); err != nil || string(b) != "beta" {
		t.Errorf("ReadFile(sub/b.txt) = %q, %v", b, err)
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 || names[0] != "a.txt" || names[1] != "sub" {
		t.Errorf("ReadDir(.) = %v, want [a.txt sub]", names)
	}
	dir, err := fsys.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := dir.(fs.ReadDirFile).ReadDir(-1)
	dir.Close()
	if err != nil || len(listed) != 2 {
		t.Errorf("Open(.).ReadDir = %d entries, %v; want 2", len(listed), err)
	}
	for _, p := range []string{"inner-link", "outer-link/secret", "outer-link"} {
		if _, err := fsys.Open(p); !errors.Is(err, mounts.ErrInvalidPath) {
			t.Errorf("Open(%q) = %v, want ErrInvalidPath", p, err)
		}
	}
	for _, p := range []string{"../x", "/etc/passwd"} {
		if _, err := fsys.Open(p); err == nil {
			t.Errorf("Open(%q) succeeded", p)
		}
	}
	walked := 0
	if err := fs.WalkDir(fsys, ".", func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		walked++
		return nil
	}); err != nil || walked != 4 {
		t.Errorf("WalkDir saw %d entries, %v; want 4 (., a.txt, sub, sub/b.txt)", walked, err)
	}
}

func TestPutAndDeleteWithIfMatch(t *testing.T) {
	d, ctx := folder.New(), context.Background()
	m := tree(t)

	if _, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "a.txt", Data: []byte("x")}); !errors.Is(err, mounts.ErrConflict) {
		t.Errorf("create-only Put over an existing file = %v, want ErrConflict", err)
	}
	res, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "new/c.txt", Data: []byte("gamma")})
	if err != nil {
		t.Fatalf("create-only Put of a new file = %v", err)
	}
	if v, err := d.Stat(ctx, m, nil, "new/c.txt"); err != nil || v != res.File || v == "" {
		t.Errorf("Stat = %q, %v; want %q", v, err, res.File)
	}

	v, err := d.Stat(ctx, m, nil, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "a.txt", Data: []byte("alpha2"), IfMatch: v}); err != nil {
		t.Fatalf("Put with the current version = %v", err)
	}
	if _, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "a.txt", Data: []byte("alpha3"), IfMatch: v}); !errors.Is(err, mounts.ErrConflict) {
		t.Errorf("Put with a stale version = %v, want ErrConflict", err)
	}
	if _, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "a.txt", Data: []byte("alpha4"), IfMatch: mounts.AnyVersion}); err != nil {
		t.Errorf("Put with AnyVersion = %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(m.Source, "a.txt")); string(b) != "alpha4" {
		t.Errorf("a.txt = %q, want alpha4", b)
	}

	if _, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "inner-link", Data: []byte("x"), IfMatch: mounts.AnyVersion}); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("Put through a link = %v, want ErrInvalidPath", err)
	}
	if _, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "outer-link/secret", Data: []byte("x"), IfMatch: mounts.AnyVersion}); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("Put through a folder link = %v, want ErrInvalidPath", err)
	}
	if _, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "../escape", Data: []byte("x"), IfMatch: mounts.AnyVersion}); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("Put climbing out = %v, want ErrInvalidPath", err)
	}

	if _, err := d.Delete(ctx, m, nil, mounts.DeleteRequest{Path: "new/c.txt", IfMatch: "sha256:stale"}); !errors.Is(err, mounts.ErrConflict) {
		t.Errorf("Delete with a stale version = %v, want ErrConflict", err)
	}
	if _, err := d.Delete(ctx, m, nil, mounts.DeleteRequest{Path: "new/c.txt", IfMatch: res.File}); err != nil {
		t.Errorf("Delete with the current version = %v", err)
	}
	if _, err := d.Stat(ctx, m, nil, "new/c.txt"); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Stat after delete = %v, want ErrNotFound", err)
	}
	if _, err := d.Delete(ctx, m, nil, mounts.DeleteRequest{Path: "new/c.txt", IfMatch: mounts.AnyVersion}); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Delete of a missing file = %v, want ErrNotFound", err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(m.Source, "*.tmp*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestHostPath(t *testing.T) {
	d := folder.New()
	m := tree(t)
	if p, err := d.HostPath(m, "sub/b.txt"); err != nil || p != filepath.Join(m.Source, "sub", "b.txt") {
		t.Errorf("HostPath = %q, %v", p, err)
	}
	if p, err := d.HostPath(m, ""); err != nil || p != m.Source {
		t.Errorf("HostPath(root) = %q, %v", p, err)
	}
	for _, p := range []string{"../x", "outer-link/secret", "inner-link"} {
		if _, err := d.HostPath(m, p); !errors.Is(err, mounts.ErrInvalidPath) {
			t.Errorf("HostPath(%q) = %v, want ErrInvalidPath", p, err)
		}
	}
}
