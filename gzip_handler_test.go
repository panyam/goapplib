package goapplib

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// GzipFileServer serves a .gz beside a file inside root, and a path that climbs out of root learns
// nothing about the files there, not even from Content-Encoding.
func TestGzipFileServerLooksOnlyInsideRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "public")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{
		filepath.Join(root, "app.js"):      "plain",
		filepath.Join(root, "app.js.gz"):   "zipped",
		filepath.Join(dir, "secret.js.gz"): "outside",
	} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := GzipFileServer(http.Dir(root))
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.URL.Path = path
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := get("/app.js"); w.Header().Get("Content-Encoding") != "gzip" || w.Body.String() != "zipped" {
		t.Errorf("GET /app.js = %q with Content-Encoding %q; want the .gz beside it, encoded gzip", w.Body.String(), w.Header().Get("Content-Encoding"))
	}
	if w := get("/../secret.js"); w.Header().Get("Content-Encoding") != "" {
		t.Errorf("GET /../secret.js set Content-Encoding %q; want no sign of the .gz outside root", w.Header().Get("Content-Encoding"))
	}
}

// GzipFileServer finds a .gz through the http.FileSystem it serves, so a root from http.FS, such as
// an embedded folder, gets its .gz files too.
func TestGzipFileServerServesAGzFromAnyFileSystem(t *testing.T) {
	h := GzipFileServer(http.FS(fstest.MapFS{
		"app.wasm":    {Data: []byte("plain")},
		"app.wasm.gz": {Data: []byte("zipped")},
	}))
	r := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "gzip" || w.Body.String() != "zipped" || w.Header().Get("Content-Type") != "application/wasm" {
		t.Errorf("GET /app.wasm from an http.FS = %q, Content-Encoding %q, Content-Type %q; want the .gz, gzip, application/wasm", w.Body.String(), w.Header().Get("Content-Encoding"), w.Header().Get("Content-Type"))
	}
}
