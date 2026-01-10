package goapplib

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	if rl == nil {
		t.Fatal("expected non-nil rate limiter")
	}
	if rl.limit != 10 {
		t.Errorf("expected limit 10, got %d", rl.limit)
	}
	if rl.window != time.Minute {
		t.Errorf("expected window 1m, got %v", rl.window)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		if !rl.Allow("client1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	if rl.Allow("client1") {
		t.Error("4th request should be denied")
	}

	// Different client should still be allowed
	if !rl.Allow("client2") {
		t.Error("different client should be allowed")
	}
}

func TestRateLimiter_SlidingWindow(t *testing.T) {
	// Use a very short window for testing
	rl := NewRateLimiter(2, 100*time.Millisecond)

	// Make 2 requests
	rl.Allow("client1")
	rl.Allow("client1")

	// 3rd should be denied
	if rl.Allow("client1") {
		t.Error("3rd request should be denied")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Now should be allowed again
	if !rl.Allow("client1") {
		t.Error("request after window expiry should be allowed")
	}
}

func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()
	if config.AuthLimit != 10 {
		t.Errorf("expected AuthLimit 10, got %d", config.AuthLimit)
	}
	if config.AuthWindow != 15*time.Minute {
		t.Errorf("expected AuthWindow 15m, got %v", config.AuthWindow)
	}
	if config.APILimit != 100 {
		t.Errorf("expected APILimit 100, got %d", config.APILimit)
	}
	if config.APIWindow != time.Minute {
		t.Errorf("expected APIWindow 1m, got %v", config.APIWindow)
	}
}

func TestNewRateLimitMiddleware(t *testing.T) {
	m := NewRateLimitMiddleware(nil)
	if m == nil {
		t.Fatal("expected non-nil middleware")
	}
	if m.authLimiter == nil {
		t.Error("expected authLimiter to be initialized")
	}
	if m.apiLimiter == nil {
		t.Error("expected apiLimiter to be initialized")
	}
	if m.keyFunc == nil {
		t.Error("expected keyFunc to be initialized")
	}
}

func TestDefaultRateLimitKeyFunc(t *testing.T) {
	// Test with X-Forwarded-For
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	key := DefaultRateLimitKeyFunc(req)
	if key != "192.168.1.1" {
		t.Errorf("expected key '192.168.1.1', got %q", key)
	}

	// Test without X-Forwarded-For
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	key2 := DefaultRateLimitKeyFunc(req2)
	if key2 != "10.0.0.1:1234" {
		t.Errorf("expected key '10.0.0.1:1234', got %q", key2)
	}
}

func TestRateLimitMiddleware_WrapAuth(t *testing.T) {
	config := &RateLimitConfig{
		AuthLimit:  2,
		AuthWindow: time.Minute,
		APILimit:   100,
		APIWindow:  time.Minute,
	}
	m := NewRateLimitMiddleware(config)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := m.WrapAuth(handler)

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
}

func TestRateLimitMiddleware_WrapAPI(t *testing.T) {
	config := &RateLimitConfig{
		AuthLimit:  10,
		AuthWindow: time.Minute,
		APILimit:   2,
		APIWindow:  time.Minute,
	}
	m := NewRateLimitMiddleware(config)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := m.WrapAPI(handler)

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
}

func TestRateLimitMiddleware_CustomKeyFunc(t *testing.T) {
	config := &RateLimitConfig{
		AuthLimit:  1,
		AuthWindow: time.Minute,
		APILimit:   1,
		APIWindow:  time.Minute,
	}

	// Use user ID as key instead of IP
	keyFunc := func(r *http.Request) string {
		return r.Header.Get("X-User-ID")
	}

	m := NewRateLimitMiddlewareWithKeyFunc(config, keyFunc)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := m.WrapAPI(handler)

	// User 1's first request
	req1 := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req1.Header.Set("X-User-ID", "user1")
	rr1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("user1 first request: expected 200, got %d", rr1.Code)
	}

	// User 1's second request should be denied
	req2 := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req2.Header.Set("X-User-ID", "user1")
	rr2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("user1 second request: expected 429, got %d", rr2.Code)
	}

	// User 2's first request should be allowed
	req3 := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req3.Header.Set("X-User-ID", "user2")
	rr3 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("user2 first request: expected 200, got %d", rr3.Code)
	}
}

func TestRateLimitMiddleware_Wrap(t *testing.T) {
	config := &RateLimitConfig{
		AuthLimit:  10,
		AuthWindow: time.Minute,
		APILimit:   1,
		APIWindow:  time.Minute,
	}
	m := NewRateLimitMiddleware(config)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap uses API limiter
	wrapped := m.Wrap(handler)

	req := httptest.NewRequest(http.MethodGet, "/something", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Second request should be denied (API limit is 1)
	rr2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr2.Code)
	}
}
