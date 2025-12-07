package goapplib

import (
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

// Loader is the interface for mixins that can be loaded.
// AC is the application context type.
type Loader[AC any] interface {
	Load(r *http.Request, w http.ResponseWriter, app *App[AC]) (err error, finished bool)
}

// LoaderFunc wraps a function as a Loader.
type LoaderFunc[AC any] func(r *http.Request, w http.ResponseWriter, app *App[AC]) (error, bool)

func (f LoaderFunc[AC]) Load(r *http.Request, w http.ResponseWriter, app *App[AC]) (error, bool) {
	return f(r, w, app)
}

// LoadAll chains multiple loaders, stopping on first error or finished=true.
func LoadAll[AC any](r *http.Request, w http.ResponseWriter, app *App[AC], loaders ...Loader[AC]) (error, bool) {
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
