// Package common provides shared utilities for content services.
//
// # Mounting Patterns
//
// This package supports multiple patterns for mounting services at custom paths:
//
// ## Pattern A: In-Process (Direct)
//
// Service instance runs in the same process as the HTTP server.
// Use RegisterXYZServiceHandlerServer or the service's RESTHandler method.
//
//	service := backends.NewGORMLikesService(db)
//	handler := service.RESTHandler(ctx, common.WithEntityParam("songId"))
//	router.Handle("/songs/{songId}/likes/", handler)
//
// ## Pattern B: Remote Endpoint (gRPC Gateway Client)
//
// HTTP gateway connects to a remote gRPC server. Entity ID flows through gRPC metadata.
//
// Gateway side (HTTP server):
//
//	mux := common.NewGatewayMux(common.WithEntityParamToMetadata("songId"))
//	likesv1.RegisterLikesServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
//	router.Handle("/songs/{songId}/likes/", mux)
//
// gRPC server side:
//
//	grpcServer := grpc.NewServer(
//	    grpc.UnaryInterceptor(common.EntityIDFromMetadataInterceptor()),
//	)
//	likesv1.RegisterLikesServiceServer(grpcServer, likesService)
package common

import (
	"context"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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

// ============================================================================
// Pattern B: Remote Endpoint (gRPC Gateway Client)
// ============================================================================

// Metadata keys for passing entity context through gRPC.
const (
	// MetadataKeyEntityID is the gRPC metadata key for entity ID.
	MetadataKeyEntityID = "x-entity-id"

	// MetadataKeyUserID is the gRPC metadata key for user ID.
	MetadataKeyUserID = "x-user-id"
)

// GatewayMuxOption configures a gRPC-gateway ServeMux for entity extraction.
type GatewayMuxOption func(*gatewayMuxConfig)

type gatewayMuxConfig struct {
	entityParamName string
	userParamName   string
	extraMetadata   func(context.Context, *http.Request) metadata.MD
}

// WithEntityParamToMetadata configures the gateway to extract a path parameter
// and send it as gRPC metadata to the remote server.
//
// Example:
//
//	mux := common.NewGatewayMux(common.WithEntityParamToMetadata("songId"))
//	likesv1.RegisterLikesServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
//	router.Handle("/songs/{songId}/likes/", mux)
func WithEntityParamToMetadata(paramName string) GatewayMuxOption {
	return func(c *gatewayMuxConfig) {
		c.entityParamName = paramName
	}
}

// WithUserParamToMetadata configures the gateway to extract a user ID path parameter
// and send it as gRPC metadata.
func WithUserParamToMetadata(paramName string) GatewayMuxOption {
	return func(c *gatewayMuxConfig) {
		c.userParamName = paramName
	}
}

// WithExtraMetadata allows adding custom metadata extraction logic.
func WithExtraMetadata(fn func(context.Context, *http.Request) metadata.MD) GatewayMuxOption {
	return func(c *gatewayMuxConfig) {
		c.extraMetadata = fn
	}
}

// NewGatewayMux creates a gRPC-gateway ServeMux configured for entity extraction.
// Use this with RegisterXYZServiceHandlerFromEndpoint for remote gRPC servers.
//
// Example:
//
//	mux := common.NewGatewayMux(
//	    common.WithEntityParamToMetadata("songId"),
//	    common.WithExtraMetadata(func(ctx context.Context, r *http.Request) metadata.MD {
//	        return metadata.Pairs("x-custom", r.Header.Get("X-Custom"))
//	    }),
//	)
//	likesv1.RegisterLikesServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
func NewGatewayMux(opts ...GatewayMuxOption) *runtime.ServeMux {
	cfg := &gatewayMuxConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return runtime.NewServeMux(
		runtime.WithMetadata(func(ctx context.Context, r *http.Request) metadata.MD {
			md := metadata.MD{}

			// Extract entity ID from path param
			if cfg.entityParamName != "" {
				if entityID := ExtractPathParam(r, cfg.entityParamName); entityID != "" {
					md.Set(MetadataKeyEntityID, entityID)
				}
			}

			// Extract user ID from path param
			if cfg.userParamName != "" {
				if userID := ExtractPathParam(r, cfg.userParamName); userID != "" {
					md.Set(MetadataKeyUserID, userID)
				}
			}

			// Add any extra metadata
			if cfg.extraMetadata != nil {
				extra := cfg.extraMetadata(ctx, r)
				for k, vals := range extra {
					md.Set(k, vals...)
				}
			}

			return md
		}),
	)
}

// NewGatewayMuxWithOptions creates a gRPC-gateway ServeMux with entity extraction
// and additional runtime.ServeMuxOption options.
func NewGatewayMuxWithOptions(gatewayOpts []GatewayMuxOption, runtimeOpts ...runtime.ServeMuxOption) *runtime.ServeMux {
	// Prepend our metadata handler to the runtime options
	allOpts := append([]runtime.ServeMuxOption{
		EntityMetadataOption(gatewayOpts...),
	}, runtimeOpts...)

	return runtime.NewServeMux(allOpts...)
}

// EntityMetadataOption returns a runtime.ServeMuxOption that extracts path parameters
// and adds them to gRPC metadata. Use this when you want to configure your own mux.
//
// Example:
//
//	mux := runtime.NewServeMux(
//	    common.EntityMetadataOption(common.WithEntityParamToMetadata("songId")),
//	    runtime.WithErrorHandler(myErrorHandler),
//	    runtime.WithMarshalerOption(...),
//	)
//	likesv1.RegisterLikesServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
func EntityMetadataOption(opts ...GatewayMuxOption) runtime.ServeMuxOption {
	cfg := &gatewayMuxConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return runtime.WithMetadata(func(ctx context.Context, r *http.Request) metadata.MD {
		md := metadata.MD{}

		if cfg.entityParamName != "" {
			if entityID := ExtractPathParam(r, cfg.entityParamName); entityID != "" {
				md.Set(MetadataKeyEntityID, entityID)
			}
		}

		if cfg.userParamName != "" {
			if userID := ExtractPathParam(r, cfg.userParamName); userID != "" {
				md.Set(MetadataKeyUserID, userID)
			}
		}

		if cfg.extraMetadata != nil {
			extra := cfg.extraMetadata(ctx, r)
			for k, vals := range extra {
				md.Set(k, vals...)
			}
		}

		return md
	})
}

// ============================================================================
// gRPC Server-Side Interceptors
// ============================================================================

// EntityIDFromMetadataInterceptor returns a gRPC unary interceptor that reads
// entity ID from incoming metadata and stores it in context.
//
// Use this on the gRPC server side when the gateway sends entity ID via metadata.
//
// Example:
//
//	grpcServer := grpc.NewServer(
//	    grpc.UnaryInterceptor(common.EntityIDFromMetadataInterceptor()),
//	)
func EntityIDFromMetadataInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(MetadataKeyEntityID); len(vals) > 0 && vals[0] != "" {
				ctx = WithMountedEntityID(ctx, vals[0])
			}
		}
		return handler(ctx, req)
	}
}

// UserIDFromMetadataInterceptor returns a gRPC unary interceptor that reads
// user ID from incoming metadata and stores it in context with the given key.
//
// Example:
//
//	grpcServer := grpc.NewServer(
//	    grpc.ChainUnaryInterceptor(
//	        common.EntityIDFromMetadataInterceptor(),
//	        common.UserIDFromMetadataInterceptor(myUserIDKey),
//	    ),
//	)
func UserIDFromMetadataInterceptor(contextKey any) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(MetadataKeyUserID); len(vals) > 0 && vals[0] != "" {
				ctx = context.WithValue(ctx, contextKey, vals[0])
			}
		}
		return handler(ctx, req)
	}
}

// CombinedMetadataInterceptor returns a gRPC unary interceptor that reads both
// entity ID and user ID from incoming metadata.
func CombinedMetadataInterceptor(userIDContextKey any) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(MetadataKeyEntityID); len(vals) > 0 && vals[0] != "" {
				ctx = WithMountedEntityID(ctx, vals[0])
			}
			if vals := md.Get(MetadataKeyUserID); len(vals) > 0 && vals[0] != "" {
				ctx = context.WithValue(ctx, userIDContextKey, vals[0])
			}
		}
		return handler(ctx, req)
	}
}

// ============================================================================
// Helpers for checking both context and metadata
// ============================================================================

// GetEntityIDFromContextOrMetadata retrieves entity ID from context first,
// then falls back to gRPC incoming metadata. Useful for services that support
// both in-process and remote patterns.
func GetEntityIDFromContextOrMetadata(ctx context.Context) string {
	// Check context first (in-process pattern)
	if entityID := GetMountedEntityID(ctx); entityID != "" {
		return entityID
	}

	// Check gRPC metadata (remote pattern)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get(MetadataKeyEntityID); len(vals) > 0 {
			return vals[0]
		}
	}

	return ""
}
