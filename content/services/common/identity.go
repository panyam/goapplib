package common

import (
	"context"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrUnauthenticated is returned when a client call needs a logged-in user and
// has none. The gateway maps it to HTTP 401.
var ErrUnauthenticated = status.Error(codes.Unauthenticated, "login required")

// PermissionDenied returns a gRPC PermissionDenied error, which the gateway
// maps to HTTP 403.
func PermissionDenied(msg string) error {
	return status.Error(codes.PermissionDenied, msg)
}

// NotFound returns a gRPC NotFound error, which the gateway maps to HTTP 404.
// Authorizers return it for private resources so their existence doesn't leak.
func NotFound(msg string) error {
	return status.Error(codes.NotFound, msg)
}

type clientCallKey struct{}

// WithClientCall marks ctx as a request that came from a client over the
// network, as opposed to server code calling a service in-process.
//
// The content services trust the user fields of an in-process request
// (owner_id, user_id, added_by, ...), because server code sets them. They
// don't trust them on a client call: there the acting user is the one an
// interceptor or middleware put in the context, and a client call with no
// such user can't write.
//
// goapplib's own entry points mark their requests: CombinedMetadataInterceptor,
// UserIDFromMetadataInterceptor, ClientCallInterceptor, ClientCallMiddleware,
// and each service's RESTHandler and ConnectHandler. An app that wires its own
// entry point (RegisterRESTHandlers on its own mux, its own gRPC interceptor)
// must mark requests itself, or they get the in-process trust.
func WithClientCall(ctx context.Context) context.Context {
	return context.WithValue(ctx, clientCallKey{}, true)
}

// IsClientCall reports whether ctx was marked by WithClientCall.
func IsClientCall(ctx context.Context) bool {
	v, _ := ctx.Value(clientCallKey{}).(bool)
	return v
}

// ClientCallInterceptor marks every request a gRPC server receives as a
// client call. CombinedMetadataInterceptor and UserIDFromMetadataInterceptor
// already do this; use it when the user ID comes from some other interceptor.
func ClientCallInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(WithClientCall(ctx), req)
	}
}

// ClientCallMiddleware marks every HTTP request as a client call before it
// reaches an in-process gateway or Connect handler.
func ClientCallMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithClientCall(r.Context())))
	})
}

// ResolveActingUser returns the user a write acts as.
//
//   - With a user in the context, that user acts. A request naming a
//     different user gets PermissionDenied.
//   - On a client call with no user in the context, the call gets
//     ErrUnauthenticated, whatever the request names.
//   - On an in-process call with no user in the context, the request's user
//     acts, since server code set it.
func ResolveActingUser(ctx context.Context, contextUser, requestUser string) (string, error) {
	if contextUser != "" {
		if requestUser != "" && requestUser != contextUser {
			return "", PermissionDenied("request names a different user than the caller")
		}
		return contextUser, nil
	}
	if IsClientCall(ctx) {
		return "", ErrUnauthenticated
	}
	return requestUser, nil
}

// TrustedHeaderMatcher is the gateway's incoming header matcher. It forwards
// what runtime.DefaultHeaderMatcher forwards, except the metadata keys that
// carry identity: a client could otherwise send "Grpc-Metadata-X-User-Id" and
// act as any user. Only the gateway's own annotators (WithExtraMetadata,
// WithUserParamToMetadata, WithEntityParamToMetadata) set those keys.
func TrustedHeaderMatcher(key string) (string, bool) {
	canonical := textproto.CanonicalMIMEHeaderKey(key)
	if rest, ok := strings.CutPrefix(canonical, runtime.MetadataHeaderPrefix); ok {
		switch strings.ToLower(rest) {
		case MetadataKeyUserID, MetadataKeyEntityID:
			return "", false
		}
	}
	return runtime.DefaultHeaderMatcher(key)
}

// TrustedHeaderMatcherOption installs TrustedHeaderMatcher on a ServeMux.
// NewGatewayMux and NewGatewayMuxWithOptions include it; add it yourself when
// you build a mux with runtime.NewServeMux.
func TrustedHeaderMatcherOption() runtime.ServeMuxOption {
	return runtime.WithIncomingHeaderMatcher(TrustedHeaderMatcher)
}
