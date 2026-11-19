// Package exercise is the storage mission's exercise (#150), run by `make exercise-storage`. It
// runs the notes example app against a fake GitHub, restarts it over the same folders, and prints
// one line per check. TestRealGitHub reads a private repo by hand.
package exercise

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panyam/goapplib/docsite/examples/notes"
	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
)

func ok(format string, args ...any) {
	fmt.Printf("exercise-storage: ok   "+format+"\n", args...)
}

type client struct {
	t   *testing.T
	srv *httptest.Server
}

func (c client) do(method, path string, body, out any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, rd)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		c.t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, b)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			c.t.Fatalf("%s %s: decoding %q: %v", method, path, b, err)
		}
	}
}

func (c client) text(path string) string {
	c.t.Helper()
	resp, err := http.Get(c.srv.URL + path)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("GET %s: %d %s", path, resp.StatusCode, b)
	}
	return string(b)
}

func start(t *testing.T, cfg notes.Config) client {
	t.Helper()
	app, err := notes.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	return client{t: t, srv: srv}
}

func TestStorageExercise(t *testing.T) {
	raw := make([]byte, 12)
	rand.Read(raw)
	token := "ghp_exercise_" + hex.EncodeToString(raw)
	gh := githubtest.New(t)
	gh.AddToken(token)
	repo := gh.AddRepo("acme", "notebook", true, map[string]string{"README.md": "the team's shared notebook"})
	t.Setenv("NOTES_GITHUB_TOKEN", token)

	cfg := notes.Config{StoreDir: t.TempDir(), NotesDir: t.TempDir(), GitHubAPI: gh.URL}
	c := start(t, cfg)

	var first notes.Note
	c.do("POST", "/notes", map[string]string{"title": "Groceries", "author": "ada", "mount": "local", "text": "eggs, flour, butter"}, &first)
	if b, err := os.ReadFile(filepath.Join(cfg.NotesDir, filepath.FromSlash(first.Path))); err != nil || string(b) != "eggs, flour, butter" {
		t.Fatalf("the note's text isn't in the folder mount: %q, %v", b, err)
	}
	ok("wrote note %s into the folder mount at %s", first.ID, first.Path)

	c.srv.Close()
	c = start(t, cfg)
	ok("restarted the app over the same folders")

	var got notes.NoteWithText
	c.do("GET", "/notes/"+first.ID, nil, &got)
	if got.Title != "Groceries" || got.Text != "eggs, flour, butter" {
		t.Fatalf("after the restart, note = %+v", got)
	}
	ok("read note %s back after the restart: %q", first.ID, got.Text)

	c.do("POST", "/connections", map[string]any{"ID": "acme", "Kind": "github-token", "Scope": "server", "Name": "acme org", "TokenRef": "env:NOTES_GITHUB_TOKEN"}, nil)
	ok("made connection acme, whose record names env:NOTES_GITHUB_TOKEN")

	c.do("POST", "/mounts", map[string]any{"Name": "shared", "Source": repo.Source(), "Mode": "live", "Connection": "acme"}, nil)
	before := repo.Head("main")
	var shared notes.Note
	c.do("POST", "/notes", map[string]string{"title": "Standup", "author": "ada", "mount": "shared", "text": "ship the storage module"}, &shared)
	if repo.Head("main") == before {
		t.Fatal("writing a note to the GitHub mount made no commit")
	}
	if s, _ := repo.File("main", shared.Path); s != "ship the storage module" {
		t.Fatalf("the fake repo has %q at %s", s, shared.Path)
	}
	ok("wrote note %s to the private repo as commit %s", shared.ID, repo.Head("main")[:7])

	var pinned struct{ Version string }
	c.do("POST", "/mounts", map[string]any{"Name": "pinned", "Source": repo.Source(), "Mode": "snapshot", "Connection": "acme"}, &pinned)
	at := repo.Head("main")
	if pinned.Version != at {
		t.Fatalf("the snapshot is at %q, want the head %q", pinned.Version, at)
	}
	repo.Commit("main", map[string]string{"README.md": "rewritten after the snapshot"}, nil)
	ok("made snapshot mount pinned at commit %s, then changed the repo", at[:7])

	c.srv.Close()
	c = start(t, cfg)
	reqs := len(gh.Requests())
	if s := c.text("/files/pinned/README.md"); s != "the team's shared notebook" {
		t.Fatalf("after a restart, the snapshot reads %q, want the bytes at %s", s, at[:7])
	}
	if n := len(gh.Requests()) - reqs; n != 0 {
		t.Fatalf("reading the snapshot after a restart made %d GitHub requests", n)
	}
	if s := c.text("/files/shared/README.md"); s != "rewritten after the snapshot" {
		t.Fatalf("the live mount reads %q, want the branch's latest", s)
	}
	ok("after a restart, pinned still reads commit %s with no GitHub requests, and shared reads the latest", at[:7])

	var ms []struct{ Name, Kind, Mode, Version string }
	c.do("GET", "/mounts", nil, &ms)
	var cs []struct{ ID, Kind, Scope, TokenRef string }
	c.do("GET", "/connections", nil, &cs)
	var names []string
	for _, m := range ms {
		names = append(names, fmt.Sprintf("%s (%s %s)", m.Name, m.Kind, m.Mode))
	}
	if len(ms) != 3 || len(cs) != 1 || cs[0].TokenRef != "env:NOTES_GITHUB_TOKEN" {
		t.Fatalf("mounts %v, connections %+v", ms, cs)
	}
	ok("the app holds mounts %s and connection %s (%s)", strings.Join(names, ", "), cs[0].ID, cs[0].Kind)

	for _, r := range gh.Requests() {
		if r.Token != token {
			t.Fatalf("%s %s reached GitHub without the connection's token", r.Method, r.Path)
		}
	}
	ok("every one of %d requests to GitHub carried the connection's token", len(gh.Requests()))

	files := 0
	for _, dir := range []string{cfg.StoreDir, cfg.NotesDir} {
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			files++
			if bytes.Contains(b, []byte(token)) {
				t.Errorf("the token is on disk in %s", p)
			}
			return nil
		})
	}
	if t.Failed() {
		t.FailNow()
	}
	ok("no secret in any of the %d files on disk", files)
	fmt.Println("exercise-storage: ok")
}

// TestRealGitHub reads a file from a private repo on github.com through a token connection. It
// runs only by hand, with GITHUB_TOKEN set and STORAGE_EXERCISE_REPO=owner/repo.
func TestRealGitHub(t *testing.T) {
	repo := os.Getenv("STORAGE_EXERCISE_REPO")
	if os.Getenv("GITHUB_TOKEN") == "" || repo == "" {
		fmt.Println("exercise-storage: skip reading a private repo on github.com (set GITHUB_TOKEN and STORAGE_EXERCISE_REPO=owner/repo)")
		t.Skip("GITHUB_TOKEN and STORAGE_EXERCISE_REPO not set")
	}
	cfg := notes.Config{StoreDir: t.TempDir(), NotesDir: t.TempDir()}
	c := start(t, cfg)
	c.do("POST", "/connections", map[string]any{"ID": "mine", "Kind": "github-token", "Scope": "server", "TokenRef": "env:GITHUB_TOKEN"}, nil)
	var m struct{ Version string }
	c.do("POST", "/mounts", map[string]any{"Name": "private", "Source": "github://" + repo, "Mode": "snapshot", "Connection": "mine"}, &m)
	var list []string
	c.do("GET", "/files/private/", nil, &list)
	if len(list) == 0 {
		t.Fatalf("%s at %s lists no files", repo, m.Version)
	}
	ok("read %s at %s through a token connection: %s", repo, m.Version[:7], strings.Join(list, ", "))
}
