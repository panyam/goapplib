package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestMountedEntityContext(t *testing.T) {
	t.Run("WithMountedEntityID stores value in context", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithMountedEntityID(ctx, "song:123")

		got := GetMountedEntityID(ctx)
		if got != "song:123" {
			t.Errorf("GetMountedEntityID() = %q, want %q", got, "song:123")
		}
	})

	t.Run("GetMountedEntityID returns empty string for missing value", func(t *testing.T) {
		ctx := context.Background()

		got := GetMountedEntityID(ctx)
		if got != "" {
			t.Errorf("GetMountedEntityID() = %q, want empty string", got)
		}
	})

	t.Run("WithMountedEntityID overwrites previous value", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithMountedEntityID(ctx, "song:123")
		ctx = WithMountedEntityID(ctx, "song:456")

		got := GetMountedEntityID(ctx)
		if got != "song:456" {
			t.Errorf("GetMountedEntityID() = %q, want %q", got, "song:456")
		}
	})
}

func TestEntityParamMiddleware(t *testing.T) {
	t.Run("extracts path param and stores in context", func(t *testing.T) {
		var capturedEntityID string

		// Inner handler captures the entity ID from context
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedEntityID = GetMountedEntityID(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		// Wrap with middleware
		handler := EntityParamMiddleware("songId")(inner)

		// Create request with path value (Go 1.22+ style)
		req := httptest.NewRequest("GET", "/songs/song123/likes", nil)
		req.SetPathValue("songId", "song123")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if capturedEntityID != "song123" {
			t.Errorf("captured entity ID = %q, want %q", capturedEntityID, "song123")
		}
	})

	t.Run("passes through when param not found", func(t *testing.T) {
		var capturedEntityID string

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedEntityID = GetMountedEntityID(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		handler := EntityParamMiddleware("songId")(inner)

		// Request without path value
		req := httptest.NewRequest("GET", "/songs/likes", nil)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if capturedEntityID != "" {
			t.Errorf("captured entity ID = %q, want empty string", capturedEntityID)
		}
	})
}

func TestWrapWithEntityExtraction(t *testing.T) {
	var capturedEntityID string

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedEntityID = GetMountedEntityID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := WrapWithEntityExtraction("songId", inner)

	req := httptest.NewRequest("GET", "/songs/song456/likes", nil)
	req.SetPathValue("songId", "song456")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedEntityID != "song456" {
		t.Errorf("captured entity ID = %q, want %q", capturedEntityID, "song456")
	}
}

func TestExtractPathParam(t *testing.T) {
	t.Run("extracts Go 1.22+ PathValue", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/songs/song789/likes", nil)
		req.SetPathValue("songId", "song789")

		got := ExtractPathParam(req, "songId")
		if got != "song789" {
			t.Errorf("ExtractPathParam() = %q, want %q", got, "song789")
		}
	})

	t.Run("returns empty for missing param", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/songs/likes", nil)

		got := ExtractPathParam(req, "songId")
		if got != "" {
			t.Errorf("ExtractPathParam() = %q, want empty string", got)
		}
	})
}

func TestGatewayMuxConfig(t *testing.T) {
	t.Run("WithEntityParamToMetadata sets entity param name", func(t *testing.T) {
		cfg := &gatewayMuxConfig{}
		WithEntityParamToMetadata("songId")(cfg)

		if cfg.entityParamName != "songId" {
			t.Errorf("entityParamName = %q, want %q", cfg.entityParamName, "songId")
		}
	})

	t.Run("WithUserParamToMetadata sets user param name", func(t *testing.T) {
		cfg := &gatewayMuxConfig{}
		WithUserParamToMetadata("userId")(cfg)

		if cfg.userParamName != "userId" {
			t.Errorf("userParamName = %q, want %q", cfg.userParamName, "userId")
		}
	})

	t.Run("WithExtraMetadata sets custom function", func(t *testing.T) {
		cfg := &gatewayMuxConfig{}
		fn := func(ctx context.Context, r *http.Request) metadata.MD {
			return metadata.Pairs("x-test", "value")
		}
		WithExtraMetadata(fn)(cfg)

		if cfg.extraMetadata == nil {
			t.Error("extraMetadata should not be nil")
		}
	})
}

func TestEntityIDFromMetadataInterceptor(t *testing.T) {
	interceptor := EntityIDFromMetadataInterceptor()

	t.Run("extracts entity ID from metadata", func(t *testing.T) {
		// Create context with incoming metadata
		md := metadata.Pairs(MetadataKeyEntityID, "song:123")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		var capturedCtx context.Context
		handler := func(ctx context.Context, req any) (any, error) {
			capturedCtx = ctx
			return "ok", nil
		}

		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("interceptor returned error: %v", err)
		}

		entityID := GetMountedEntityID(capturedCtx)
		if entityID != "song:123" {
			t.Errorf("entity ID = %q, want %q", entityID, "song:123")
		}
	})

	t.Run("passes through when no metadata", func(t *testing.T) {
		ctx := context.Background()

		var capturedCtx context.Context
		handler := func(ctx context.Context, req any) (any, error) {
			capturedCtx = ctx
			return "ok", nil
		}

		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("interceptor returned error: %v", err)
		}

		entityID := GetMountedEntityID(capturedCtx)
		if entityID != "" {
			t.Errorf("entity ID = %q, want empty string", entityID)
		}
	})

	t.Run("ignores empty entity ID in metadata", func(t *testing.T) {
		md := metadata.Pairs(MetadataKeyEntityID, "")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		var capturedCtx context.Context
		handler := func(ctx context.Context, req any) (any, error) {
			capturedCtx = ctx
			return "ok", nil
		}

		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("interceptor returned error: %v", err)
		}

		entityID := GetMountedEntityID(capturedCtx)
		if entityID != "" {
			t.Errorf("entity ID = %q, want empty string", entityID)
		}
	})
}

func TestUserIDFromMetadataInterceptor(t *testing.T) {
	type userIDKey struct{}
	interceptor := UserIDFromMetadataInterceptor(userIDKey{})

	t.Run("extracts user ID from metadata", func(t *testing.T) {
		md := metadata.Pairs(MetadataKeyUserID, "user:456")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		var capturedCtx context.Context
		handler := func(ctx context.Context, req any) (any, error) {
			capturedCtx = ctx
			return "ok", nil
		}

		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("interceptor returned error: %v", err)
		}

		userID := capturedCtx.Value(userIDKey{})
		if userID != "user:456" {
			t.Errorf("user ID = %q, want %q", userID, "user:456")
		}
	})
}

func TestCombinedMetadataInterceptor(t *testing.T) {
	type userIDKey struct{}
	interceptor := CombinedMetadataInterceptor(userIDKey{})

	t.Run("extracts both entity ID and user ID", func(t *testing.T) {
		md := metadata.Pairs(
			MetadataKeyEntityID, "song:123",
			MetadataKeyUserID, "user:456",
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		var capturedCtx context.Context
		handler := func(ctx context.Context, req any) (any, error) {
			capturedCtx = ctx
			return "ok", nil
		}

		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("interceptor returned error: %v", err)
		}

		entityID := GetMountedEntityID(capturedCtx)
		if entityID != "song:123" {
			t.Errorf("entity ID = %q, want %q", entityID, "song:123")
		}

		userID := capturedCtx.Value(userIDKey{})
		if userID != "user:456" {
			t.Errorf("user ID = %q, want %q", userID, "user:456")
		}
	})
}

func TestGetEntityIDFromContextOrMetadata(t *testing.T) {
	t.Run("prefers context over metadata", func(t *testing.T) {
		// Set both context and metadata
		ctx := WithMountedEntityID(context.Background(), "from-context")
		md := metadata.Pairs(MetadataKeyEntityID, "from-metadata")
		ctx = metadata.NewIncomingContext(ctx, md)

		got := GetEntityIDFromContextOrMetadata(ctx)
		if got != "from-context" {
			t.Errorf("got %q, want %q (should prefer context)", got, "from-context")
		}
	})

	t.Run("falls back to metadata when context empty", func(t *testing.T) {
		md := metadata.Pairs(MetadataKeyEntityID, "from-metadata")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		got := GetEntityIDFromContextOrMetadata(ctx)
		if got != "from-metadata" {
			t.Errorf("got %q, want %q", got, "from-metadata")
		}
	})

	t.Run("returns empty when neither set", func(t *testing.T) {
		ctx := context.Background()

		got := GetEntityIDFromContextOrMetadata(ctx)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestMetadataKeys(t *testing.T) {
	if MetadataKeyEntityID != "x-entity-id" {
		t.Errorf("MetadataKeyEntityID = %q, want %q", MetadataKeyEntityID, "x-entity-id")
	}
	if MetadataKeyUserID != "x-user-id" {
		t.Errorf("MetadataKeyUserID = %q, want %q", MetadataKeyUserID, "x-user-id")
	}
}
