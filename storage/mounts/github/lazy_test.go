package github_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/github"
)

// boardCap holds board/ (31 bytes) but not the whole repo (76 bytes).
var boardCap = mounts.Limits{MaxBytes: 40}

func (e *env) tarballs() int {
	n := 0
	for _, r := range e.gh.Requests() {
		if strings.Contains(r.Path, "/tarball/") {
			n++
		}
	}
	return n
}

func TestLazySnapshotMountsARepoOverTheCap(t *testing.T) {
	eager := newEnv(t, false, github.WithLimits(boardCap))
	if _, err := eager.drv.Prepare(eager.ctx, mounts.Mount{Name: "x", Kind: mounts.KindGitHub, Source: eager.repo.Source(), Version: eager.repo.Head("main"), Mode: mounts.ModeSnapshot}, nil); !errors.Is(err, mounts.ErrTooLarge) {
		t.Fatalf("eager Prepare over the cap = %v, want ErrTooLarge", err)
	}

	e := newEnv(t, false, github.WithLimits(boardCap), github.WithLazySnapshots(), github.WithRawURL(""))
	m := e.create(t, mounts.Mount{Name: "jetson", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}, nil)
	if m.Manifest != "" {
		t.Errorf("lazy Prepare set Manifest %q, want none", m.Manifest)
	}
	if n := e.tarballs(); n != 0 {
		t.Errorf("lazy Prepare asked for %d tarballs, want 0", n)
	}
	seen := len(e.gh.Requests())
	if got := e.downloads(&seen); len(got) != 0 {
		t.Errorf("lazy Prepare downloaded %v, want nothing", got)
	}
	pinned := e.repo.Head("main")
	e.repo.Commit("main", map[string]string{"README.md": "changed after the mount"}, nil)

	fsys, err := e.drv.Open(e.ctx, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ents, err := fs.ReadDir(fsys, "board"); err != nil || len(ents) != 2 {
		t.Errorf("ReadDir(board) = %v, %v", ents, err)
	}
	if got := e.downloads(&seen); len(got) != 0 {
		t.Errorf("listing folders downloaded %v, want nothing", got)
	}
	if b, err := fs.ReadFile(fsys, "README.md"); err != nil || string(b) != boardFiles["README.md"] {
		t.Errorf("README = %q, %v; want the bytes at %s", b, err, pinned[:7])
	}
	if got := e.downloads(&seen); len(got) != 1 {
		t.Errorf("reading README downloaded %v, want one file", got)
	}
	if err := fstest.TestFS(fsys, "README.md", "board/main.kicad_sch", "board/power.kicad_sch", "docs/notes/assembly.md"); err != nil {
		t.Fatal(err)
	}
}

func TestLazyHostPathFetchesOnlyTheFolderAsked(t *testing.T) {
	e := newEnv(t, false, github.WithLimits(boardCap), github.WithLazySnapshots(), github.WithRawURL(""))
	m := e.create(t, mounts.Mount{Name: "jetson", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}, nil)
	seen := len(e.gh.Requests())

	p, err := e.drv.HostPath(m, "board")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, e.views) {
		t.Errorf("host path %q is outside the views folder %q", p, e.views)
	}
	for _, f := range []string{"main.kicad_sch", "power.kicad_sch"} {
		if b, err := os.ReadFile(filepath.Join(p, f)); err != nil || string(b) != boardFiles["board/"+f] {
			t.Errorf("board/%s on disk = %q, %v", f, b, err)
		}
	}
	if got := e.downloads(&seen); len(got) != 2 {
		t.Errorf("HostPath(board) downloaded %v, want the two board files", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p), "docs")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("docs/ is on disk after HostPath(board): %v", err)
	}

	if _, err := e.drv.HostPath(m, "board/main.kicad_sch"); err != nil {
		t.Fatal(err)
	}
	if got := e.downloads(&seen); len(got) != 0 {
		t.Errorf("a second HostPath downloaded %v, want nothing", got)
	}

	if _, err := e.drv.HostPath(m, ""); !errors.Is(err, mounts.ErrTooLarge) {
		t.Errorf("HostPath of the whole repo over the cap = %v, want ErrTooLarge", err)
	}
	if _, err := e.drv.HostPath(m, "../../etc/passwd"); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("HostPath climbing out = %v, want ErrInvalidPath", err)
	}
	if p, err := e.drv.HostPath(m, "nothing/here"); err != nil {
		t.Errorf("HostPath of a missing path = %v, want its would-be path", err)
	} else if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing path exists on disk: %v", err)
	}
}

// TestTheRegistryFetchesALazyPrivateRepoThroughItsConnection checks that the registry hands its
// connection's token to HostPathWith. Without it the fetch would carry no token and 404.
func TestTheRegistryFetchesALazyPrivateRepoThroughItsConnection(t *testing.T) {
	e := newEnv(t, true)
	e.gh.AddToken("ghs_installation")
	e.repo.Grant("ghs_installation")
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	drv := github.New(e.blobs, github.WithBaseURL(e.gh.URL), github.WithViews(e.views), github.WithLazySnapshots())
	reg := mounts.NewRegistry(store, []mounts.Driver{drv}, mounts.WithCredentials(func(_ context.Context, _, conn string) (mounts.Credentials, error) {
		return static("ghs_installation"), nil
	}))
	if _, err := reg.Create(e.ctx, mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot, Connection: "acme-app"}); err != nil {
		t.Fatal(err)
	}
	p, err := reg.HostPath(e.ctx, mounts.URI{Mount: "board", Path: "board/power.kicad_sch"})
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != boardFiles["board/power.kicad_sch"] {
		t.Errorf("host file = %q, %v", b, err)
	}
	if b, err := fs.ReadFile(reg.FS(e.ctx), "board/docs/notes/assembly.md"); err != nil || string(b) != boardFiles["docs/notes/assembly.md"] {
		t.Errorf("read through the registry = %q, %v", b, err)
	}
	for _, r := range e.gh.Requests() {
		if r.Token != "ghs_installation" {
			t.Errorf("%s %s carried token %q, want the connection's", r.Method, r.Path, r.Token)
		}
	}
}

func TestLazySnapshotServesAnLFSObject(t *testing.T) {
	e, _, commit := lfsEnv(t)
	drv := github.New(e.blobs, github.WithBaseURL(e.gh.URL+"/api/v3"), github.WithViews(e.views), github.WithLazySnapshots())
	m := mounts.Mount{Name: "big", Kind: mounts.KindGitHub, Source: e.repo.Source(), Version: commit, Mode: mounts.ModeSnapshot}
	m, err := drv.Prepare(e.ctx, m, static("tok"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := drv.HostPathWith(e.ctx, m, static("tok"), "designs/big.kicad_pcb")
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != design {
		t.Errorf("LFS file on disk = %q, %v; want the object", b, err)
	}
}

// TestLazySnapshotReadsAfterASweep sweeps every blob with only the registry's referrer, which
// names no file of a lazy snapshot. The view's link keeps the file, so it reads with no download.
// With the view gone too, the file is fetched again rather than lost.
func TestLazySnapshotReadsAfterASweep(t *testing.T) {
	e := newEnv(t, false, github.WithLazySnapshots(), github.WithRawURL(""))
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(store, []mounts.Driver{e.drv})
	if _, err := reg.Create(e.ctx, mounts.Mount{Name: "jetson", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}); err != nil {
		t.Fatal(err)
	}
	read := func() {
		t.Helper()
		if b, err := fs.ReadFile(reg.FS(e.ctx), "jetson/board/main.kicad_sch"); err != nil || string(b) != boardFiles["board/main.kicad_sch"] {
			t.Errorf("read = %q, %v", b, err)
		}
	}
	read()
	orphan, _ := e.blobs.Put(e.ctx, strings.NewReader("nobody names me"))
	old := time.Now().Add(-time.Hour)
	filepath.WalkDir(filepath.Dir(filepath.Dir(mustPath(t, e.blobs, orphan))), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			os.Chtimes(p, old, old)
		}
		return nil
	})
	if n, err := e.blobs.Sweep(e.ctx, time.Minute, reg.BlobRefs(e.blobs)); err != nil || n == 0 {
		t.Fatalf("Sweep = %d, %v; want the fetched blob removed", n, err)
	}
	seen := len(e.gh.Requests())
	read()
	if got := e.downloads(&seen); len(got) != 0 {
		t.Errorf("after the sweep the read downloaded %v, want nothing, since the view keeps the file", got)
	}
	if err := os.RemoveAll(filepath.Join(e.views, "jetson")); err != nil {
		t.Fatal(err)
	}
	read()
	if got := e.downloads(&seen); !slices.Equal(got, []string{e.blobID(t, "board/main.kicad_sch")}) {
		t.Errorf("with the view gone too, the read downloaded %v, want the file again", got)
	}
}

func (e *env) blobID(t *testing.T, p string) string {
	t.Helper()
	files, err := e.drv.ListAt(e.ctx, mounts.Mount{Source: e.repo.Source()}, nil, e.repo.Head("main"), p)
	if err != nil || len(files) != 1 {
		t.Fatalf("ListAt(%s) = %v, %v", p, files, err)
	}
	return files[0].ID
}

// TestALazySnapshotServesWhatItFetchedAfterARestart restarts the driver with GitHub unreachable.
// The kept listing lists the mount, and the view serves the files read before the restart.
func TestALazySnapshotServesWhatItFetchedAfterARestart(t *testing.T) {
	e := newEnv(t, false, github.WithLazySnapshots(), github.WithRawURL(""))
	m := e.create(t, mounts.Mount{Name: "jetson", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}, nil)
	if _, err := e.drv.HostPath(m, "board"); err != nil {
		t.Fatal(err)
	}
	again := github.New(e.blobs, github.WithBaseURL("http://127.0.0.1:1"), github.WithViews(e.views), github.WithLazySnapshots())
	fsys, err := again.Open(e.ctx, m, nil)
	if err != nil {
		t.Fatalf("Open after a restart with GitHub down = %v", err)
	}
	if ents, err := fs.ReadDir(fsys, "."); err != nil || len(ents) != 3 {
		t.Errorf("the root after a restart lists %v, %v; want README.md, board and docs", ents, err)
	}
	if b, err := fs.ReadFile(fsys, "board/power.kicad_sch"); err != nil || string(b) != boardFiles["board/power.kicad_sch"] {
		t.Errorf("a file read before the restart = %q, %v", b, err)
	}
	if _, err := fs.ReadFile(fsys, "README.md"); !errors.Is(err, mounts.ErrUnavailable) {
		t.Errorf("a file never read, with GitHub down = %v; want ErrUnavailable", err)
	}
}

// TestLazyPastLimitsSnapshotsWhatFitsWhole checks both sides of the cap. A repo under it gets an
// eager snapshot, with a manifest. A repo past it is made lazy with no tarball and no download.
func TestLazyPastLimitsSnapshotsWhatFitsWhole(t *testing.T) {
	e := newEnv(t, false, github.WithLazyPastLimits(), github.WithRawURL(""))
	m := e.create(t, mounts.Mount{Name: "fits", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}, nil)
	if m.Manifest == "" || e.tarballs() != 1 {
		t.Errorf("a repo under the cap: Manifest %q after %d tarballs; want an eager snapshot", m.Manifest, e.tarballs())
	}

	e = newEnv(t, false, github.WithLimits(boardCap), github.WithLazyPastLimits(), github.WithRawURL(""))
	m = e.create(t, mounts.Mount{Name: "past", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot}, nil)
	seen := 0
	if m.Manifest != "" || e.tarballs() != 0 || len(e.downloads(&seen)) != 0 {
		t.Errorf("a repo past the cap: Manifest %q, %d tarballs; want a lazy mount with nothing downloaded", m.Manifest, e.tarballs())
	}
	p, err := e.drv.HostPath(m, "board")
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(p, "main.kicad_sch")); err != nil || string(b) != boardFiles["board/main.kicad_sch"] {
		t.Errorf("board/main.kicad_sch on disk = %q, %v", b, err)
	}
}

// TestARegistryWithNoCredentialsServesAnEagerPrivateSnapshot is a read-only reader, such as agni's
// agnids, over the records a server made. It has no credentials, and an eager snapshot needs none,
// since its view comes from the blobs.
func TestARegistryWithNoCredentialsServesAnEagerPrivateSnapshot(t *testing.T) {
	e := newEnv(t, true)
	e.gh.AddToken("ghs_installation")
	e.repo.Grant("ghs_installation")
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	drv := github.New(e.blobs, github.WithBaseURL(e.gh.URL), github.WithViews(e.views), github.WithLazyPastLimits())
	maker := mounts.NewRegistry(store, []mounts.Driver{drv}, mounts.WithCredentials(func(_ context.Context, _, conn string) (mounts.Credentials, error) {
		return static("ghs_installation"), nil
	}))
	if _, err := maker.Create(e.ctx, mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot, Connection: "acme-app"}); err != nil {
		t.Fatal(err)
	}
	reader := mounts.NewRegistry(store, []mounts.Driver{drv})
	p, err := reader.HostPath(e.ctx, mounts.URI{Mount: "board", Path: "board/main.kicad_sch"})
	if err != nil {
		t.Fatalf("HostPath with no credentials on an eager snapshot = %v; want its view", err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != boardFiles["board/main.kicad_sch"] {
		t.Errorf("host file = %q, %v", b, err)
	}
}
