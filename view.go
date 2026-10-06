package goapplib

import (
	"fmt"
	"net/http"
)

// View is the interface that all pages/views must implement.
// AC is the application context type (e.g., *WeewarApp).
type View[AC any] interface {
	// Load prepares the view data from the request.
	// app provides access to the application context and templates.
	// Returns (error, finished):
	//   - error: non-nil if an error occurred (will be displayed)
	//   - finished: true if response was already written (redirect, etc.)
	Load(r *http.Request, w http.ResponseWriter, app *App[AC]) (err error, finished bool)
}

// Loader is one step of a page's Load, run in order by LoadAll. goapplib's mixins (BasePage,
// WithPagination, WithFiltering, WithAuth, WithHtmx) are Loaders as they are, so a page chains them
// with LoadAll. app is the page's *App[AC], passed through as is; a loader that needs it typed is a
// LoaderFunc[AC].
//
// Before 0.7.0 this was Loader[AC], with app typed as *App[AC], which no built-in mixin satisfied
// (issue 107).
type Loader interface {
	Load(r *http.Request, w http.ResponseWriter, app any) (err error, finished bool)
}

// LoaderFunc is a typed Loader: an app's own loading step, which gets app as its *App[AC]. Given
// an app of any other type, it doesn't run, and returns an error naming both types, which LoadAll
// returns, so a page renders a 500 rather than panicking.
type LoaderFunc[AC any] func(r *http.Request, w http.ResponseWriter, app *App[AC]) (error, bool)

// Load implements Loader.
func (f LoaderFunc[AC]) Load(r *http.Request, w http.ResponseWriter, app any) (error, bool) {
	a, ok := app.(*App[AC])
	if !ok {
		return fmt.Errorf("goapplib: a LoaderFunc for %T was given an app of type %T", a, app), false
	}
	return f(r, w, a)
}

// LoadAll runs loaders in order with the page's app, and stops at the first one that returns an
// error or finished = true, returning what it returned. A nil loader is skipped. A page typically
// calls it from its own Load:
//
//	func (p *GamesPage) Load(r *http.Request, w http.ResponseWriter, app *goapplib.App[*Site]) (error, bool) {
//		return goapplib.LoadAll(r, w, app, &p.BasePage, &p.WithPagination, goapplib.LoaderFunc[*Site](p.loadGames))
//	}
func LoadAll(r *http.Request, w http.ResponseWriter, app any, loaders ...Loader) (error, bool) {
	for _, loader := range loaders {
		if loader == nil {
			continue
		}
		if err, finished := loader.Load(r, w, app); finished || err != nil {
			return err, finished
		}
	}
	return nil, false
}

// PageGroup is the interface for a group of related pages.
// Implement this to define a set of routes under a common prefix.
type PageGroup[AC any] interface {
	// RegisterRoutes returns a ServeMux with all routes for this group.
	// Patterns should be relative (prefix is stripped by RegisterGroup).
	RegisterRoutes(app *App[AC]) *http.ServeMux
}
