package github_test

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/github"
	"golang.org/x/oauth2"
)

// TestLiveGitHub checks the driver against real GitHub, so the fake can't drift from what GitHub
// answers. It reads a small public repo with no token, and skips unless STORAGE_LIVE_GITHUB=1.
// With STORAGE_LIVE_GITHUB_TOKEN set too, it lists a repo's tags through GraphQL, which needs a
// token.
func TestLiveGitHub(t *testing.T) {
	if os.Getenv("STORAGE_LIVE_GITHUB") != "1" {
		t.Skip("set STORAGE_LIVE_GITHUB=1 to run against api.github.com")
	}
	ctx := context.Background()
	b, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := github.New(b, github.WithViews(t.TempDir()))
	const src = "github://octocat/Hello-World"

	v, err := d.Resolve(ctx, src, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := d.Prepare(ctx, mounts.Mount{Name: "hello", Kind: mounts.KindGitHub, Source: src, Version: v, Mode: mounts.ModeSnapshot}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fsys, err := d.Open(ctx, snap, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(fsys, "README"); err != nil {
		t.Fatal(err)
	}
	p, err := d.HostPath(snap, "README")
	if err != nil {
		t.Fatal(err)
	}
	host, _ := os.ReadFile(p)

	live, err := d.Prepare(ctx, mounts.Mount{Name: "hello-live", Kind: mounts.KindGitHub, Source: src, Version: v, Mode: mounts.ModeLive}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lfs, _ := d.Open(ctx, live, nil)
	got, err := fs.ReadFile(lfs, "README")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Hello World") || string(host) != string(got) {
		t.Errorf("README from the snapshot view (%q) and the live mount (%q) differ, or miss \"Hello World\"", host, got)
	}
	if _, err := d.Stat(ctx, live, nil, "README"); err != nil {
		t.Errorf("Stat on real GitHub = %v", err)
	}

	commits, err := d.ListRefs(ctx, src, mounts.ListRefsRequest{Kind: mounts.RefCommit, PageSize: 1}, nil)
	if err != nil || len(commits.Refs) != 1 || commits.Refs[0].Version != v || commits.Refs[0].Time.IsZero() || commits.NextPageToken != "2" {
		t.Errorf("commits on the default branch = %+v, %v; want %s first, dated, and a next page", commits, err, v)
	}
	tok := os.Getenv("STORAGE_LIVE_GITHUB_TOKEN")
	if tok == "" {
		t.Log("STORAGE_LIVE_GITHUB_TOKEN unset, so tags aren't listed")
		return
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tok})
	tags, err := d.ListRefs(ctx, "github://golang/go", mounts.ListRefsRequest{Kind: mounts.RefTag, PageSize: 5}, ts)
	if err != nil || len(tags.Refs) != 5 || tags.NextPageToken == "" {
		t.Fatalf("golang/go tags = %+v, %v; want a full page and a next token", tags, err)
	}
	for i, r := range tags.Refs {
		if r.Version == "" || r.Time.IsZero() || i > 0 && r.Time.After(tags.Refs[i-1].Time) {
			t.Errorf("tag %d = %+v; want a commit and a date no newer than the tag before", i, r)
		}
	}
}
