package github_test

import (
	"archive/tar"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/github"
	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
	"golang.org/x/oauth2"
)

var boardFiles = map[string]string{
	"README.md":              "the jetson baseboard",
	"board/main.kicad_sch":   "(kicad_sch v1)",
	"board/power.kicad_sch":  "(kicad_sch power)",
	"docs/notes/assembly.md": "solder the big parts last",
}

type env struct {
	ctx   context.Context
	gh    *githubtest.Server
	repo  *githubtest.Repo
	blobs *blobs.Dir
	drv   *github.Driver
	views string
}

func newEnv(t *testing.T, private bool, opts ...github.Option) *env {
	t.Helper()
	gh := githubtest.New(t)
	repo := gh.AddRepo("acme", "board", private, boardFiles)
	b, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	views := t.TempDir()
	drv := github.New(b, append([]github.Option{github.WithBaseURL(gh.URL), github.WithViews(views)}, opts...)...)
	return &env{ctx: context.Background(), gh: gh, repo: repo, blobs: b, drv: drv, views: views}
}

func (e *env) create(t *testing.T, m mounts.Mount, ts oauth2.TokenSource) mounts.Mount {
	t.Helper()
	v, err := e.drv.Resolve(e.ctx, m.Source, m.Ref, ts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	m.Version = v
	if m, err = e.drv.Prepare(e.ctx, m, ts); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return m
}

func static(tok string) oauth2.TokenSource {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tok})
}

func TestResolve(t *testing.T) {
	e := newEnv(t, false)
	head := e.repo.Head("main")
	for ref, want := range map[string]string{"": head, "main": head, head: head} {
		if got, err := e.drv.Resolve(e.ctx, e.repo.Source(), ref, nil); err != nil || got != want {
			t.Errorf("Resolve(ref %q) = %q, %v; want %q", ref, got, err, want)
		}
	}
	if _, err := e.drv.Resolve(e.ctx, e.repo.Source(), "no-such-branch", nil); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Resolve of a missing ref = %v, want ErrNotFound", err)
	}
	if _, err := e.drv.Resolve(e.ctx, "github://acme/nope", "", nil); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Resolve of a missing repo = %v, want ErrNotFound", err)
	}
	for _, src := range []string{"https://github.com/acme/board", "github://acme", "github://acme/board/extra", "github://ac me/board", "file:///etc"} {
		if _, err := e.drv.Resolve(e.ctx, src, "", nil); !errors.Is(err, mounts.ErrInvalidArgument) {
			t.Errorf("Resolve(%q) = %v, want ErrInvalidArgument", src, err)
		}
	}
	if got, err := e.drv.Resolve(e.ctx, e.repo.Source()+".git", "", nil); err != nil || got != head {
		t.Errorf("Resolve with a .git suffix = %q, %v", got, err)
	}
}

func TestSnapshot(t *testing.T) {
	e := newEnv(t, false)
	m := e.create(t, mounts.Mount{Name: "jetson", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}, nil)
	if _, err := blobs.ParseHash(m.Manifest); err != nil {
		t.Fatalf("Prepare left Manifest = %q: %v", m.Manifest, err)
	}
	pinned := m.Version
	e.repo.Commit("main", map[string]string{"README.md": "changed after the snapshot"}, nil)

	fsys, err := e.drv.Open(e.ctx, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(fsys, "README.md", "board/main.kicad_sch", "board/power.kicad_sch", "docs/notes/assembly.md"); err != nil {
		t.Fatal(err)
	}
	if b, _ := fs.ReadFile(fsys, "README.md"); string(b) != boardFiles["README.md"] {
		t.Errorf("snapshot README = %q, want the bytes at %s", b, pinned[:7])
	}
	if _, err := e.drv.Stat(e.ctx, m, nil, "README.md"); !errors.Is(err, mounts.ErrReadOnly) {
		t.Errorf("Stat on a snapshot = %v, want ErrReadOnly", err)
	}

	p, err := e.drv.HostPath(m, "board/main.kicad_sch")
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != boardFiles["board/main.kicad_sch"] {
		t.Errorf("host file = %q, %v", b, err)
	}
	if !strings.HasPrefix(p, e.views) {
		t.Errorf("host path %q is outside the views folder %q", p, e.views)
	}
	if _, err := e.drv.HostPath(m, "../../etc/passwd"); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("HostPath climbing out = %v, want ErrInvalidPath", err)
	}

	// After a restart, a new driver over the same blobs and views serves the same files, and
	// rebuilds a view that was lost, without asking GitHub.
	os.RemoveAll(e.views)
	before := len(e.gh.Requests())
	again := github.New(e.blobs, github.WithBaseURL(e.gh.URL), github.WithViews(e.views))
	fsys, _ = again.Open(e.ctx, m, nil)
	if b, err := fs.ReadFile(fsys, "docs/notes/assembly.md"); err != nil || string(b) != boardFiles["docs/notes/assembly.md"] {
		t.Errorf("after a restart, read = %q, %v", b, err)
	}
	if p, err := again.HostPath(m, "README.md"); err != nil {
		t.Errorf("after a restart, HostPath = %v", err)
	} else if b, _ := os.ReadFile(p); string(b) != boardFiles["README.md"] {
		t.Errorf("rebuilt view README = %q", b)
	}
	if n := len(e.gh.Requests()) - before; n != 0 {
		t.Errorf("serving a snapshot after a restart made %d GitHub requests, want 0", n)
	}
}

func TestSnapshotSkipsLinksAndRefusesClimbing(t *testing.T) {
	e := newEnv(t, false)
	e.repo.Symlink("main", "board/escape", "/etc/passwd")
	m := e.create(t, mounts.Mount{Name: "jetson", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}, nil)
	fsys, _ := e.drv.Open(e.ctx, m, nil)
	if _, err := fs.Stat(fsys, "board/escape"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a link in the tarball appears in the snapshot: %v", err)
	}

	for _, bad := range []githubtest.TarEntry{
		{Name: "../outside.txt", Type: tar.TypeReg, Body: "x"},
		{Name: "a/../../outside.txt", Type: tar.TypeReg, Body: "x"},
		{Name: "a//b.txt", Type: tar.TypeReg, Body: "x"},
	} {
		sha := e.repo.Commit("main", map[string]string{"bump": bad.Name}, nil)
		e.repo.AddTarEntries(sha, bad)
		_, err := e.drv.Prepare(e.ctx, mounts.Mount{Name: "bad", Kind: mounts.KindGitHub, Source: e.repo.Source(), Version: sha, Mode: mounts.ModeSnapshot}, nil)
		if !errors.Is(err, mounts.ErrInvalidPath) {
			t.Errorf("tar entry %q: Prepare = %v, want ErrInvalidPath", bad.Name, err)
		}
	}
}

func TestSnapshotLimits(t *testing.T) {
	e := newEnv(t, false, github.WithLimits(mounts.Limits{MaxFiles: 3}))
	_, err := e.drv.Prepare(e.ctx, mounts.Mount{Name: "x", Kind: mounts.KindGitHub, Source: e.repo.Source(), Version: e.repo.Head("main"), Mode: mounts.ModeSnapshot}, nil)
	if !errors.Is(err, mounts.ErrTooLarge) {
		t.Errorf("4 files over a 3-file cap = %v, want ErrTooLarge", err)
	}
	e = newEnv(t, false, github.WithLimits(mounts.Limits{MaxBytes: 20}))
	_, err = e.drv.Prepare(e.ctx, mounts.Mount{Name: "x", Kind: mounts.KindGitHub, Source: e.repo.Source(), Version: e.repo.Head("main"), Mode: mounts.ModeSnapshot}, nil)
	if !errors.Is(err, mounts.ErrTooLarge) {
		t.Errorf("over a 20-byte cap = %v, want ErrTooLarge", err)
	}
}

func TestLive(t *testing.T) {
	e := newEnv(t, false)
	m := e.create(t, mounts.Mount{Name: "notes", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive}, nil)
	if m.Ref != "main" {
		t.Errorf("Prepare left a live mount's Ref = %q, want the default branch", m.Ref)
	}
	fsys, err := e.drv.Open(e.ctx, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(fsys, "README.md", "board/main.kicad_sch", "docs/notes/assembly.md"); err != nil {
		t.Fatal(err)
	}

	e.repo.Commit("main", map[string]string{"README.md": "edited on GitHub"}, nil)
	if b, _ := fs.ReadFile(fsys, "README.md"); string(b) != "edited on GitHub" {
		t.Errorf("a live mount read %q, want the branch's latest", b)
	}

	v, err := e.drv.Stat(e.ctx, m, nil, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.drv.Put(e.ctx, m, nil, mounts.PutRequest{Path: "README.md", Data: []byte("saved by the app"), IfMatch: v, Message: "save"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mount != e.repo.Head("main") || res.File == "" || res.File == v {
		t.Errorf("Put = %+v; want the new commit %s and a new file version", res, e.repo.Head("main"))
	}
	if got, _ := e.repo.File("main", "README.md"); got != "saved by the app" {
		t.Errorf("README on GitHub = %q", got)
	}
	if _, err := e.drv.Put(e.ctx, m, nil, mounts.PutRequest{Path: "README.md", Data: []byte("stale"), IfMatch: v}); !errors.Is(err, mounts.ErrConflict) {
		t.Errorf("Put with a stale IfMatch = %v, want ErrConflict", err)
	}
	if _, err := e.drv.Put(e.ctx, m, nil, mounts.PutRequest{Path: "README.md", Data: []byte("x")}); !errors.Is(err, mounts.ErrConflict) {
		t.Errorf("Put with an empty IfMatch over an existing file = %v, want ErrConflict", err)
	}
	if _, err := e.drv.Put(e.ctx, m, nil, mounts.PutRequest{Path: "new/note.md", Data: []byte("fresh")}); err != nil {
		t.Errorf("create-only Put of a new file = %v", err)
	}
	if _, err := e.drv.Put(e.ctx, m, nil, mounts.PutRequest{Path: "README.md", Data: []byte("forced"), IfMatch: mounts.AnyVersion}); err != nil {
		t.Errorf("Put with AnyVersion = %v", err)
	}
	if got, _ := e.repo.File("main", "README.md"); got != "forced" {
		t.Errorf("README after AnyVersion = %q", got)
	}

	cur, _ := e.drv.Stat(e.ctx, m, nil, "new/note.md")
	if _, err := e.drv.Delete(e.ctx, m, nil, mounts.DeleteRequest{Path: "new/note.md", IfMatch: "0000000000000000000000000000000000000000"}); !errors.Is(err, mounts.ErrConflict) {
		t.Errorf("Delete with a stale IfMatch = %v, want ErrConflict", err)
	}
	if _, err := e.drv.Delete(e.ctx, m, nil, mounts.DeleteRequest{Path: "new/note.md", IfMatch: cur}); err != nil {
		t.Errorf("Delete = %v", err)
	}
	if _, err := e.drv.Stat(e.ctx, m, nil, "new/note.md"); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Stat after Delete = %v, want ErrNotFound", err)
	}
	if _, err := e.drv.HostPath(m, "README.md"); !errors.Is(err, mounts.ErrNotLocal) {
		t.Errorf("HostPath of a live mount = %v, want ErrNotLocal", err)
	}
	for _, p := range []string{"../x", "/abs"} {
		if _, err := e.drv.Put(e.ctx, m, nil, mounts.PutRequest{Path: p, Data: []byte("x"), IfMatch: mounts.AnyVersion}); !errors.Is(err, mounts.ErrInvalidPath) {
			t.Errorf("Put(%q) = %v, want ErrInvalidPath", p, err)
		}
	}
}

func TestPrivateRepoNeedsTheToken(t *testing.T) {
	e := newEnv(t, true)
	if _, err := e.drv.Resolve(e.ctx, e.repo.Source(), "", nil); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("a private repo without a token = %v, want ErrNotFound", err)
	}
	e.gh.AddToken("ghs_installation")
	ts := static("ghs_installation")
	before := len(e.gh.Requests())
	snap := e.create(t, mounts.Mount{Name: "snap", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}, ts)
	live := e.create(t, mounts.Mount{Name: "live", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive}, ts)
	fsys, _ := e.drv.Open(e.ctx, live, ts)
	fs.ReadFile(fsys, "README.md")
	v, _ := e.drv.Stat(e.ctx, live, ts, "README.md")
	e.drv.Put(e.ctx, live, ts, mounts.PutRequest{Path: "README.md", Data: []byte("x"), IfMatch: v})
	_ = snap
	reqs := e.gh.Requests()[before:]
	if len(reqs) < 6 {
		t.Fatalf("only %d requests reached the fake", len(reqs))
	}
	for _, r := range reqs {
		if r.Token != "ghs_installation" {
			t.Errorf("%s %s carried token %q, want the connection's", r.Method, r.Path, r.Token)
		}
	}
	if _, err := e.drv.Resolve(e.ctx, e.repo.Source(), "", static("ghs_wrong")); !errors.Is(err, mounts.ErrUnavailable) {
		t.Errorf("a rejected token = %v, want ErrUnavailable", err)
	}
	boom := errors.New("connect again")
	if _, err := e.drv.Resolve(e.ctx, e.repo.Source(), "", failing{boom}); !errors.Is(err, boom) {
		t.Errorf("a token source error = %v, want it passed through", err)
	}
}

type failing struct{ err error }

func (f failing) Token() (*oauth2.Token, error) { return nil, f.err }

func TestErrorMapping(t *testing.T) {
	e := newEnv(t, false)
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusBadGateway} {
		e.gh.FailNext(status)
		if _, err := e.drv.Resolve(e.ctx, e.repo.Source(), "", nil); !errors.Is(err, mounts.ErrUnavailable) {
			t.Errorf("HTTP %d = %v, want ErrUnavailable", status, err)
		}
	}
}

func TestDriverIsAWriterAndLocalizer(t *testing.T) {
	var d mounts.Driver = github.New(nil)
	if _, ok := d.(mounts.Writer); !ok {
		t.Error("the GitHub driver isn't a mounts.Writer")
	}
	if _, ok := d.(mounts.Localizer); !ok {
		t.Error("the GitHub driver isn't a mounts.Localizer")
	}
	if _, ok := d.(mounts.Preparer); !ok {
		t.Error("the GitHub driver isn't a mounts.Preparer")
	}
	if d.Kind() != mounts.KindGitHub || len(d.Modes()) != 2 {
		t.Errorf("Kind %q, Modes %v", d.Kind(), d.Modes())
	}
	if _, err := github.New(nil).Prepare(context.Background(), mounts.Mount{Kind: mounts.KindGitHub, Source: "github://a/b", Mode: mounts.ModeSnapshot}, nil); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("a snapshot with no blob store = %v, want ErrInvalidArgument", err)
	}
}

// TestRegistryBlobRefsKeepSnapshots runs a snapshot through the registry, then sweeps the blob
// store with only the registry's referrer, and checks the snapshot still reads after a restart.
func TestRegistryBlobRefsKeepSnapshots(t *testing.T) {
	e := newEnv(t, false)
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(store, []mounts.Driver{e.drv})
	m, err := reg.Create(e.ctx, mounts.Mount{Name: "jetson", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	orphan, _ := e.blobs.Put(e.ctx, strings.NewReader("nobody names me"))
	for _, h := range []blobs.Hash{orphan, blobs.Hash(m.Manifest)} {
		p, _ := e.blobs.Path(h)
		old := time.Now().Add(-time.Hour)
		os.Chtimes(p, old, old)
	}
	filepath.WalkDir(filepath.Dir(filepath.Dir(mustPath(t, e.blobs, orphan))), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			old := time.Now().Add(-time.Hour)
			os.Chtimes(p, old, old)
		}
		return nil
	})
	n, err := e.blobs.Sweep(e.ctx, time.Minute, reg.BlobRefs(e.blobs))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("Sweep removed %d blobs, want 1 (the orphan)", n)
	}
	os.RemoveAll(e.views)
	fsys := reg.FS(e.ctx)
	if b, err := fs.ReadFile(fsys, "jetson/board/power.kicad_sch"); err != nil || string(b) != boardFiles["board/power.kicad_sch"] {
		t.Errorf("snapshot after the sweep = %q, %v", b, err)
	}
}

func mustPath(t *testing.T, d *blobs.Dir, h blobs.Hash) string {
	t.Helper()
	p, err := d.Path(h)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
