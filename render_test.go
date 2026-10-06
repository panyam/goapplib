package goapplib

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// halfPage's template writes its header and then fails on Body.
type halfPage struct{}

func (p *halfPage) Load(r *http.Request, w http.ResponseWriter, app *App[*testSite]) (error, bool) {
	return nil, false
}

func (p *halfPage) Body() (string, error) { return "", errors.New("body failed") }

func halfPageApp(t *testing.T) *App[*testSite] {
	t.Helper()
	dir := t.TempDir()
	tmpl := `{{ define "halfPage" }}<header>the header</header>{{ .Body }}{{ end }}`
	if err := os.WriteFile(filepath.Join(dir, "halfPage.html"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewApp(&testSite{}, SetupTemplates(dir))
}

func TestRenderTemplateWritesNothingWhenRenderingFails(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := halfPageApp(t).RenderTemplate(rec, "halfPage", "halfPage", &halfPage{}); err == nil {
		t.Fatal("want a render error")
	}
	if rec.Body.Len() != 0 || rec.Code != http.StatusOK || rec.Flushed {
		t.Errorf("wrote %q before failing, want nothing written", rec.Body.String())
	}
}

func TestRegisterAnswers500WithNoPartialPageOnARenderError(t *testing.T) {
	rec := get(t, Register[*halfPage](halfPageApp(t), nil, "/{$}"), "/")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "the header") {
		t.Errorf("500 carries part of the page: %q", rec.Body.String())
	}
}
