package goapplib

import (
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"
)

// Option configures registration behavior.
type Option func(*options)

type options struct {
	templateFileName  string
	templateBlockName string
	middleware        []func(http.Handler) http.Handler
}

// WithTemplate sets the template file and optional block name.
// Format: "path/to/template" or "path/to/template:BlockName"
//
// If no ":" is present, block name is auto-derived from the base filename.
// If ":" is present, the part after ":" is used as the explicit block name.
// An empty block name after ":" means render the entire template file.
//
// Examples:
//   - "CanvasListingPage" -> file: CanvasListingPage.html, block: CanvasListingPage (auto-derived)
//   - "canvases/CanvasListingPage" -> file: canvases/CanvasListingPage.html, block: CanvasListingPage (auto-derived)
//   - "canvases/CanvasListingPage:MyBlock" -> file: canvases/CanvasListingPage.html, block: MyBlock
//   - "canvases/CanvasListingPage:" -> file: canvases/CanvasListingPage.html, block: "" (entire file)
func WithTemplate(spec string) Option {
	return func(o *options) {
		o.templateFileName, o.templateBlockName = ParseTemplateSpec(spec)
	}
}

// baseFileName extracts the base name from a path (without extension).
// e.g., "canvases/CanvasListingPage" -> "CanvasListingPage"
func baseFileName(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// ParseTemplateSpec parses a template spec into file name and block name.
// Format: "path/to/template" or "path/to/template:BlockName"
//
// If no ":" is present, block name is auto-derived from the base filename.
// If ":" is present, the part after ":" is used as the explicit block name.
// An empty block name after ":" means render the entire template file.
func ParseTemplateSpec(spec string) (fileName, blockName string) {
	if idx := strings.LastIndex(spec, ":"); idx >= 0 {
		fileName = spec[:idx]
		blockName = spec[idx+1:] // Use as-is (could be empty or explicit name)
	} else {
		fileName = spec
		blockName = baseFileName(spec) // Auto-derive from base filename
	}
	return
}

// WithMiddleware adds middleware to the handler.
func WithMiddleware(mw ...func(http.Handler) http.Handler) Option {
	return func(o *options) {
		o.middleware = append(o.middleware, mw...)
	}
}

// Register registers a View at the given pattern.
// If mux is nil, a new ServeMux is created.
// Returns the mux for chaining.
//
// Usage:
//
//	mux := http.NewServeMux()
//	goapplib.Register[HomePage](app, mux, "/")
//	goapplib.Register[GameListingPage](app, mux, "/games/")
func Register[V View[AC], AC any](
	app *App[AC],
	mux *http.ServeMux,
	pattern string,
	opts ...Option,
) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	// Apply options
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	// Determine template file name
	templateFileName := o.templateFileName
	if templateFileName == "" {
		templateFileName = typeNameOf[V]()
	}

	// Determine template block name
	templateBlockName := o.templateBlockName
	if templateBlockName == "" && !strings.Contains(o.templateFileName, ":") {
		// No explicit block specified, auto-derive from base filename
		templateBlockName = baseFileName(templateFileName)
	}

	// Create handler
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create new instance of view
		view := newInstance[V]()

		// Load view data - pass the whole app
		err, finished := view.Load(r, w, app)
		if finished {
			return
		}

		if err != nil {
			log.Printf("View load error for %s[%s]: %v", templateFileName, templateBlockName, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Render template
		if renderErr := app.RenderTemplate(w, templateFileName, templateBlockName, view); renderErr != nil {
			log.Printf("Render error for %s[%s]: %v", templateFileName, templateBlockName, renderErr)
			http.Error(w, "Template render error", http.StatusInternalServerError)
		}
	})

	// Apply middleware in reverse order
	for i := len(o.middleware) - 1; i >= 0; i-- {
		handler = o.middleware[i](handler)
	}

	mux.Handle(pattern, handler)
	return mux
}

// RegisterGroup registers a PageGroup under the given prefix.
// The group's routes are mounted with the prefix stripped.
//
// Usage:
//
//	goal.RegisterGroup[GamesGroup](app, rootMux, "/games")
func RegisterGroup[G PageGroup[AC], AC any](
	app *App[AC],
	mux *http.ServeMux,
	prefix string,
	opts ...Option,
) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	// Create group instance and get its routes
	group := newInstance[G]()
	groupMux := group.RegisterRoutes(app)

	// Mount with StripPrefix
	// Ensure prefix ends with / for proper matching
	mountPattern := prefix
	if len(prefix) > 0 && prefix[len(prefix)-1] != '/' {
		mountPattern = prefix + "/"
	}

	mux.Handle(mountPattern, http.StripPrefix(prefix, groupMux))

	return mux
}

// RegisterFunc registers a handler function at the given pattern.
// Convenience wrapper for mux.HandleFunc.
func RegisterFunc(
	mux *http.ServeMux,
	pattern string,
	handler http.HandlerFunc,
) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}
	mux.HandleFunc(pattern, handler)
	return mux
}

// RegisterHandler registers an http.Handler at the given pattern.
// Convenience wrapper for mux.Handle.
func RegisterHandler(
	mux *http.ServeMux,
	pattern string,
	handler http.Handler,
) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}
	mux.Handle(pattern, handler)
	return mux
}

// SmartRegister registers a View that can render as full page or fragment.
// Uses HTMX detection to choose the appropriate template.
// Template specs use the same format as WithTemplate: "path/file:BlockName"
func SmartRegister[V interface {
	View[AC]
	HtmxAware
}, AC any](
	app *App[AC],
	mux *http.ServeMux,
	pattern string,
	fullTemplateSpec string,
	fragmentTemplateSpec string,
	opts ...Option,
) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	// Apply options
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	// Parse template specs
	fullFileName, fullBlockName := ParseTemplateSpec(fullTemplateSpec)
	fragFileName, fragBlockName := ParseTemplateSpec(fragmentTemplateSpec)

	// Create handler
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view := newInstance[V]()

		err, finished := view.Load(r, w, app)
		if finished {
			return
		}

		if err != nil {
			log.Printf("View load error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Choose template based on HTMX
		fileName, blockName := fullFileName, fullBlockName
		if view.ShouldRenderFragment() {
			fileName, blockName = fragFileName, fragBlockName
		}

		if renderErr := app.RenderTemplate(w, fileName, blockName, view); renderErr != nil {
			http.Error(w, "Template render error", http.StatusInternalServerError)
		}
	})

	// Apply middleware
	for i := len(o.middleware) - 1; i >= 0; i-- {
		handler = o.middleware[i](handler)
	}

	mux.Handle(pattern, handler)
	return mux
}

// HtmxAware is implemented by views that can detect HTMX requests.
type HtmxAware interface {
	ShouldRenderFragment() bool
}

// newInstance creates a new zero instance of type T.
// T must be a pointer type.
func newInstance[T any]() T {
	var t T
	typ := reflect.TypeOf(t)

	// If T is already a pointer, create a new instance of the underlying type
	if typ.Kind() == reflect.Ptr {
		return reflect.New(typ.Elem()).Interface().(T)
	}

	// If T is not a pointer, this will panic at compile time due to type constraints
	panic(fmt.Sprintf("type %T must be a pointer type", t))
}

// typeNameOf returns the type name of T without package prefix.
func typeNameOf[T any]() string {
	var t T
	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}
