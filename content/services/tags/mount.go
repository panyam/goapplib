package tags

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	v1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	"github.com/panyam/goapplib/content/gen/go/tags/v1/tagsv1connect"
	"github.com/panyam/goapplib/content/services/common"
)

// RegisterRESTHandlers registers the tags service REST handlers with the given gRPC-gateway mux.
// The caller provides the mux so they can configure it with their own middleware, marshalers, etc.
//
// Example: Mount at /songs/{songId}/tags/ to handle:
//   - POST /songs/123/tags/entity     -> TagEntity with entity_id=123
//   - GET  /songs/123/tags/entity/123 -> GetEntityTags for entity_id=123
//
// Usage:
//
//	mux := runtime.NewServeMux(/* your options */)
//	tagsService.RegisterRESTHandlers(ctx, mux)
//	router.Handle("/songs/{songId}/tags/", common.WrapWithEntityExtraction("songId", mux))
func (s *BaseTagsService) RegisterRESTHandlers(ctx context.Context, mux *runtime.ServeMux) error {
	return v1.RegisterTagsServiceHandlerServer(ctx, mux, s)
}

// RESTHandler returns an http.Handler for REST endpoints with a default mux.
// For more control over the mux configuration, use RegisterRESTHandlers instead.
//
// Usage:
//
//	router.Handle("/songs/{songId}/tags/", tagsService.RESTHandler(ctx,
//	    common.WithEntityParam("songId"),
//	))
func (s *BaseTagsService) RESTHandler(ctx context.Context, opts ...common.HandlerOption) http.Handler {
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
//   - POST /songs/123/rpc/content.tags.v1.TagsService/TagEntity
//
// Usage:
//
//	path, handler := tagsService.ConnectHandler(ctx,
//	    common.WithEntityParam("songId"),
//	)
//	router.Handle("/songs/{songId}/rpc/", handler)
//
//nolint:revive // ctx reserved for future use (e.g., interceptors that need startup context)
func (s *BaseTagsService) ConnectHandler(ctx context.Context, opts ...common.HandlerOption) (string, http.Handler) {
	_ = ctx // Reserved for future use
	cfg := &common.HandlerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Create Connect handler using the adapter
	adapter := NewConnectTagsServer(s)
	path, handler := tagsv1connect.NewTagsServiceHandler(adapter)

	// Wrap with entity param extraction middleware if configured
	if cfg.EntityParamName != "" {
		handler = common.EntityParamMiddleware(cfg.EntityParamName)(handler)
	}

	return path, handler
}

// ConnectHandlerWithOptions returns an http.Handler for Connect RPC with additional connect options.
//
//nolint:revive // ctx reserved for future use
func (s *BaseTagsService) ConnectHandlerWithOptions(ctx context.Context, connectOpts []connect.HandlerOption, mountOpts ...common.HandlerOption) (string, http.Handler) {
	_ = ctx // Reserved for future use
	cfg := &common.HandlerConfig{}
	for _, opt := range mountOpts {
		opt(cfg)
	}

	// Create Connect handler with options
	adapter := NewConnectTagsServer(s)
	path, handler := tagsv1connect.NewTagsServiceHandler(adapter, connectOpts...)

	// Wrap with entity param extraction middleware if configured
	if cfg.EntityParamName != "" {
		handler = common.EntityParamMiddleware(cfg.EntityParamName)(handler)
	}

	return path, handler
}
