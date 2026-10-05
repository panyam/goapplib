package wasmhost

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sync"
)

// Cache keeps state a service builds from its inputs and would rather not build again, such as a
// parsed design, a model or an index, so a reload or a restarted worker can skip the rebuild. Keys
// name the inputs (see CacheKey), not the blob, so a service computes the key from a request
// before it has anything cached, and the same inputs always find the same blob. In the browser it's
// BrowserCache, over the Origin Private File System, which outlives a reload; natively it's
// DirCache or MemCache.
//
// It's a cache, not a datastore: anything in it must be rebuildable from its inputs, because a
// miss, ErrNoCache, a browser clearing the origin's storage, or any other error just means the
// service builds it again. Data a user would lose belongs in the app's server-side storage.
//
// Get and Put may wait on the browser, so call them from a handler or another goroutine, never from
// inside a js.FuncOf callback (see the package doc). Implementations are safe for concurrent use.
type Cache interface {
	// Get returns a copy of the blob under key, or ErrMiss. A key that isn't one CacheKey could
	// make is an error rather than a miss.
	Get(ctx context.Context, key string) ([]byte, error)
	// Put stores b under key, replacing what was there. A reader never sees half a blob: it gets
	// the old one, the new one, or ErrMiss. The cache keeps its own copy of b.
	Put(ctx context.Context, key string, b []byte) error
}

// ErrMiss is what Get returns for a key the cache has nothing under.
var ErrMiss = errors.New("wasmhost: not in the cache")

// ErrNoCache is what BrowserCache's methods return when the browser has no storage to offer the
// worker (no Origin Private File System, or a worker started without one). A service treats it as
// a miss and rebuilds.
var ErrNoCache = errors.New("wasmhost: no cache in this browser")

var keyPattern = regexp.MustCompile(`^[a-z0-9_-][a-z0-9._-]{0,127}$`)

func checkKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("wasmhost: cache key %q isn't 1 to 128 of a-z, 0-9, '.', '_' and '-' (not starting with '.')", key)
	}
	return nil
}

// CacheKey is a cache key for inputs: the hex SHA-256 of the parts, each prefixed with its length, so
// ("ab", "c") and ("a", "bc") get different keys. Put a version string first, so a service that
// changes what it stores stops finding blobs an older build wrote.
func CacheKey(parts ...[]byte) string {
	h := sha256.New()
	var n [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(n[:], uint64(len(p)))
		h.Write(n[:])
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// MemCache is a Cache in memory, for tests and for a service with nowhere to persist. It forgets
// everything when the process ends.
type MemCache struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

// Get implements Cache.
func (m *MemCache) Get(_ context.Context, key string) ([]byte, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blobs[key]
	if !ok {
		return nil, ErrMiss
	}
	return append([]byte(nil), b...), nil
}

// Put implements Cache.
func (m *MemCache) Put(_ context.Context, key string, b []byte) error {
	if err := checkKey(key); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.blobs == nil {
		m.blobs = map[string][]byte{}
	}
	m.blobs[key] = append([]byte(nil), b...)
	return nil
}
