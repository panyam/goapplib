package goapplib

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// htmxPage is HtmxAware through its embedded WithHtmx.
type htmxPage struct {
	WithHtmx
}

func (p *htmxPage) Load(r *http.Request, w http.ResponseWriter, app *App[*testSite]) (error, bool) {
	return LoadAll(r, w, app, &p.WithHtmx)
}

func getHtmx(t *testing.T, h http.Handler, headers map[string]string) string {
	t.Helper()
	r := httptest.NewRequest("GET", "/", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Body.String()
}

func TestWithFragmentTemplateRendersTheFragmentForHtmx(t *testing.T) {
	app := recordingApp(nil)
	fragment := WithFragmentTemplate("Results")
	handlers := map[string]http.Handler{
		"Register":      Register[*htmxPage](app, nil, "/{$}", fragment),
		"MuxBuilder":    app.NewMux().Page("/{$}", func() View[*testSite] { return &htmxPage{} }, fragment).Build(),
		"SmartRegister": SmartRegister[*htmxPage](app, nil, "/{$}", "htmxPage", "Results"),
	}
	for name, h := range handlers {
		for _, c := range []struct {
			headers map[string]string
			want    string
		}{
			{nil, "htmxPage|htmxPage"},
			{map[string]string{"HX-Request": "true"}, "Results|Results"},
			// A boosted link is a whole-page navigation, so it gets the full page.
			{map[string]string{"HX-Request": "true", "HX-Boosted": "true"}, "htmxPage|htmxPage"},
		} {
			if got := getHtmx(t, h, c.headers); got != c.want {
				t.Errorf("%s with %v rendered %q, want %q", name, c.headers, got, c.want)
			}
		}
	}
}

func TestWithFragmentTemplateIsIgnoredByAViewThatIsntHtmxAware(t *testing.T) {
	app := recordingApp(nil)
	h := Register[*muxHomePage](app, nil, "/{$}", WithFragmentTemplate("Results"))
	if got := getHtmx(t, h, map[string]string{"HX-Request": "true"}); got != "muxHomePage|muxHomePage" {
		t.Errorf("rendered %q, want the full page", got)
	}
}
