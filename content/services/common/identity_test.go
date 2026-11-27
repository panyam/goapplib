package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	likesv1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mdRecorder is a likes server whose GetLikeCounts records the incoming
// metadata the gateway built.
type mdRecorder struct {
	likesv1.UnimplementedLikesServiceServer
	md metadata.MD
}

func (r *mdRecorder) GetLikeCounts(ctx context.Context, _ *likesv1.GetLikeCountsRequest) (*likesv1.GetLikeCountsResponse, error) {
	r.md, _ = metadata.FromIncomingContext(ctx)
	return &likesv1.GetLikeCountsResponse{}, nil
}

func spoofedCountsRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/likes/counts", nil)
	req.Header.Set("Grpc-Metadata-X-User-Id", "user:victim")
	req.Header.Set("grpc-metadata-x-entity-id", "notation:spoofed")
	req.Header.Set("Grpc-Metadata-X-Trace", "kept")
	return req
}

func TestGatewayMux_DropsClientIdentityHeaders(t *testing.T) {
	asAlice := WithExtraMetadata(func(ctx context.Context, r *http.Request) metadata.MD {
		return metadata.Pairs(MetadataKeyUserID, "user:alice")
	})
	anonymous := WithExtraMetadata(func(ctx context.Context, r *http.Request) metadata.MD {
		return metadata.MD{}
	})
	cases := []struct {
		name     string
		mux      *runtime.ServeMux
		wantUser []string
	}{
		{"NewGatewayMux logged in", NewGatewayMux(asAlice), []string{"user:alice"}},
		{"NewGatewayMux anonymous", NewGatewayMux(anonymous), nil},
		{"NewGatewayMuxWithOptions logged in", NewGatewayMuxWithOptions([]GatewayMuxOption{asAlice}), []string{"user:alice"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &mdRecorder{}
			if err := likesv1.RegisterLikesServiceHandlerServer(context.Background(), tc.mux, rec); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			tc.mux.ServeHTTP(w, spoofedCountsRequest())
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if got := rec.md.Get(MetadataKeyUserID); !equal(got, tc.wantUser) {
				t.Errorf("x-user-id = %q, want %q", got, tc.wantUser)
			}
			if got := rec.md.Get(MetadataKeyEntityID); len(got) != 0 {
				t.Errorf("x-entity-id = %q, want none", got)
			}
			if got := rec.md.Get("x-trace"); !equal(got, []string{"kept"}) {
				t.Errorf("x-trace = %q, want other Grpc-Metadata headers kept", got)
			}
		})
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestResolveActingUser(t *testing.T) {
	inProcess := context.Background()
	client := WithClientCall(context.Background())
	cases := []struct {
		name               string
		ctx                context.Context
		contextUser, reqID string
		want               string
		wantCode           codes.Code
	}{
		{"context user, empty request", client, "user:alice", "", "user:alice", codes.OK},
		{"context user, same request", client, "user:alice", "user:alice", "user:alice", codes.OK},
		{"context user, other request", client, "user:alice", "user:victim", "", codes.PermissionDenied},
		{"in-process context user, other request", inProcess, "user:alice", "user:victim", "", codes.PermissionDenied},
		{"client call, no user", client, "", "", "", codes.Unauthenticated},
		{"client call, no user, request names one", client, "", "user:victim", "", codes.Unauthenticated},
		{"in-process, no user, request names one", inProcess, "", "user:server-set", "user:server-set", codes.OK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveActingUser(tc.ctx, tc.contextUser, tc.reqID)
			if status.Code(err) != tc.wantCode {
				t.Fatalf("code = %v (%v), want %v", status.Code(err), err, tc.wantCode)
			}
			if got != tc.want {
				t.Errorf("user = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMetadataInterceptors_MarkClientCall(t *testing.T) {
	for name, ic := range map[string]grpc.UnaryServerInterceptor{
		"Combined":   CombinedMetadataInterceptor(DefaultUserIDContextKey),
		"UserID":     UserIDFromMetadataInterceptor(DefaultUserIDContextKey),
		"ClientCall": ClientCallInterceptor(),
	} {
		t.Run(name, func(t *testing.T) {
			var marked bool
			_, _ = ic(context.Background(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
				marked = IsClientCall(ctx)
				return nil, nil
			})
			if !marked {
				t.Error("handler ctx not marked as a client call")
			}
		})
	}
	if IsClientCall(context.Background()) {
		t.Error("a bare context reads as a client call")
	}
}
