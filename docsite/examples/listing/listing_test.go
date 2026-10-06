package listing

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goal "github.com/panyam/goapplib"
)

func newHandler() http.Handler {
	site := &Site{Projects: []Project{
		{"1", "Atlas", "Maps"}, {"2", "Beacon", "Alerts"}, {"3", "Atrium", "Lobby"},
	}}
	return NewHandler(goal.NewApp(site, goal.SetupTemplates("templates", "../../../templates")))
}

func do(h http.Handler, method, path string, htmx bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if htmx {
		r.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func body(rec *httptest.ResponseRecorder) string { b, _ := io.ReadAll(rec.Body); return string(b) }

func TestTheListingIsAWholePage(t *testing.T) {
	rec := do(newHandler(), "GET", "/projects/?q=at", false)
	b := body(rec)
	if rec.Code != 200 || !strings.Contains(b, "<html") || !strings.Contains(b, "2 projects") ||
		!strings.Contains(b, "Atlas") || !strings.Contains(b, "Atrium") || strings.Contains(b, "Beacon") {
		t.Errorf("GET /projects/?q=at: %d\n%s", rec.Code, b)
	}
}

func TestAnHtmxSearchGetsOnlyTheResults(t *testing.T) {
	rec := do(newHandler(), "GET", "/projects/?q=be", true)
	b := body(rec)
	if rec.Code != 200 || strings.Contains(b, "<html") || !strings.Contains(b, `id="results"`) ||
		!strings.Contains(b, "Beacon") || strings.Contains(b, "Atlas") {
		t.Errorf("htmx GET /projects/?q=be: %d\n%s", rec.Code, b)
	}
}

func TestDeletingAnswersHtmxOutOfBand(t *testing.T) {
	h := newHandler()
	rec := do(h, "DELETE", "/projects/2", true)
	if b := body(rec); rec.Code != 200 || rec.Header().Get("HX-Trigger") != "entityUpdated" ||
		!strings.Contains(b, `hx-swap-oob="true"`) || !strings.Contains(b, "2 projects") {
		t.Errorf("htmx DELETE: %d, HX-Trigger %q\n%s", rec.Code, rec.Header().Get("HX-Trigger"), b)
	}
	if rec := do(h, "DELETE", "/projects/1", false); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/projects/" {
		t.Errorf("plain DELETE: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestTheEditorIsABorderLayout(t *testing.T) {
	rec := do(newHandler(), "GET", "/editor", false)
	b := body(rec)
	for _, want := range []string{`id="border-layout-wrapper"`, "<b>Editor</b>", ">Ready<", `id="canvas"`} {
		if !strings.Contains(b, want) {
			t.Errorf("the editor page has no %q", want)
		}
	}
	if strings.Contains(b, `id="theme-toggle-button"`) {
		t.Error("CustomHeader is set, but goapplib's header rendered anyway")
	}
}

func TestThePagerShowsWhereYouAre(t *testing.T) {
	rec := do(newHandler(), "GET", "/projects/?pageSize=1&page=1", false)
	b := body(rec)
	if rec.Code != 200 || !strings.Contains(b, `Showing page <span class="font-medium">2</span>`) ||
		!strings.Contains(b, `of <span class="font-medium">3</span>`) || !strings.Contains(b, "?page=2") {
		t.Errorf("page 1 of 3 at one per page: %d\n%s", rec.Code, b)
	}
}
