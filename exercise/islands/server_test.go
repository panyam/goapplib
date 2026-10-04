package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/panyam/goapplib/page"
)

func TestPageWritesAValidSpecAndAFallbackInEachSlot(t *testing.T) {
	h, err := newHandler(t.TempDir(), filepath.Join("..", "..", "templates", "page", "Islands.html"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	html := string(body)

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
