package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/panyam/goapplib/page"
)

// get renders the page over a dist holding page/testdata/esbuild-meta.json, a real metafile whose
// islands are hero, below and narrow.
func get(t *testing.T) string {
	t.Helper()
	dist := t.TempDir()
	meta, err := os.ReadFile(filepath.Join("..", "..", "page", "testdata", "esbuild-meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(dist, filepath.Join("..", "..", "templates", "page", "Islands.html"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	return string(body)
}

func TestPagePreloadsTheEagerIslandsChunksOnly(t *testing.T) {
	var got []string
	for _, m := range regexp.MustCompile(`<link rel="modulepreload" href="([^"]+)">`).FindAllStringSubmatch(get(t), -1) {
		got = append(got, m[1])
	}
	want := []string{"/static/chunks/hero-HVBOBZWN.js", "/static/chunks/chunk-I2CYLLOU.js", "/static/chunks/chunk-M4DPGN35.js"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("modulepreload links %v, want %v", got, want)
	}
}

func TestPageWritesAValidSpecAndAFallbackInEachSlot(t *testing.T) {
	html := get(t)

	m := regexp.MustCompile(`(?s)<script type="application/json" id="page-spec">(.*?)</script>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no #page-spec script in:\n%s", html)
	}
	var spec page.Spec
	if err := json.Unmarshal([]byte(m[1]), &spec); err != nil {
		t.Fatalf("spec JSON: %v\n%s", err, m[1])
	}
	if err := spec.Validate(); err != nil {
		t.Errorf("spec doesn't validate: %v", err)
	}
	loads := map[string]string{}
	for _, is := range spec.Islands {
		loads[is.Name] = is.Load
		slot := regexp.MustCompile(`(?s)<div data-slot="` + is.Slot + `"[^>]*>(.*?)</div>`).FindStringSubmatch(html)
		if slot == nil || !strings.Contains(slot[1], "data-fallback") {
			t.Errorf("slot %q has no fallback", is.Slot)
		}
	}
	want := map[string]string{"hero": "", "below": "visible", "narrow": "media:(max-width: 600px)"}
	for name, load := range want {
		if got, ok := loads[name]; !ok || got != load {
			t.Errorf("island %q: load %q (present %v), want %q", name, got, ok, load)
		}
	}
}

func TestSpecNamesOnlyIslandsTheBuildHas(t *testing.T) {
	assets, err := page.LoadEsbuildMetafile(filepath.Join("..", "..", "page", "testdata", "esbuild-meta.json"), page.EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
	if err != nil {
		t.Fatal(err)
	}
	if err := page.CheckIslands(assets.Names(), spec); err != nil {
		t.Fatal(err)
	}
}
