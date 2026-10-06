package goapplib

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testSite struct{ Name string }

// The chain every guide shows: goapplib's own mixins, then an app's typed loader.
type chainPage struct {
	BasePage
	WithPagination
	WithFiltering
	WithHtmx
	SiteName string
}

func (p *chainPage) Load(r *http.Request, w http.ResponseWriter, app *App[*testSite]) (error, bool) {
	return LoadAll(r, w, app, &p.BasePage, &p.WithPagination, &p.WithFiltering, &p.WithHtmx,
		LoaderFunc[*testSite](func(r *http.Request, w http.ResponseWriter, app *App[*testSite]) (error, bool) {
			p.SiteName = app.Context.Name
			return nil, false
		}))
}

func TestLoadAllChainsMixinsAndTypedLoaders(t *testing.T) {
	app := NewApp(&testSite{Name: "Site"}, nil)
	r := httptest.NewRequest("GET", "/?page=3&q=go", nil)
	r.Header.Set("HX-Request", "true")
	var p chainPage
	if err, done := p.Load(r, httptest.NewRecorder(), app); err != nil || done {
		t.Fatalf("Load: %v, %v", err, done)
	}
	if p.BodyClass == "" || p.CurrentPage != 3 || p.Query != "go" || !p.IsHtmx || p.SiteName != "Site" {
		t.Errorf("a loader didn't run: BodyClass=%q CurrentPage=%d Query=%q IsHtmx=%v SiteName=%q",
			p.BodyClass, p.CurrentPage, p.Query, p.IsHtmx, p.SiteName)
	}
}

func TestLoadAllStopsAtTheFirstErrorOrFinish(t *testing.T) {
	app := NewApp(&testSite{}, nil)
	var ran []string
	step := func(name string, err error, done bool) Loader {
		return LoaderFunc[*testSite](func(*http.Request, http.ResponseWriter, *App[*testSite]) (error, bool) {
			ran = append(ran, name)
			return err, done
		})
	}
	r := httptest.NewRequest("GET", "/", nil)

	boom := errors.New("boom")
	if err, _ := LoadAll(r, httptest.NewRecorder(), app, step("a", nil, false), step("b", boom, false), step("c", nil, false)); err != boom {
		t.Errorf("err = %v, want boom", err)
	}
	if got := strings.Join(ran, ""); got != "ab" {
		t.Errorf("ran %q, want ab", got)
	}

	ran = nil
	if _, done := LoadAll(r, httptest.NewRecorder(), app, step("a", nil, true), step("b", nil, false), nil); !done {
		t.Error("finished = false, want true")
	}
	if got := strings.Join(ran, ""); got != "a" {
		t.Errorf("ran %q, want a", got)
	}
}

func TestLoaderFuncReportsAnAppOfTheWrongType(t *testing.T) {
	other := NewApp("not a *testSite", nil)
	f := LoaderFunc[*testSite](func(*http.Request, http.ResponseWriter, *App[*testSite]) (error, bool) {
		t.Fatal("ran with the wrong app")
		return nil, false
	})
	err, done := LoadAll(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder(), other, f)
	if err == nil || done || !strings.Contains(err.Error(), "testSite") || !strings.Contains(err.Error(), "App[string]") {
		t.Errorf("err = %v, done = %v; want an error naming the type it wanted", err, done)
	}
}
