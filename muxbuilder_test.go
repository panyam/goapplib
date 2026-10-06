package goapplib

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type muxHomePage struct{ Title string }

func (p *muxHomePage) Load(r *http.Request, w http.ResponseWriter, app *App[*testSite]) (error, bool) {
	p.Title = "home"
	return nil, false
}

// recordingApp renders "file|block" for each page, or fails with failRender.
func recordingApp(failRender error) *App[*testSite] {
	app := NewApp(&testSite{}, nil)
	app.RenderTemplateFunc = func(w http.ResponseWriter, file, block string, view any) error {
		if failRender != nil {
			return failRender
		}
		_, err := w.Write([]byte(file + "|" + block))
		return err
	}
	return app
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestMuxBuilderPageRendersTheSameTemplateAsRegister(t *testing.T) {
	app := recordingApp(nil)
	built := app.NewMux().Page("/{$}", func() View[*testSite] { return &muxHomePage{} }).Build()
	registered := Register[*muxHomePage](app, nil, "/{$}")
	for name, h := range map[string]http.Handler{"MuxBuilder.Page": built, "Register": registered} {
		if rec := get(t, h, "/"); rec.Code != 200 || rec.Body.String() != "muxHomePage|muxHomePage" {
			t.Errorf("%s: %d %q, want 200 \"muxHomePage|muxHomePage\"", name, rec.Code, rec.Body.String())
		}
	}
}

func TestMuxBuilderPageHonoursWithTemplate(t *testing.T) {
	app := recordingApp(nil)
	h := app.NewMux().
		Page("/a", func() View[*testSite] { return &muxHomePage{} }, WithTemplate("pages/Other")).
		Page("/b", func() View[*testSite] { return &muxHomePage{} }, WithTemplate("pages/Other:Block")).
		Build()
	for path, want := range map[string]string{"/a": "pages/Other|Other", "/b": "pages/Other|Block"} {
		if got := get(t, h, path).Body.String(); got != want {
			t.Errorf("%s rendered %q, want %q", path, got, want)
		}
	}
}

func TestMuxBuilderPageAnswers500OnARenderError(t *testing.T) {
	app := recordingApp(errors.New("no such template"))
	h := app.NewMux().Page("/{$}", func() View[*testSite] { return &muxHomePage{} }).Build()
	if rec := get(t, h, "/"); rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
}

func TestMuxBuilderUseWrapsLaterRoutesOnly(t *testing.T) {
	app := recordingApp(nil)
	tag := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Tag", "yes")
			next.ServeHTTP(w, r)
		})
	}
	ok := func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }
	h := app.NewMux().
		HandleFunc("/before", ok).
		Use(tag).
		HandleFunc("/after", ok).
		Page("/page", func() View[*testSite] { return &muxHomePage{} }).
		Group("/g", func(m *MuxBuilder[*testSite]) { m.HandleFunc("/x", ok) }).
		Build()
	for path, want := range map[string]string{"/before": "", "/after": "yes", "/page": "yes", "/g/x": "yes"} {
		rec := get(t, h, path)
		if rec.Code != 200 || rec.Header().Get("X-Tag") != want {
			t.Errorf("%s: %d, X-Tag %q, want 200 and %q", path, rec.Code, rec.Header().Get("X-Tag"), want)
		}
	}
}

func TestMuxBuilderGroupStripsItsPrefix(t *testing.T) {
	app := recordingApp(nil)
	h := app.NewMux().Group("/games", func(m *MuxBuilder[*testSite]) {
		m.HandleFunc("/{id}", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("view " + r.PathValue("id"))) })
		m.Group("/admin", func(m *MuxBuilder[*testSite]) {
			m.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("admin")) })
		})
	}).Build()
	for path, want := range map[string]string{"/games/7": "view 7", "/games/admin/": "admin"} {
		if got := get(t, h, path).Body.String(); !strings.Contains(got, want) {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
}
