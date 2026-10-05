package wasmhost

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// testCache is what every Cache must do, run against each implementation.
func testCache(t *testing.T, s Cache) {
	ctx := context.Background()
	key := CacheKey([]byte("v1"), []byte("design.edif"))

	if _, err := s.Get(ctx, key); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get of a missing key: %v, want ErrMiss", err)
	}

	in := []byte("parsed design")
	if err := s.Put(ctx, key, in); err != nil {
		t.Fatal(err)
	}
	in[0] = 'X'
	got, err := s.Get(ctx, key)
	if err != nil || string(got) != "parsed design" {
		t.Fatalf("Get after Put: %q, %v; want the bytes as they were put", got, err)
	}
	got[0] = 'Y'
	if again, _ := s.Get(ctx, key); string(again) != "parsed design" {
		t.Fatalf("changing a Get's result changed the cache: %q", again)
	}

	if err := s.Put(ctx, key, []byte("rebuilt")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, key); string(got) != "rebuilt" {
		t.Fatalf("Get after a second Put: %q", got)
	}

	big := bytes.Repeat([]byte{7}, 3<<20)
	if err := s.Put(ctx, "big", big); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, "big"); !bytes.Equal(got, big) {
		t.Fatalf("a 3 MB blob came back as %d bytes", len(got))
	}

	for _, bad := range []string{"", ".", "..", "../x", "a/b", "UPPER", ".hidden", string(make([]byte, 129))} {
		if err := s.Put(ctx, bad, nil); err == nil {
			t.Errorf("Put accepted key %q", bad)
		}
		if _, err := s.Get(ctx, bad); err == nil || errors.Is(err, ErrMiss) {
			t.Errorf("Get of key %q: %v, want a key error", bad, err)
		}
	}
}

func TestMemCache(t *testing.T) {
	testCache(t, &MemCache{})
}

func TestCacheKey(t *testing.T) {
	if CacheKey([]byte("ab"), []byte("c")) == CacheKey([]byte("a"), []byte("bc")) {
		t.Fatal(`("ab","c") and ("a","bc") got the same key`)
	}
	if a, b := CacheKey([]byte("x")), CacheKey([]byte("x")); a != b || len(a) != 64 {
		t.Fatalf("Key isn't a stable 64-character hex string: %q %q", a, b)
	}
	if err := checkKey(CacheKey([]byte("x"))); err != nil {
		t.Fatalf("CacheKey made a key the caches reject: %v", err)
	}
}
