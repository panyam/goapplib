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
	// With no token, tags come from the REST tags API, which orders goapplib's by version.
	anon, err := d.ListRefs(ctx, "github://panyam/goapplib", mounts.ListRefsRequest{Kind: mounts.RefTag, PageSize: 3}, nil)
	if err != nil || len(anon.Refs) != 3 || anon.NextPageToken != "2" {
		t.Fatalf("goapplib tags with no token = %+v, %v; want three and a next page", anon, err)
	}
	for _, r := range anon.Refs {
		if !strings.HasPrefix(r.Name, "v") || len(r.Version) != 40 || r.Time.IsZero() {
			t.Errorf("tag with no token = %+v; want a v tag, its commit and a date", r)
		}
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

// TestLiveGitHubLFS reads one LFS file through a snapshot and a live mount on real GitHub. It
// skips unless STORAGE_LIVE_GITHUB_LFS names the file as "owner/repo:path", since no small public
// LFS fixture is known to stay put. STORAGE_LIVE_GITHUB_TOKEN is sent when set, for a private repo.
func TestLiveGitHubLFS(t *testing.T) {
	spec := os.Getenv("STORAGE_LIVE_GITHUB_LFS")
	if spec == "" {
		t.Skip("set STORAGE_LIVE_GITHUB_LFS=owner/repo:path to read an LFS file from github.com")
	}
	rp, p, ok := strings.Cut(spec, ":")
	if !ok {
		t.Fatalf("STORAGE_LIVE_GITHUB_LFS = %q, want owner/repo:path", spec)
	}
	var ts oauth2.TokenSource
	if tok := os.Getenv("STORAGE_LIVE_GITHUB_TOKEN"); tok != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tok})
	}
	ctx := context.Background()
	b, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := github.New(b)
	src := "github://" + rp
	v, err := d.Resolve(ctx, src, "", ts)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []mounts.Mode{mounts.ModeSnapshot, mounts.ModeLive} {
		m, err := d.Prepare(ctx, mounts.Mount{Name: "lfs", Kind: mounts.KindGitHub, Source: src, Version: v, Mode: mode}, ts)
		if err != nil {
			t.Fatalf("%s Prepare: %v", mode, err)
		}
		fsys, err := d.Open(ctx, m, ts)
		if err != nil {
			t.Fatal(err)
		}
		got, err := fs.ReadFile(fsys, p)
		if err != nil {
			t.Fatalf("%s ReadFile(%s): %v", mode, p, err)
		}
		if _, isPointer := mounts.ParseLFSPointer(got); isPointer {
			t.Errorf("%s read %s as its pointer", mode, p)
		}
		t.Logf("%s read %s: %d bytes", mode, p, len(got))
	}
}
