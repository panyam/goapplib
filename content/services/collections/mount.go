package collections

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	v1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	"github.com/panyam/goapplib/content/gen/go/collections/v1/collectionsv1connect"
)

// RegisterRESTHandlers registers the collections service REST handlers with the given gRPC-gateway mux.
// The caller provides the mux so they can configure it with their own middleware, marshalers, etc.
//
// Collections can be mounted at arbitrary paths:
//   - /collections/ - global collections
//   - /users/{userId}/collections/ - user's collections (caller handles userId extraction)
//   - /songs/{songId}/collections/ - collections containing a song (caller maps externally)
//
// Usage:
//
//	mux := runtime.NewServeMux(/* your options */)
//	collectionsService.RegisterRESTHandlers(ctx, mux)
//	router.Handle("/users/{userId}/collections/", mux)
func (s *BaseCollectionsService) RegisterRESTHandlers(ctx context.Context, mux *runtime.ServeMux) error {
	return v1.RegisterCollectionsServiceHandlerServer(ctx, mux, s)
}

// RESTHandler returns an http.Handler for REST endpoints with a default mux.
// For more control over the mux configuration, use RegisterRESTHandlers instead.
//
// Usage:
//
//	router.Handle("/collections/", collectionsService.RESTHandler(ctx))
//	// Or mount under a user path (caller extracts userId separately)
//	router.Handle("/users/{userId}/collections/", collectionsService.RESTHandler(ctx))
func (s *BaseCollectionsService) RESTHandler(ctx context.Context) http.Handler {
	mux := runtime.NewServeMux()
	if err := s.RegisterRESTHandlers(ctx, mux); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "failed to register service handler", http.StatusInternalServerError)
		})
	}
	return mux
}

// ConnectHandler returns an http.Handler for Connect RPC.
//
// Usage:
//
//	path, handler := collectionsService.ConnectHandler(ctx)
//	router.Handle(path, handler)
//
//nolint:revive // ctx reserved for future use
func (s *BaseCollectionsService) ConnectHandler(ctx context.Context) (string, http.Handler) {
	_ = ctx // Reserved for future use
	adapter := NewConnectCollectionsServer(s)
	return collectionsv1connect.NewCollectionsServiceHandler(adapter)
}

// ConnectHandlerWithOptions returns an http.Handler for Connect RPC with additional connect options.
//
//nolint:revive // ctx reserved for future use
func (s *BaseCollectionsService) ConnectHandlerWithOptions(ctx context.Context, opts ...connect.HandlerOption) (string, http.Handler) {
	_ = ctx // Reserved for future use
	adapter := NewConnectCollectionsServer(s)
	return collectionsv1connect.NewCollectionsServiceHandler(adapter, opts...)
}
