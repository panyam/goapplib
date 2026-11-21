package github_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/github"
	"golang.org/x/oauth2"
)

// downloads lists the files the fake's raw host or git blobs endpoint served since the last call.
func (e *env) downloads(seen *int) []string {
	var out []string
	reqs := e.gh.Requests()
	for _, r := range reqs[*seen:] {
		switch {
		case strings.HasPrefix(r.Path, "/raw/"):
			parts := strings.SplitN(r.Path, "/", 6)
			out = append(out, parts[5])
		case strings.Contains(r.Path, "/git/blobs/"):
			out = append(out, r.Path[strings.LastIndex(r.Path, "/")+1:])
		}
	}
	*seen = len(reqs)
	slices.Sort(out)
	return out
}

func paths(fs []mounts.File) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Path)
	}
	return out
}

func TestReadingAtAVersionDownloadsOnlyWhatChanged(t *testing.T) {
	e := newEnv(t, false)
	drv := github.New(e.blobs, github.WithBaseURL(e.gh.URL), github.WithRawURL(e.gh.RawURL()))
	m := mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive}
	v1 := e.repo.Head("main")
	e.repo.Symlink("main", "board/escape", "../../etc/passwd")
	linked := e.repo.Head("main")

	files, err := drv.ListAt(e.ctx, m, nil, linked, "board")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := paths(files), []string{"board/main.kicad_sch", "board/power.kicad_sch"}; !slices.Equal(got, want) {
		t.Errorf("ListAt(board) = %v, want %v, sorted, with the link left out", got, want)
	}
	if all, _ := drv.ListAt(e.ctx, m, nil, v1, ""); len(all) != len(boardFiles) {
		t.Errorf("ListAt of the root lists %d files, want %d", len(all), len(boardFiles))
	}
	if one, _ := drv.ListAt(e.ctx, m, nil, v1, "README.md"); len(one) != 1 || one[0].ID == "" {
		t.Errorf("ListAt of one file = %v, want the file with its git blob id", one)
	}

	seen := len(e.gh.Requests())
	for _, f := range files {
		h, fetched, err := drv.FetchAt(e.ctx, m, nil, linked, f)
		if err != nil || !fetched || h == "" {
			t.Fatalf("FetchAt(%s) = %q, %v, %v; want a download", f.Path, h, fetched, err)
		}
	}
	if got := e.downloads(&seen); len(got) != 2 {
		t.Errorf("the first read downloaded %v, want both sheets", got)
	}

	v2 := e.repo.Commit("main", map[string]string{"board/power.kicad_sch": "(kicad_sch power v2)"}, nil)
	files2, err := drv.ListAt(e.ctx, m, nil, v2, "board")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files2 {
		if _, _, err := drv.FetchAt(e.ctx, m, nil, v2, f); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.downloads(&seen); !slices.Equal(got, []string{"board/power.kicad_sch"}) {
		t.Errorf("the second commit downloaded %v, want only the sheet whose git blob id changed", got)
	}

	e.gh.CorruptRaw(true)
	readme, _ := drv.ListAt(e.ctx, m, nil, v1, "README.md")
	if _, _, err := drv.FetchAt(e.ctx, m, nil, v1, readme[0]); !errors.Is(err, mounts.ErrUnavailable) {
		t.Errorf("a download that doesn't match its git blob id = %v, want ErrUnavailable", err)
	}
}

func TestWithoutARawHostFetchAtUsesTheGitBlobsAPI(t *testing.T) {
	e := newEnv(t, true)
	e.gh.AddToken("ghs_installation")
	ts := static("ghs_installation")
	m := mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive}
	v := e.repo.Head("main")
	files, err := e.drv.ListAt(e.ctx, m, ts, v, "README.md")
	if err != nil || len(files) != 1 {
		t.Fatalf("ListAt = %v, %v", files, err)
	}
	seen := len(e.gh.Requests())
	if _, fetched, err := e.drv.FetchAt(e.ctx, m, ts, v, files[0]); err != nil || !fetched {
		t.Fatalf("FetchAt = %v, %v", fetched, err)
	}
	reqs := e.gh.Requests()[seen:]
	if len(reqs) != 1 || !strings.Contains(reqs[0].Path, "/git/blobs/"+files[0].ID) || reqs[0].Token != "ghs_installation" {
		t.Errorf("FetchAt sent %v; want one git blobs request carrying the connection's token", reqs)
	}
	if _, err := e.drv.ListAt(e.ctx, m, nil, v, ""); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("ListAt on a private repo without a token = %v, want ErrNotFound", err)
	}
}

func TestMovesFollowsBranchesAndPinsCommits(t *testing.T) {
	e := newEnv(t, false)
	for _, c := range []struct {
		ref  string
		want bool
	}{{"", true}, {"main", true}, {e.repo.Head("main"), false}, {"v1.0", false}} {
		got, err := e.drv.Moves(e.ctx, e.repo.Source(), c.ref, nil)
		if err != nil || got != c.want {
			t.Errorf("Moves(%q) = %v, %v; want %v", c.ref, got, err, c.want)
		}
	}
}

func TestListAtRefusesWhatItCannotList(t *testing.T) {
	e := newEnv(t, false, github.WithLimits(mounts.Limits{MaxFiles: 2}))
	m := mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive}
	v := e.repo.Head("main")
	if _, err := e.drv.ListAt(e.ctx, m, nil, v, ""); !errors.Is(err, mounts.ErrTooLarge) {
		t.Errorf("ListAt over the file cap = %v, want ErrTooLarge", err)
	}
	if got, err := e.drv.ListAt(e.ctx, m, nil, v, "board"); err != nil || len(got) != 2 {
		t.Errorf("ListAt within the cap = %v, %v; want the two sheets", got, err)
	}
	if _, err := e.drv.ListAt(e.ctx, m, nil, "main", ""); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("ListAt at a branch name = %v, want ErrInvalidArgument, since a version is a commit", err)
	}
	if _, _, err := e.drv.FetchAt(e.ctx, m, nil, v, mounts.File{Path: "README.md"}); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("FetchAt of a file with no git blob id = %v, want ErrInvalidArgument", err)
	}
}

func TestTheRegistryReadsAPrivateRepoAtAVersionThroughItsConnection(t *testing.T) {
	e := newEnv(t, true)
	e.gh.AddToken("ghs_installation")
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	drv := github.New(e.blobs, github.WithBaseURL(e.gh.URL), github.WithRawURL(e.gh.RawURL()))
	reg := mounts.NewRegistry(store, []mounts.Driver{drv}, mounts.WithCredentials(func(_ context.Context, _, conn string) (oauth2.TokenSource, error) {
		return static("ghs_installation"), nil
	}))
	if _, err := reg.Create(e.ctx, mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive, Connection: "acme-app"}); err != nil {
		t.Fatal(err)
	}
	seen := len(e.gh.Requests())
	v, moves, err := reg.ResolveAt(e.ctx, "board", "main")
	if err != nil || v != e.repo.Head("main") || !moves {
		t.Fatalf("ResolveAt(main) = %q, %v, %v; want the head, moving", v, moves, err)
	}
	files, err := reg.ListAt(e.ctx, "board", v, "board")
	if err != nil || len(files) != 2 {
		t.Fatalf("ListAt = %v, %v", files, err)
	}
	if _, fetched, err := reg.FetchAt(e.ctx, "board", v, files[0]); err != nil || !fetched {
		t.Fatalf("FetchAt = %v, %v", fetched, err)
	}
	for _, r := range e.gh.Requests()[seen:] {
		if r.Token != "ghs_installation" {
			t.Errorf("%s %s carried token %q, want the connection's", r.Method, r.Path, r.Token)
		}
	}
}
