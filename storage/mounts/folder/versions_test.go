package folder_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/folder"
)

func TestAFolderIsReadAtItsOnlyVersion(t *testing.T) {
	ctx := context.Background()
	m := tree(t)
	b, err := blobs.NewDir(t.TempDir())
	must(t, err)
	d := folder.New(folder.WithBlobs(b))

	if moves, err := d.Moves(ctx, m.Source, "", nil); err != nil || !moves {
		t.Errorf("Moves = %v, %v; want true, since a folder has no history", moves, err)
	}
	all, err := d.ListAt(ctx, m, nil, "", "")
	must(t, err)
	var got []string
	for _, f := range all {
		got = append(got, f.Path)
	}
	if want := []string{"a.txt", "sub/b.txt"}; !slices.Equal(got, want) {
		t.Errorf("ListAt of the root = %v, want %v with both links left out", got, want)
	}
	if one, err := d.ListAt(ctx, m, nil, "", "sub/b.txt"); err != nil || len(one) != 1 || one[0].Size != 4 {
		t.Errorf("ListAt of one file = %v, %v; want it with its size", one, err)
	}
	if none, err := d.ListAt(ctx, m, nil, "", "missing"); err != nil || len(none) != 0 {
		t.Errorf("ListAt of a missing path = %v, %v; want none", none, err)
	}
	if _, err := d.ListAt(ctx, m, nil, "", "outer-link"); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("ListAt through a link = %v, want ErrInvalidPath", err)
	}
	if _, err := d.ListAt(ctx, m, nil, "v1", ""); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("ListAt at a version = %v, want ErrInvalidArgument", err)
	}

	h, fetched, err := d.FetchAt(ctx, m, nil, "", all[0])
	if err != nil || !fetched || h != blobs.HashOf([]byte("alpha")) {
		t.Errorf("a first FetchAt = %q, %v, %v; want alpha's hash, kept", h, fetched, err)
	}
	if _, fetched, err := d.FetchAt(ctx, m, nil, "", all[0]); err != nil || fetched {
		t.Errorf("a second FetchAt = %v, %v; want nothing new kept", fetched, err)
	}
	must(t, os.WriteFile(filepath.Join(m.Source, "a.txt"), []byte("alpha 2"), 0o644))
	if h2, fetched, _ := d.FetchAt(ctx, m, nil, "", all[0]); !fetched || h2 == h {
		t.Errorf("FetchAt after the file changed = %q, %v; want the new contents kept", h2, fetched)
	}
	if _, _, err := folder.New().FetchAt(ctx, m, nil, "", all[0]); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("FetchAt without a blob store = %v, want ErrInvalidArgument", err)
	}
}

func TestAFolderVersionMovesWithSizesAndMtimesOnly(t *testing.T) {
	ctx := context.Background()
	m := tree(t)
	d := folder.New()
	resolve := func() string {
		t.Helper()
		v, err := d.Resolve(ctx, m.Source, "", nil)
		must(t, err)
		return v
	}
	v1 := resolve()
	if v2 := resolve(); v2 != v1 {
		t.Fatalf("two Resolves of an unchanged folder = %q and %q, want one version", v1, v2)
	}

	b := filepath.Join(m.Source, "sub", "b.txt")
	// A file nobody can read still counts, which shows Resolve reads no file's bytes.
	must(t, os.Chmod(b, 0))
	t.Cleanup(func() { os.Chmod(b, 0o644) })
	if v := resolve(); v != v1 {
		t.Errorf("Resolve after a chmod = %q, want %q, since only size and mtime count", v, v1)
	}

	later := time.Now().Add(time.Hour)
	must(t, os.Chtimes(b, later, later))
	v2 := resolve()
	if v2 == v1 {
		t.Errorf("Resolve after a touch = %q, want a new version", v2)
	}

	a := filepath.Join(m.Source, "a.txt")
	fi, err := os.Stat(a)
	must(t, err)
	must(t, os.WriteFile(a, []byte("alpha, longer"), 0o644))
	must(t, os.Chtimes(a, fi.ModTime(), fi.ModTime()))
	if v3 := resolve(); v3 == v2 {
		t.Errorf("Resolve after a resize with the mtime kept = %q, want a new version", v3)
	}

	if _, err := d.ListAt(ctx, m, nil, v1, ""); err != nil {
		t.Errorf("ListAt at an older mtime version = %v, want the folder as it is now", err)
	}
}

func TestAFolderListsNoRefs(t *testing.T) {
	page, err := folder.New().ListRefs(context.Background(), tree(t).Source, mounts.ListRefsRequest{Kind: mounts.RefTag}, nil)
	if err != nil || len(page.Refs) != 0 || page.NextPageToken != "" {
		t.Errorf("ListRefs = %+v, %v; want an empty page", page, err)
	}
}
