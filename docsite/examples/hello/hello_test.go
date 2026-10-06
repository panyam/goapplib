package hello

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHomePageRenders(t *testing.T) {
	h := NewHandler(&Site{Name: "Hello", Tagline: "A goapplib app."}, "templates", "../../../templates")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	body, _ := io.ReadAll(rec.Body)
	if rec.Code != 200 {
		t.Fatalf("GET /: %d\n%s", rec.Code, body)
	}
	for _, want := range []string{
		"<title>Home</title>",           // BasePage's TitleSection, from p.Title
		`<h1 class="text-3xl font-bold">Hello</h1>`, // the page's BodySection
		"<p>A goapplib app.</p>",
		`id="theme-toggle-button"`, // goapplib's Header, so the layout came from goapplib
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the page has no %q", want)
		}
	}
}
