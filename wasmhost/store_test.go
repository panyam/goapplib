package wasmhost

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// testStore is what every Store must do, run against each implementation.
func testStore(t *testing.T, s Store) {
	ctx := context.Background()
	key := Key([]byte("v1"), []byte("design.edif"))

	if _, err := s.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get of a missing key: %v, want ErrNotFound", err)
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
		t.Fatalf("changing a Get's result changed the store: %q", again)
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
		if _, err := s.Get(ctx, bad); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("Get of key %q: %v, want a key error", bad, err)
		}
	}
}

func TestMemStore(t *testing.T) {
	testStore(t, &MemStore{})
}

func TestKey(t *testing.T) {
	if Key([]byte("ab"), []byte("c")) == Key([]byte("a"), []byte("bc")) {
		t.Fatal(`("ab","c") and ("a","bc") got the same key`)
	}
	if a, b := Key([]byte("x")), Key([]byte("x")); a != b || len(a) != 64 {
		t.Fatalf("Key isn't a stable 64-character hex string: %q %q", a, b)
	}
	if err := checkKey(Key([]byte("x"))); err != nil {
		t.Fatalf("Key made a key the stores reject: %v", err)
	}
}
