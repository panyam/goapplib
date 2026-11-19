package notes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
)

func call(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(b)))
	return w
}

func TestNotesInMounts(t *testing.T) {
	gh := githubtest.New(t)
	repo := gh.AddRepo("acme", "public-notes", false, map[string]string{"README.md": "hi"})
	app, err := Open(Config{StoreDir: t.TempDir(), NotesDir: t.TempDir(), GitHubAPI: gh.URL})
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler()

	if w := call(t, h, "POST", "/notes", map[string]string{"title": "a", "mount": "nowhere", "text": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("a note in a missing mount = %d %s, want 404", w.Code, w.Body)
	}
	if w := call(t, h, "POST", "/mounts", map[string]any{"Name": "pub", "Source": repo.Source(), "Mode": "live"}); w.Code != http.StatusOK {
		t.Fatalf("a mount over a public repo with no connection = %d %s", w.Code, w.Body)
	}
	if w := call(t, h, "POST", "/notes", map[string]string{"title": "b", "mount": "pub", "text": "y"}); w.Code != http.StatusOK {
		t.Fatalf("a note in the GitHub mount = %d %s", w.Code, w.Body)
	}
	if w := call(t, h, "DELETE", "/mounts/pub", nil); w.Code != http.StatusConflict {
		t.Errorf("deleting a mount a note points into = %d %s, want 409", w.Code, w.Body)
	}
	if w := call(t, h, "DELETE", "/mounts/local", nil); w.Code != http.StatusConflict {
		t.Errorf("deleting the fixed folder mount = %d %s, want 409", w.Code, w.Body)
	}
	if w := call(t, h, "POST", "/connections", map[string]any{"ID": "leak", "Kind": "github-token", "Scope": "server", "TokenRef": "ghp_pasted_secret"}); w.Code != http.StatusBadRequest {
		t.Errorf("a connection with a pasted token = %d %s, want 400", w.Code, w.Body)
	}
	var list []Note
	w := call(t, h, "GET", "/notes", nil)
	if json.Unmarshal(w.Body.Bytes(), &list); len(list) != 1 || list[0].Mount != "pub" {
		t.Errorf("GET /notes = %s", w.Body)
	}
}
