package goapplib

import (
	"net/http"
	"reflect"
)

// MuxBuilder builds a ServeMux fluently, as an alternative to calling Register and RegisterGroup:
//
//	mux := app.NewMux().
//		Page("/{$}", func() goapplib.View[*Site] { return &HomePage{} }).
//		Group("/games", func(m *goapplib.MuxBuilder[*Site]) {
//			m.Page("/{id}", func() goapplib.View[*Site] { return &GamePage{} })
//		}).
//		Build()
//
// A Page renders exactly as Register would render the same view type.
type MuxBuilder[AC any] struct {
	app        *App[AC]
	mux        *http.ServeMux
	middleware []func(http.Handler) http.Handler
}

// handle registers h at pattern, wrapped in the middleware added by Use so far.
func (b *MuxBuilder[AC]) handle(pattern string, h http.Handler) {
	for i := len(b.middleware) - 1; i >= 0; i-- {
		h = b.middleware[i](h)
	}
	b.mux.Handle(pattern, h)
}

// Page registers a view at pattern. maker returns a fresh view for each request, and the view's
// type picks its template the way Register's does (HomePage renders HomePage.html's HomePage
// block), unless WithTemplate says otherwise.
func (b *MuxBuilder[AC]) Page(pattern string, maker func() View[AC], opts ...Option) *MuxBuilder[AC] {
	b.handle(pattern, pageHandler(b.app, typeNameFromValue(maker()), maker, opts))
	return b
}

// Group mounts the routes setup adds under prefix, which is stripped before they match, so a route
// "/{id}" in a Group("/games", ...) answers /games/7. Middleware added by Use before the Group wraps
// the whole group.
func (b *MuxBuilder[AC]) Group(prefix string, setup func(*MuxBuilder[AC])) *MuxBuilder[AC] {
	sub := &MuxBuilder[AC]{app: b.app, mux: http.NewServeMux()}
	setup(sub)
	mountPattern := prefix
	if len(prefix) > 0 && prefix[len(prefix)-1] != '/' {
		mountPattern = prefix + "/"
	}
	b.handle(mountPattern, http.StripPrefix(prefix, sub.mux))
	return b
}

// Handler registers an http.Handler.
func (b *MuxBuilder[AC]) Handler(pattern string, h http.Handler) *MuxBuilder[AC] {
	b.handle(pattern, h)
	return b
}

// HandleFunc registers an http.HandlerFunc.
func (b *MuxBuilder[AC]) HandleFunc(pattern string, h http.HandlerFunc) *MuxBuilder[AC] {
	b.handle(pattern, h)
	return b
}

// Static serves the files in dir at pattern, with pattern stripped from the path.
func (b *MuxBuilder[AC]) Static(pattern string, dir string) *MuxBuilder[AC] {
	b.handle(pattern, http.StripPrefix(pattern, http.FileServer(http.Dir(dir))))
	return b
}

// Use adds middleware to every route registered after it on this builder, groups included. Routes
// registered before it aren't wrapped. Middleware added by several Uses runs in the order added.
func (b *MuxBuilder[AC]) Use(mw func(http.Handler) http.Handler) *MuxBuilder[AC] {
	b.middleware = append(b.middleware, mw)
	return b
}

// Build returns the constructed ServeMux.
func (b *MuxBuilder[AC]) Build() *http.ServeMux {
	return b.mux
}

// Mux returns the underlying ServeMux for direct access.
func (b *MuxBuilder[AC]) Mux() *http.ServeMux {
	return b.mux
}

// typeNameFromValue is the name of v's type, or of what it points to: "HomePage" for a *HomePage.
func typeNameFromValue(v any) string {
	t := reflect.TypeOf(v)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
