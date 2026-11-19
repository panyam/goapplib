package mounts_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/folder"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"golang.org/x/oauth2"
)

// fakeGitHub is a snapshot-only driver that records the token each call got.
type fakeGitHub struct{ tokens []string }

func (f *fakeGitHub) Kind() mounts.Kind    { return mounts.KindGitHub }
func (f *fakeGitHub) Modes() []mounts.Mode { return []mounts.Mode{mounts.ModeSnapshot} }

func (f *fakeGitHub) note(ts oauth2.TokenSource) {
	if ts == nil {
		f.tokens = append(f.tokens, "")
		return
	}
	tok, _ := ts.Token()
	f.tokens = append(f.tokens, tok.AccessToken)
}

func (f *fakeGitHub) Resolve(_ context.Context, source, ref string, ts oauth2.TokenSource) (string, error) {
	f.note(ts)
	if source != "github://acme/boards" {
		return "", mounts.ErrNotFound
	}
	return "c0ffee" + ref, nil
}

func (f *fakeGitHub) Prepare(_ context.Context, m mounts.Mount, ts oauth2.TokenSource) (mounts.Mount, error) {
	f.note(ts)
	m.Manifest = "sha256:manifest-of-" + m.Version
	return m, nil
}

func (f *fakeGitHub) Open(_ context.Context, m mounts.Mount, ts oauth2.TokenSource) (fs.FS, error) {
	f.note(ts)
	return fstest.MapFS{"board.kicad_sch": {Data: []byte("at " + m.Version)}}, nil
}

type env struct {
	ctx    context.Context
	dir    string
	notes  string
	gh     *fakeGitHub
	inUse  map[string]bool
	reg    *mounts.Registry
	tokens map[string]string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{ctx: context.Background(), dir: t.TempDir(), notes: t.TempDir(), gh: &fakeGitHub{},
		inUse: map[string]bool{}, tokens: map[string]string{"acme-app": "tok-acme"}}
	if err := os.WriteFile(filepath.Join(e.notes, "hello.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.reg = e.registry(t)
	return e
}

func (e *env) registry(t *testing.T) *mounts.Registry {
	t.Helper()
	store, err := fsstore.New(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	return mounts.NewRegistry(store, []mounts.Driver{folder.New(), e.gh},
		mounts.WithFixed(mounts.Mount{Name: "notes", Kind: mounts.KindFolder, Source: e.notes, Mode: mounts.ModeLive}),
		mounts.WithCredentials(func(_ context.Context, conn string) (oauth2.TokenSource, error) {
			tok, ok := e.tokens[conn]
			if !ok {
				return nil, errors.New("no such connection")
			}
			return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tok}), nil
		}),
		mounts.WithInUse(func(_ context.Context, name string) (bool, error) { return e.inUse[name], nil }),
		mounts.WithLimits(mounts.Limits{MaxBytes: 16}),
		mounts.WithRecheck(time.Millisecond),
	)
}

func names(t *testing.T, r *mounts.Registry) []string {
	t.Helper()
	ms, err := r.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range ms {
		out = append(out, m.Name)
	}
	return out
}

func uri(t *testing.T, s string) mounts.URI {
	t.Helper()
	u, err := mounts.ParseURI(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCreateResolvesThroughTheConnectionAndOpens(t *testing.T) {
	e := newEnv(t)
	m, err := e.reg.Create(e.ctx, mounts.Mount{Name: "boards", Kind: mounts.KindGitHub, Source: "github://acme/boards", Ref: "main", Mode: mounts.ModeSnapshot, Connection: "acme-app"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "c0ffeemain" || m.Created.IsZero() || m.Manifest != "sha256:manifest-of-c0ffeemain" {
		t.Errorf("Create = %+v, want Version c0ffeemain, a Created time, and Prepare's manifest", m)
	}
	if stored, err := e.registry(t).Get(e.ctx, "boards"); err != nil || stored.Manifest != m.Manifest {
		t.Errorf("a fresh registry's Get = %+v, %v; want the prepared record", stored, err)
	}
	f, err := e.reg.Open(e.ctx, uri(t, "mount://boards/board.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "at c0ffeemain" {
		t.Errorf("read %q, want the pinned version's bytes", b)
	}
	for i, tok := range e.gh.tokens {
		if tok != "tok-acme" {
			t.Errorf("driver call %d got token %q, want tok-acme", i, tok)
		}
	}
	if got := names(t, e.reg); len(got) != 2 || got[0] != "boards" || got[1] != "notes" {
		t.Errorf("List = %v, want [boards notes]", got)
	}
}

func TestCreateRefusals(t *testing.T) {
	e := newEnv(t)
	for name, tc := range map[string]struct {
		m    mounts.Mount
		want error
	}{
		"folder from a request": {mounts.Mount{Name: "etc", Kind: mounts.KindFolder, Source: "/etc", Mode: mounts.ModeLive}, mounts.ErrInvalidArgument},
		"unknown kind":          {mounts.Mount{Name: "x", Kind: "s3", Source: "s3://b", Mode: mounts.ModeSnapshot}, mounts.ErrInvalidArgument},
		"unsupported mode":      {mounts.Mount{Name: "x", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeLive}, mounts.ErrInvalidArgument},
		"bad name":              {mounts.Mount{Name: "X", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeSnapshot}, mounts.ErrInvalidArgument},
		"fixed name":            {mounts.Mount{Name: "notes", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeSnapshot}, mounts.ErrExists},
		"missing source":        {mounts.Mount{Name: "x", Kind: mounts.KindGitHub, Source: "github://acme/none", Mode: mounts.ModeSnapshot}, mounts.ErrNotFound},
	} {
		if _, err := e.reg.Create(e.ctx, tc.m); !errors.Is(err, tc.want) {
			t.Errorf("%s: Create = %v, want %v", name, err, tc.want)
		}
	}
	ok := mounts.Mount{Name: "boards", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeSnapshot}
	if _, err := e.reg.Create(e.ctx, ok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.reg.Create(e.ctx, ok); !errors.Is(err, mounts.ErrExists) {
		t.Errorf("second Create = %v, want ErrExists", err)
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	if err := e.reg.Delete(e.ctx, "notes"); !errors.Is(err, mounts.ErrFixed) {
		t.Errorf("Delete(fixed) = %v, want ErrFixed", err)
	}
	if err := e.reg.Delete(e.ctx, "nope"); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Delete(missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.reg.Create(e.ctx, mounts.Mount{Name: "boards", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeSnapshot}); err != nil {
		t.Fatal(err)
	}
	e.inUse["boards"] = true
	if err := e.reg.Delete(e.ctx, "boards"); !errors.Is(err, mounts.ErrInUse) {
		t.Errorf("Delete(in use) = %v, want ErrInUse", err)
	}
	e.inUse["boards"] = false
	if err := e.reg.Delete(e.ctx, "boards"); err != nil {
		t.Errorf("Delete = %v", err)
	}
	if _, err := e.reg.Get(e.ctx, "boards"); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
}

func TestTwoRegistriesOverOneStoreSeeEachOther(t *testing.T) {
	e := newEnv(t)
	other := e.registry(t)
	if got := names(t, other); len(got) != 1 {
		t.Fatalf("other.List = %v, want [notes]", got)
	}
	if _, err := e.reg.Create(e.ctx, mounts.Mount{Name: "boards", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeSnapshot}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := other.Get(e.ctx, "boards"); err != nil {
		t.Errorf("other.Get after the first registry's create = %v", err)
	}
	if err := other.Delete(e.ctx, "boards"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := e.reg.Get(e.ctx, "boards"); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Get after the other registry's delete = %v, want ErrNotFound", err)
	}
}

func TestWritesThroughTheRegistry(t *testing.T) {
	e := newEnv(t)
	res, err := e.reg.Put(e.ctx, "notes", mounts.PutRequest{Path: "today.md", Data: []byte("note")})
	if err != nil {
		t.Fatal(err)
	}
	if v, err := e.reg.Stat(e.ctx, uri(t, "mount://notes/today.md")); err != nil || v != res.File {
		t.Errorf("Stat = %q, %v; want %q", v, err, res.File)
	}
	if b, err := fs.ReadFile(e.reg.FS(e.ctx), "notes/today.md"); err != nil || string(b) != "note" {
		t.Errorf("FS read = %q, %v", b, err)
	}
	if _, err := e.reg.Put(e.ctx, "notes", mounts.PutRequest{Path: "big.md", Data: make([]byte, 17)}); !errors.Is(err, mounts.ErrTooLarge) {
		t.Errorf("Put over MaxBytes = %v, want ErrTooLarge", err)
	}
	if _, err := e.reg.Put(e.ctx, "notes", mounts.PutRequest{Path: "../x", Data: []byte("x")}); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("Put climbing out = %v, want ErrInvalidPath", err)
	}
	if _, err := e.reg.Remove(e.ctx, "notes", mounts.DeleteRequest{Path: "today.md", IfMatch: res.File}); err != nil {
		t.Errorf("Remove = %v", err)
	}

	if _, err := e.reg.Create(e.ctx, mounts.Mount{Name: "boards", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeSnapshot}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.reg.Put(e.ctx, "boards", mounts.PutRequest{Path: "x", Data: []byte("x")}); !errors.Is(err, mounts.ErrReadOnly) {
		t.Errorf("Put to a snapshot = %v, want ErrReadOnly", err)
	}
	if _, err := e.reg.Put(e.ctx, "nope", mounts.PutRequest{Path: "x", Data: []byte("x")}); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("Put to a missing mount = %v, want ErrNotFound", err)
	}
}

func TestFSAndHostPath(t *testing.T) {
	e := newEnv(t)
	if _, err := e.reg.Create(e.ctx, mounts.Mount{Name: "boards", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeSnapshot}); err != nil {
		t.Fatal(err)
	}
	fsys := e.reg.FS(e.ctx)
	top, err := fs.ReadDir(fsys, ".")
	if err != nil || len(top) != 2 {
		t.Fatalf("FS root = %v, %v; want boards and notes", top, err)
	}
	if b, err := fs.ReadFile(fsys, "boards/board.kicad_sch"); err != nil || string(b) != "at c0ffee" {
		t.Errorf("FS read of a snapshot = %q, %v", b, err)
	}
	if p, err := e.reg.HostPath(e.ctx, uri(t, "mount://notes/hello.md")); err != nil || p != filepath.Join(e.notes, "hello.md") {
		t.Errorf("HostPath(folder) = %q, %v", p, err)
	}
	if _, err := e.reg.HostPath(e.ctx, uri(t, "mount://boards/board.kicad_sch")); !errors.Is(err, mounts.ErrNotLocal) {
		t.Errorf("HostPath(a driver with no Localizer) = %v, want ErrNotLocal", err)
	}
}

func TestTheRegistryReadsAMountAtAVersionThroughItsDriver(t *testing.T) {
	e := newEnv(t)
	v, moves, err := e.reg.ResolveAt(e.ctx, "notes", "")
	if err != nil || v != "" || !moves {
		t.Errorf("ResolveAt(notes) = %q, %v, %v; want no version, moving", v, moves, err)
	}
	files, err := e.reg.ListAt(e.ctx, "notes", "", "")
	if err != nil || len(files) != 1 || files[0].Path != "hello.md" {
		t.Errorf("ListAt(notes) = %v, %v; want hello.md", files, err)
	}
	if _, err := e.reg.ListAt(e.ctx, "notes", "", "../outside"); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("ListAt of a path climbing out = %v, want ErrInvalidPath", err)
	}
	if _, _, err := e.reg.FetchAt(e.ctx, "notes", "", mounts.File{}); !errors.Is(err, mounts.ErrInvalidPath) {
		t.Errorf("FetchAt of the root = %v, want ErrInvalidPath", err)
	}
	if _, _, err := e.reg.ResolveAt(e.ctx, "nope", ""); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("ResolveAt of an unknown mount = %v, want ErrNotFound", err)
	}
	if _, err := e.reg.Create(e.ctx, mounts.Mount{Name: "boards", Kind: mounts.KindGitHub, Source: "github://acme/boards", Mode: mounts.ModeSnapshot, Connection: "acme-app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.reg.ListAt(e.ctx, "boards", "", ""); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("ListAt through a driver that can't read versions = %v, want ErrInvalidArgument", err)
	}
}
