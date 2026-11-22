package github_test

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/github"
)

const design = "(kicad_pcb the real 48 MB design, shortened for the test)"

// lfsEnv is a private repo whose designs/big.kicad_pcb is in LFS, read with a granted token
// through a driver pointed at the fake as a GitHub Enterprise Server host, so the LFS host comes
// from the API base.
func lfsEnv(t *testing.T) (*env, *github.Driver, string) {
	t.Helper()
	e := newEnv(t, true)
	e.gh.AddToken("tok")
	e.repo.Grant("tok")
	commit, _ := e.repo.CommitLFS("main", "designs/big.kicad_pcb", design)
	drv := github.New(e.blobs, github.WithBaseURL(e.gh.URL+"/api/v3"))
	return e, drv, commit
}

func (e *env) lfsBatches() []string {
	var toks []string
	for _, r := range e.gh.Requests() {
		if strings.HasSuffix(r.Path, "/info/lfs/objects/batch") {
			toks = append(toks, r.Token)
		}
	}
	return toks
}

func TestLFSSnapshotServesTheObject(t *testing.T) {
	e, drv, commit := lfsEnv(t)
	m := mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot, Version: commit}
	m, err := drv.Prepare(e.ctx, m, static("tok"))
	if err != nil {
		t.Fatal(err)
	}
	fsys, err := drv.Open(e.ctx, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := fs.ReadFile(fsys, "designs/big.kicad_pcb"); err != nil || string(b) != design {
		t.Fatalf("ReadFile = %q, %v; want the LFS object", b, err)
	}
	if fi, err := fs.Stat(fsys, "designs/big.kicad_pcb"); err != nil || fi.Size() != int64(len(design)) {
		t.Errorf("Stat size = %v, %v; want the object's %d bytes", fi, err, len(design))
	}
	if b, _ := fs.ReadFile(fsys, "README.md"); string(b) != boardFiles["README.md"] {
		t.Errorf("README.md = %q, want the plain file", b)
	}
	if got := e.lfsBatches(); len(got) != 1 || got[0] != "tok" {
		t.Errorf("batch requests carried tokens %q, want one with the mount's token", got)
	}

	// A second snapshot of the same commit finds the object in the blob store.
	m.Name = "board-again"
	if _, err := drv.Prepare(e.ctx, m, static("tok")); err != nil {
		t.Fatal(err)
	}
	if got := e.lfsBatches(); len(got) != 1 {
		t.Errorf("%d batch requests after a second snapshot, want still 1", len(got))
	}
}

func TestLFSFetchAtAndLiveReadServeTheObject(t *testing.T) {
	e, drv, commit := lfsEnv(t)
	m := mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive, Ref: "main"}
	files, err := drv.ListAt(e.ctx, m, static("tok"), commit, "designs")
	if err != nil || len(files) != 1 {
		t.Fatalf("ListAt = %v, %v", files, err)
	}
	h, fetched, err := drv.FetchAt(e.ctx, m, static("tok"), commit, files[0])
	if err != nil || !fetched {
		t.Fatalf("FetchAt = %v, %v", fetched, err)
	}
	rc, err := e.blobs.Open(e.ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != design {
		t.Errorf("FetchAt kept %q, want the LFS object", b)
	}

	fsys, err := drv.Open(e.ctx, m, static("tok"))
	if err != nil {
		t.Fatal(err)
	}
	if b, err := fs.ReadFile(fsys, "designs/big.kicad_pcb"); err != nil || string(b) != design {
		t.Errorf("live ReadFile = %q, %v; want the LFS object", b, err)
	}
}

func TestLFSFailuresNameThePath(t *testing.T) {
	for name, tc := range map[string]struct {
		break_ func(e *env, oid string)
		reason error
		says   string
	}{
		"missing object": {func(e *env, oid string) { e.gh.RemoveLFSObject(oid) }, mounts.ErrNotFound, "Object does not exist"},
		"over quota": {func(e *env, oid string) {
			e.gh.FailLFS(http.StatusForbidden, "This repository is over its data quota")
		}, mounts.ErrPermissionDenied, "over its data quota"},
		"corrupt object": {func(e *env, oid string) { e.gh.CorruptLFS(true) }, mounts.ErrUnavailable, "over its"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, true)
			e.gh.AddToken("tok")
			commit, oid := e.repo.CommitLFS("main", "designs/big.kicad_pcb", design)
			drv := github.New(e.blobs, github.WithBaseURL(e.gh.URL+"/api/v3"))
			tc.break_(e, oid)
			m := mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot, Version: commit}
			_, err := drv.Prepare(e.ctx, m, static("tok"))
			if !errors.Is(err, mounts.ErrLFS) || !errors.Is(err, tc.reason) {
				t.Fatalf("Prepare = %v, want ErrLFS and %v", err, tc.reason)
			}
			if msg := err.Error(); !strings.Contains(msg, "designs/big.kicad_pcb") || !strings.Contains(msg, tc.says) {
				t.Errorf("Prepare error %q should name the path and say %q", msg, tc.says)
			}
		})
	}
}

func TestLFSLiveReadRefusesACorruptObject(t *testing.T) {
	e, drv, _ := lfsEnv(t)
	e.gh.CorruptLFS(true)
	m := mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive, Ref: "main"}
	fsys, err := drv.Open(e.ctx, m, static("tok"))
	if err != nil {
		t.Fatal(err)
	}
	if b, err := fs.ReadFile(fsys, "designs/big.kicad_pcb"); !errors.Is(err, mounts.ErrLFS) {
		t.Errorf("live ReadFile = %q, %v; want ErrLFS", b, err)
	}
}

func TestLFSObjectSizeCountsTowardTheLimits(t *testing.T) {
	e := newEnv(t, false)
	commit, _ := e.repo.CommitLFS("main", "designs/big.kicad_pcb", strings.Repeat("x", 1000))
	drv := github.New(e.blobs, github.WithBaseURL(e.gh.URL+"/api/v3"), github.WithLimits(mounts.Limits{MaxBytes: 600}))
	m := mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeSnapshot, Version: commit}
	if _, err := drv.Prepare(e.ctx, m, nil); !errors.Is(err, mounts.ErrTooLarge) {
		t.Errorf("Prepare = %v; want ErrTooLarge, since the object passes 600 bytes and its pointer doesn't", err)
	}
}
