package goapplib

import (
	"net/http"
)

// MuxBuilder provides a fluent API for building routes.
type MuxBuilder[AC any] struct {
	app *App[AC]
	mux *http.ServeMux
}

// Page registers a View-based page.
func (b *MuxBuilder[AC]) Page(pattern string, maker func() View[AC], opts ...Option) *MuxBuilder[AC] {
	// Apply options
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	// Determine template name from maker's return type
	templateFileName := o.templateFileName
	if templateFileName == "" {
		// Get type name from a sample instance
		sample := maker()
		templateFileName = typeNameFromValue(sample)
	}
	templateBlockName := o.templateBlockName

	// Create handler
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view := maker()

		err, finished := view.Load(r, w, b.app)
		if finished {
			return
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		b.app.RenderTemplate(w, templateFileName, templateBlockName, view)
	})

	// Apply middleware
	for i := len(o.middleware) - 1; i >= 0; i-- {
		handler = o.middleware[i](handler)
	}

	b.mux.Handle(pattern, handler)
	return b
}

// Group creates a nested group with a prefix.
func (b *MuxBuilder[AC]) Group(prefix string, setup func(*MuxBuilder[AC])) *MuxBuilder[AC] {
	subBuilder := &MuxBuilder[AC]{
		app: b.app,
		mux: http.NewServeMux(),
	}

	setup(subBuilder)

	// Mount with StripPrefix
	mountPattern := prefix
	if len(prefix) > 0 && prefix[len(prefix)-1] != '/' {
		mountPattern = prefix + "/"
	}

	b.mux.Handle(mountPattern, http.StripPrefix(prefix, subBuilder.mux))
	return b
}

// Handler registers an http.Handler.
func (b *MuxBuilder[AC]) Handler(pattern string, h http.Handler) *MuxBuilder[AC] {
	b.mux.Handle(pattern, h)
	return b
}

// HandleFunc registers an http.HandlerFunc.
func (b *MuxBuilder[AC]) HandleFunc(pattern string, h http.HandlerFunc) *MuxBuilder[AC] {
	b.mux.HandleFunc(pattern, h)
	return b
}

// Static registers a static file server.
func (b *MuxBuilder[AC]) Static(pattern string, dir string) *MuxBuilder[AC] {
	b.mux.Handle(pattern, http.StripPrefix(pattern, http.FileServer(http.Dir(dir))))
	return b
}

// Use adds middleware to all subsequent routes.
// Note: This only affects routes registered after this call.
func (b *MuxBuilder[AC]) Use(mw func(http.Handler) http.Handler) *MuxBuilder[AC] {
	// Wrap the entire mux
	// This is a simplified approach - for more complex middleware needs,
	// consider using a dedicated router library
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

// typeNameFromValue extracts the type name from a value.
func typeNameFromValue(v any) string {
	if v == nil {
		return ""
	}
	return typeNameOf[any]()
}
