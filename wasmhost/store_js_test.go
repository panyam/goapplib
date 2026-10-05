//go:build js && wasm

package wasmhost

import (
	"context"
	"errors"
	"strings"
	"syscall/js"
	"testing"
	"time"
)

// fakeStore is a JS object shaped like the one tsappkit's worker installs: get and put return
// Promises, and get resolves to null for a missing key.
func fakeStore() js.Value {
	return js.Global().Get("Function").New(`
		const m = new Map();
		return {
			get: (k) => new Promise((ok) => setTimeout(() => ok(m.has(k) ? m.get(k) : null), 1)),
			put: (k, b) => new Promise((ok) => setTimeout(() => { m.set(k, b.slice()); ok(); }, 1)),
		};`).Invoke()
}

func TestBrowserStoreWaitsOnTheWorkersPromises(t *testing.T) {
	js.Global().Set("wasmhostStore", fakeStore())
	defer js.Global().Delete("wasmhostStore")
	testStore(t, BrowserStore())
}

func TestBrowserStoreWithoutAStoreIsErrNoStore(t *testing.T) {
	js.Global().Delete("wasmhostStore")
	s := BrowserStore()
	if _, err := s.Get(context.Background(), "k"); !errors.Is(err, ErrNoStore) {
		t.Fatalf("Get: %v", err)
	}
	if err := s.Put(context.Background(), "k", []byte("x")); !errors.Is(err, ErrNoStore) {
		t.Fatalf("Put: %v", err)
	}
}

func TestBrowserStoreReportsRejectionsAndStopsWaitingWhenCancelled(t *testing.T) {
	js.Global().Set("wasmhostStore", js.Global().Get("Function").New(`
		return {
			get: () => Promise.reject(new Error("quota exceeded")),
			put: () => new Promise(() => {}),
		};`).Invoke())
	defer js.Global().Delete("wasmhostStore")
	s := BrowserStore()
	if _, err := s.Get(context.Background(), "k"); err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("Get of a rejecting store: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.Put(ctx, "k", []byte("x")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Put that never settles: %v, want the context's error", err)
	}
}
