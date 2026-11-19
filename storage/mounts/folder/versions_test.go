package folder_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

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
