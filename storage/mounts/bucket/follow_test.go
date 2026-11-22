package bucket_test

import (
	"context"
	"slices"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
)

func TestContainsSource(t *testing.T) {
	d, _, _ := newDriver(t, true, nil)
	for _, c := range []struct {
		mount, changed string
		want           bool
	}{
		{"s3://designs/boards", "s3://designs/boards/jetson/README.md", true},
		{"s3://designs", "s3://designs/boards/jetson/README.md", true},
		{"s3://designs/boards/jetson", "s3://designs/boards/jetson", true},
		{"s3://designs/boards/jet", "s3://designs/boards/jetson/README.md", false},
		{"s3://designs/boards", "s3://other/boards/README.md", false},
		{"s3://designs/boards", "gs://designs/boards/README.md", false},
		{"not a source", "s3://designs/boards/README.md", false},
	} {
		if got := d.ContainsSource(c.mount, c.changed); got != c.want {
			t.Errorf("ContainsSource(%q, %q) = %v, want %v", c.mount, c.changed, got, c.want)
		}
	}
}

func TestFollowingFindsBucketMountsUnderAnObject(t *testing.T) {
	ctx := context.Background()
	d, b, _ := newDriver(t, true, map[string]string{"README.md": "rev 1"})
	b.Put("other/notes.md", []byte("x"))
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(store, []mounts.Driver{d})
	for name, src := range map[string]string{"jetson": "s3://designs/boards/jetson", "boards": "s3://designs/boards", "other": "s3://designs/other"} {
		mode := mounts.ModeLive
		if name == "jetson" {
			mode = mounts.ModeSnapshot
		}
		if _, err := reg.Create(ctx, mounts.Mount{Name: name, Kind: mounts.KindS3, Source: src, Mode: mode}); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}
	ms, err := reg.Following(ctx, mounts.KindS3, mounts.Change{Source: "s3://designs/boards/jetson/README.md"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range ms {
		got = append(got, m.Name)
	}
	slices.Sort(got)
	if want := []string{"boards", "jetson"}; !slices.Equal(got, want) {
		t.Errorf("Following = %v, want %v", got, want)
	}
}
