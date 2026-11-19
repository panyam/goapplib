// Package notes is the example app on the storage guide page. It keeps its own records, one per
// note, in a JSON file of its own. It keeps each note's text in a mount: a folder on the server, or
// a GitHub repo it reaches through a connection. A connection's record names where its token is,
// and never holds it.
//
// The storage exercise (exercise/exercise_test.go, `make exercise-storage`) runs it against a fake
// GitHub and restarts it over the same folders.
package notes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/connections"
	connstore "github.com/panyam/goapplib/storage/connections/fsstore"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/folder"
	mountstore "github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/github"
)

// Config says where the app keeps things. Everything lives on disk, so a restart over the same
// Config sees the same notes, mounts and connections.
type Config struct {
	// StoreDir holds the app's notes.json, the mount and connection records, the blob store for
	// GitHub snapshots, and their folder views.
	StoreDir string

	// NotesDir is the folder the fixed "local" mount serves.
	NotesDir string

	// GitHubAPI is GitHub's API root. Empty means api.github.com, and the exercise sets a fake's.
	GitHubAPI string
}

// Note is the app's own record of a note. The text isn't in it. Mount and Path say where it is.
type Note struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Author  string    `json:"author"`
	Mount   string    `json:"mount"`
	Path    string    `json:"path"`
	Created time.Time `json:"created"`
}

// NoteWithText is a note and its text, as GET /notes/{id} answers it.
type NoteWithText struct {
	Note
	Text string `json:"text"`
}

// App is the notes app: its records, a mounts registry, and a connection store.
type App struct {
	records string
	mounts  *mounts.Registry
	conns   connections.Store
	mu      sync.Mutex
}

// Open builds the app over cfg's folders, creating them as needed.
func Open(cfg Config) (*App, error) {
	mountRecords, err := mountstore.New(filepath.Join(cfg.StoreDir, "mounts"))
	if err != nil {
		return nil, err
	}
	connRecords, err := connstore.New(filepath.Join(cfg.StoreDir, "connections"))
	if err != nil {
		return nil, err
	}
	blobStore, err := blobs.NewDir(filepath.Join(cfg.StoreDir, "blobs"))
	if err != nil {
		return nil, err
	}
	ghOpts := []github.Option{github.WithViews(filepath.Join(cfg.StoreDir, "views"))}
	if cfg.GitHubAPI != "" {
		ghOpts = append(ghOpts, github.WithBaseURL(cfg.GitHubAPI))
	}
	a := &App{records: filepath.Join(cfg.StoreDir, "notes.json"), conns: connRecords}
	tokens := connections.NewService(connRecords)
	a.mounts = mounts.NewRegistry(mountRecords,
		[]mounts.Driver{folder.New(), github.New(blobStore, ghOpts...)},
		mounts.WithFixed(mounts.Mount{Name: "local", Kind: mounts.KindFolder, Source: cfg.NotesDir, Mode: mounts.ModeLive}),
		mounts.WithCredentials(tokens.TokenSource),
		mounts.WithInUse(a.pointsInto),
	)
	return a, nil
}

// Handler serves the app's JSON API.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notes", a.listNotes)
	mux.HandleFunc("POST /notes", a.createNote)
	mux.HandleFunc("GET /notes/{id}", a.readNote)
	mux.HandleFunc("GET /mounts", a.listMounts)
	mux.HandleFunc("POST /mounts", a.createMount)
	mux.HandleFunc("DELETE /mounts/{name}", a.deleteMount)
	mux.HandleFunc("GET /connections", a.listConnections)
	mux.HandleFunc("POST /connections", a.putConnection)
	mux.HandleFunc("GET /files/{mount}/{path...}", a.readFile)
	return mux
}

// createNote writes the text into the mount first, and keeps the record only if that worked. In
// a GitHub mount the write is a commit.
func (a *App) createNote(w http.ResponseWriter, r *http.Request) {
	var in struct{ Title, Author, Mount, Text string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := make([]byte, 4)
	rand.Read(id)
	n := Note{ID: hex.EncodeToString(id), Title: in.Title, Author: in.Author, Mount: in.Mount, Created: time.Now().UTC()}
	n.Path = "notes/" + n.ID + ".md"
	_, err := a.mounts.Put(r.Context(), n.Mount, mounts.PutRequest{Path: n.Path, Data: []byte(in.Text), Message: "Add note: " + n.Title})
	if err != nil {
		fail(w, err)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	all, err := a.load()
	if err == nil {
		err = a.save(append(all, n))
	}
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, n)
}

func (a *App) listNotes(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	all, err := a.load()
	a.mu.Unlock()
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, all)
}

// readNote finds the record, then reads the text from wherever the record says it is.
func (a *App) readNote(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	all, err := a.load()
	a.mu.Unlock()
	if err != nil {
		fail(w, err)
		return
	}
	for _, n := range all {
		if n.ID != r.PathValue("id") {
			continue
		}
		text, err := fs.ReadFile(a.mounts.FS(r.Context()), n.Mount+"/"+n.Path)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, NoteWithText{Note: n, Text: string(text)})
		return
	}
	fail(w, mounts.ErrNotFound)
}

// readFile answers a file's bytes, or a folder's names as JSON.
func (a *App) readFile(w http.ResponseWriter, r *http.Request) {
	u, err := mounts.NewURI(r.PathValue("mount"), r.PathValue("path"))
	if err != nil {
		fail(w, err)
		return
	}
	name := u.Mount
	if u.Path != "" {
		name += "/" + u.Path
	}
	fsys := a.mounts.FS(r.Context())
	if ents, err := fs.ReadDir(fsys, name); err == nil {
		names := []string{}
		for _, e := range ents {
			names = append(names, e.Name())
		}
		reply(w, names)
		return
	}
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		fail(w, err)
		return
	}
	w.Write(b)
}

func (a *App) listMounts(w http.ResponseWriter, r *http.Request) {
	ms, err := a.mounts.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, ms)
}

// createMount makes a GitHub mount. The registry resolves its ref and, for a snapshot, downloads
// the commit through the mount's connection, before it writes the record. The request names the
// repo but never the kind, so a client can't ask for a folder on the server.
func (a *App) createMount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string
		Source     string
		Ref        string
		Mode       mounts.Mode
		Connection string
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	m, err := a.mounts.Create(r.Context(), mounts.Mount{Name: in.Name, Kind: mounts.KindGitHub,
		Source: in.Source, Ref: in.Ref, Mode: in.Mode, Connection: in.Connection})
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, m)
}

func (a *App) deleteMount(w http.ResponseWriter, r *http.Request) {
	if err := a.mounts.Delete(r.Context(), r.PathValue("name")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pointsInto is the registry's in-use check: a mount any note lives in can't be deleted.
func (a *App) pointsInto(ctx context.Context, name string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	all, err := a.load()
	for _, n := range all {
		if n.Mount == name {
			return true, err
		}
	}
	return false, err
}

func (a *App) listConnections(w http.ResponseWriter, r *http.Request) {
	cs, err := a.conns.ListConnections(r.Context(), "")
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, cs)
}

// putConnection saves a connection record. The store validates it, so a record holding a pasted
// token, rather than a reference such as env:NAME, is refused.
func (a *App) putConnection(w http.ResponseWriter, r *http.Request) {
	var c connections.Connection
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c.Created = time.Now().UTC()
	if err := a.conns.PutConnection(r.Context(), c); err != nil {
		fail(w, err)
		return
	}
	reply(w, c)
}

// load and save keep every note in one JSON file, written through a temp file and a rename. A
// real app keeps them in its database.
func (a *App) load() ([]Note, error) {
	b, err := os.ReadFile(a.records)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []Note
	return all, json.Unmarshal(b, &all)
}

func (a *App) save(all []Note) error {
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.records + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.records)
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// fail maps the storage errors onto HTTP statuses, the way an app maps them onto its transport.
func fail(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, mounts.ErrNotFound), errors.Is(err, connections.ErrNotFound), errors.Is(err, fs.ErrNotExist):
		code = http.StatusNotFound
	case errors.Is(err, mounts.ErrInvalidArgument), errors.Is(err, mounts.ErrInvalidPath), errors.Is(err, connections.ErrInvalid):
		code = http.StatusBadRequest
	case errors.Is(err, mounts.ErrExists), errors.Is(err, mounts.ErrConflict), errors.Is(err, mounts.ErrFixed),
		errors.Is(err, mounts.ErrInUse), errors.Is(err, mounts.ErrReadOnly):
		code = http.StatusConflict
	case errors.Is(err, mounts.ErrUnavailable):
		code = http.StatusBadGateway
	}
	http.Error(w, err.Error(), code)
}
