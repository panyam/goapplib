package likes

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	v1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"github.com/panyam/goapplib/content/gen/go/likes/v1/likesv1connect"
	"github.com/panyam/goapplib/content/services/common"
)

// RegisterRESTHandlers registers the likes service REST handlers with the given gRPC-gateway mux.
// The caller provides the mux so they can configure it with their own middleware, marshalers, etc.
//
// Example: Mount at /songs/{songId}/likes/ to handle:
//   - POST /songs/123/likes/              -> AddReaction with entity_id=123
//   - GET  /songs/123/likes/counts        -> GetLikeCounts for entity_id=123
//   - GET  /songs/123/likes/reactors      -> ListReactors for entity_id=123
//
// Usage:
//
//	mux := runtime.NewServeMux(/* your options */)
//	likesService.RegisterRESTHandlers(ctx, mux)
//	router.Handle("/songs/{songId}/likes/", common.WrapWithEntityExtraction("songId", mux))
func (s *BaseLikesService) RegisterRESTHandlers(ctx context.Context, mux *runtime.ServeMux) error {
	return v1.RegisterLikesServiceHandlerServer(ctx, mux, s)
}

// RESTHandler returns an http.Handler for REST endpoints with a default mux.
// For more control over the mux configuration, use RegisterRESTHandlers instead.
//
// Usage:
//
//	router.Handle("/songs/{songId}/likes/", likesService.RESTHandler(ctx,
//	    common.WithEntityParam("songId"),
//	))
func (s *BaseLikesService) RESTHandler(ctx context.Context, opts ...common.HandlerOption) http.Handler {
	cfg := &common.HandlerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Create default gRPC-gateway mux
	mux := runtime.NewServeMux()
	if err := s.RegisterRESTHandlers(ctx, mux); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "failed to register service handler", http.StatusInternalServerError)
		})
	}

	// Wrap with entity param extraction middleware if configured
	var handler http.Handler = mux
	if cfg.EntityParamName != "" {
		handler = common.EntityParamMiddleware(cfg.EntityParamName)(handler)
	}

	return handler
}

// ConnectHandler returns an http.Handler for Connect RPC.
// Must be mounted at a path that prefixes the Connect procedure paths.
//
// Example: Mount at /songs/{songId}/rpc/ to handle:
//   - POST /songs/123/rpc/content.likes.v1.LikesService/AddReaction
//
// Usage:
//
//	path, handler := likesService.ConnectHandler(ctx,
//	    common.WithEntityParam("songId"),
//	)
//	router.Handle("/songs/{songId}/rpc/", handler)
//
//nolint:revive // ctx reserved for future use (e.g., interceptors that need startup context)
func (s *BaseLikesService) ConnectHandler(ctx context.Context, opts ...common.HandlerOption) (string, http.Handler) {
	_ = ctx // Reserved for future use
	cfg := &common.HandlerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Create Connect handler using the adapter
	adapter := NewConnectLikesServer(s)
	path, handler := likesv1connect.NewLikesServiceHandler(adapter)

	// Wrap with entity param extraction middleware if configured
	if cfg.EntityParamName != "" {
		handler = common.EntityParamMiddleware(cfg.EntityParamName)(handler)
	}

	return path, handler
}

// ConnectHandlerWithOptions returns an http.Handler for Connect RPC with additional connect options.
//
//nolint:revive // ctx reserved for future use
func (s *BaseLikesService) ConnectHandlerWithOptions(ctx context.Context, connectOpts []connect.HandlerOption, mountOpts ...common.HandlerOption) (string, http.Handler) {
	_ = ctx // Reserved for future use
	cfg := &common.HandlerConfig{}
	for _, opt := range mountOpts {
		opt(cfg)
	}

	// Create Connect handler with options
	adapter := NewConnectLikesServer(s)
	path, handler := likesv1connect.NewLikesServiceHandler(adapter, connectOpts...)

	// Wrap with entity param extraction middleware if configured
	if cfg.EntityParamName != "" {
		handler = common.EntityParamMiddleware(cfg.EntityParamName)(handler)
	}

	return path, handler
}
