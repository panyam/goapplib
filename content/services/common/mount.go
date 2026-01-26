// Package common provides shared utilities for content services.
package common

import (
	"context"
	"net/http"
)

// MountedEntityContextKey is the context key for storing the entity ID
// extracted from the mount path parameter.
type MountedEntityContextKey struct{}

// WithMountedEntityID stores the entity ID in context.
// This is typically called by mount middleware after extracting the path parameter.
func WithMountedEntityID(ctx context.Context, entityID string) context.Context {
	return context.WithValue(ctx, MountedEntityContextKey{}, entityID)
}

// GetMountedEntityID retrieves the entity ID from context.
// Returns empty string if not set.
func GetMountedEntityID(ctx context.Context) string {
	if v := ctx.Value(MountedEntityContextKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// HandlerConfig configures how a service handler extracts entity context.
type HandlerConfig struct {
	// EntityParamName is the path parameter name that maps to entity_id.
	// The router extracts this from the URL (e.g., "songId" from /songs/{songId}).
	EntityParamName string
}

// HandlerOption configures handler behavior.
type HandlerOption func(*HandlerConfig)

// WithEntityParam specifies which router path parameter to use as entity_id.
// The mount wrapper will extract this parameter from the router's context
// (chi.URLParam, gorilla mux.Vars, or Go 1.22+ request.PathValue).
//
// Example:
//
//	// Mount likes service at /songs/{songId}/likes
//	router.Handle("/songs/{songId}/likes/", likesService.RESTHandler(
//	    common.WithEntityParam("songId"),
//	))
func WithEntityParam(paramName string) HandlerOption {
	return func(c *HandlerConfig) {
		c.EntityParamName = paramName
	}
}

// ExtractPathParam extracts a path parameter from the request using multiple strategies.
// Tries in order: Go 1.22+ PathValue, chi URLParam, gorilla mux Vars.
func ExtractPathParam(r *http.Request, name string) string {
	// Try Go 1.22+ PathValue first
	if val := r.PathValue(name); val != "" {
		return val
	}

	// Try chi URLParam via context (chi stores params in context)
	if val := chiURLParam(r, name); val != "" {
		return val
	}

	// Try gorilla mux vars
	if val := gorillaMuxVar(r, name); val != "" {
		return val
	}

	return ""
}

// chiURLParam tries to extract a URL parameter the way chi does.
// chi stores route params in context with a specific key.
func chiURLParam(r *http.Request, name string) string {
	// chi's RouteContext key
	type chiCtxKey struct{}
	if ctx := r.Context().Value(chiCtxKey{}); ctx != nil {
		// chi's RouteContext has URLParams with Keys and Values slices
		if rctx, ok := ctx.(interface {
			URLParam(string) string
		}); ok {
			return rctx.URLParam(name)
		}
	}

	// Alternative: chi also checks for chi.URLParamFromCtx
	// Try the standard chi context key pattern
	type routeCtxKey struct{}
	if ctx := r.Context().Value(routeCtxKey{}); ctx != nil {
		if rctx, ok := ctx.(interface {
			URLParam(string) string
		}); ok {
			return rctx.URLParam(name)
		}
	}

	return ""
}

// gorillaMuxVar tries to extract a variable the way gorilla/mux does.
// gorilla/mux stores vars in context.
func gorillaMuxVar(r *http.Request, name string) string {
	// gorilla/mux's varsKey
	type varsKey struct{}
	if vars := r.Context().Value(varsKey{}); vars != nil {
		if m, ok := vars.(map[string]string); ok {
			return m[name]
		}
	}
	return ""
}

// EntityParamMiddleware returns middleware that extracts a path parameter
// and stores it as the mounted entity ID in context.
func EntityParamMiddleware(paramName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entityID := ExtractPathParam(r, paramName)
			if entityID != "" {
				r = r.WithContext(WithMountedEntityID(r.Context(), entityID))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WrapWithEntityExtraction wraps a handler with entity ID extraction middleware.
// This is a convenience function for routers that don't support middleware chaining.
//
// Example:
//
//	mux.Handle("/songs/{songId}/likes/",
//	    common.WrapWithEntityExtraction("songId", likesService.RESTHandler()))
func WrapWithEntityExtraction(paramName string, handler http.Handler) http.Handler {
	return EntityParamMiddleware(paramName)(handler)
}
