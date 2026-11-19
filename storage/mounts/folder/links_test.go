package folder_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/folder"
)

// linkedTree is a folder whose libraries are links: a relative link to a file inside, a link to a
// folder inside, a loop back up, a relative link out, and an absolute link to a file inside, which
// os.Root treats as leading out.
func linkedTree(t *testing.T) mounts.Mount {
	t.Helper()
	root, outside := t.TempDir(), t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, "libs", "real"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "libs", "real", "r.kicad_sym"), []byte("(sym)"), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "top.kicad_sch"), []byte("(sch)"), 0o644))
	must(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("nope"), 0o644))
	must(t, os.Symlink("real/r.kicad_sym", filepath.Join(root, "libs", "file-link.kicad_sym")))
	must(t, os.Symlink("libs/real", filepath.Join(root, "linked-libs")))
	must(t, os.Symlink("..", filepath.Join(root, "libs", "real", "loop")))
	must(t, os.Symlink(filepath.Join("..", filepath.Base(outside), "secret"), filepath.Join(root, "out-link")))
	must(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "abs-out")))
	must(t, os.Symlink(filepath.Join(root, "top.kicad_sch"), filepath.Join(root, "abs-in")))
	return mounts.Mount{Name: "boards", Kind: mounts.KindFolder, Source: root, Mode: mounts.ModeLive}
}

func TestLinksInsideAreFollowedAndLinksOutAreNot(t *testing.T) {
	ctx := context.Background()
	m := linkedTree(t)
	b, err := blobs.NewDir(t.TempDir())
	must(t, err)
	d := folder.New(folder.WithLinksInside(), folder.WithBlobs(b))
	fsys, err := d.Open(ctx, m, nil)
	must(t, err)

	if got, err := fs.ReadFile(fsys, "libs/file-link.kicad_sym"); err != nil || string(got) != "(sym)" {
		t.Errorf("reading a link to a file inside = %q, %v; want its target's contents", got, err)
	}
	if got, err := fs.ReadFile(fsys, "linked-libs/r.kicad_sym"); err != nil || string(got) != "(sym)" {
		t.Errorf("reading through a link to a folder inside = %q, %v", got, err)
	}
	for _, p := range []string{"out-link", "abs-out", "abs-in"} {
		if _, err := fs.ReadFile(fsys, p); !errors.Is(err, mounts.ErrInvalidPath) {
			t.Errorf("reading %s = %v, want ErrInvalidPath", p, err)
		}
	}
	entries, err := fs.ReadDir(fsys, ".")
	must(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{"libs", "linked-libs", "top.kicad_sch"}; !slices.Equal(names, want) {
		t.Errorf("the root lists %v, want %v with the links out left out", names, want)
	}
	if p, err := d.HostPath(m, "libs/file-link.kicad_sym"); err != nil || p != filepath.Join(m.Source, "libs", "file-link.kicad_sym") {
		t.Errorf("HostPath of a link inside = %q, %v", p, err)
	}
	if _, err := d.HostPath(m, "out-link"); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("HostPath of a link out = %v, want ErrInvalidPath", err)
	}

	files, err := d.ListAt(ctx, m, nil, "", "")
	must(t, err)
	var listed []string
	for _, f := range files {
		listed = append(listed, f.Path)
	}
	want := []string{"libs/file-link.kicad_sym", "libs/real/r.kicad_sym", "top.kicad_sch"}
	if !slices.Equal(listed, want) {
		t.Errorf("ListAt = %v, want %v: each real folder once, so linked-libs and the loop add nothing", listed, want)
	}
	if _, fetched, err := d.FetchAt(ctx, m, nil, "", mounts.File{Path: "libs/file-link.kicad_sym"}); err != nil || !fetched {
		t.Errorf("FetchAt through a link inside = %v, %v", fetched, err)
	}

	if _, err := d.Put(ctx, m, nil, mounts.PutRequest{Path: "libs/file-link.kicad_sym", Data: []byte("x"), IfMatch: mounts.AnyVersion}); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("a write through a link = %v, want ErrInvalidPath, since writes never follow links", err)
	}
}

func TestWithoutTheOptionEveryLinkIsRefused(t *testing.T) {
	m := linkedTree(t)
	fsys, err := folder.New().Open(context.Background(), m, nil)
	must(t, err)
	if _, err := fs.ReadFile(fsys, "libs/file-link.kicad_sym"); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("reading a link inside without WithLinksInside = %v, want ErrInvalidPath", err)
	}
}
