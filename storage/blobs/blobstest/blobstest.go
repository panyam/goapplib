// Package blobstest checks that a blobs.Store keeps the contract callers rely on. A store's test
// calls RunStore with a factory for a fresh, empty store, the way mountstest checks record stores
// and drivers.
package blobstest

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
)

// Fixture is one fresh, empty store and the two things RunStore can't do through the Store
// interface.
type Fixture struct {
	Store blobs.Store

	// Later makes every blob the store holds now look written by longer ago, as if by had
	// passed. Blobs put after the call are new. A store with a clock option can move its clock;
	// one on disk can set each file's mtime back.
	Later func(t *testing.T, by time.Duration)

	// Corrupt replaces the stored bytes of h with b, behind the store's back.
	Corrupt func(t *testing.T, h blobs.Hash, b []byte)
}

// Factory returns a new Fixture over an empty store.
type Factory func(t *testing.T) Fixture

// RunStore runs every contract check against stores from newFixture.
func RunStore(t *testing.T, newFixture Factory) {
	ctx := context.Background()

	t.Run("PutOpenRoundTrip", func(t *testing.T) {
		s := newFixture(t).Store
		for _, data := range [][]byte{[]byte("hello"), {}, random(t, 3<<20)} {
			h := put(t, s, data)
			if h != blobs.HashOf(data) {
				t.Fatalf("Put of %d bytes = %s, want %s", len(data), h, blobs.HashOf(data))
			}
			if got, err := read(s, h); err != nil || !bytes.Equal(got, data) {
				t.Fatalf("Open of %d bytes read %d bytes, %v", len(data), len(got), err)
			}
			if ok, err := s.Has(ctx, h); !ok || err != nil {
				t.Errorf("Has after Put = %v, %v", ok, err)
			}
		}
	})

	t.Run("MissingIsNotFound", func(t *testing.T) {
		s := newFixture(t).Store
		h := blobs.HashOf([]byte("never stored"))
		if _, err := read(s, h); !errors.Is(err, blobs.ErrNotFound) {
			t.Errorf("Open of a missing blob = %v, want ErrNotFound", err)
		}
		if ok, err := s.Has(ctx, h); ok || err != nil {
			t.Errorf("Has of a missing blob = %v, %v; want false, nil", ok, err)
		}
	})

	t.Run("MalformedHashIsInvalid", func(t *testing.T) {
		s := newFixture(t).Store
		for _, h := range []blobs.Hash{"", "sha256:", "sha256:../../etc/passwd", "md5:" + blobs.Hash(strings.Repeat("a", 64)), "sha256:" + blobs.Hash(strings.Repeat("A", 64))} {
			if _, err := read(s, h); !errors.Is(err, blobs.ErrInvalidHash) {
				t.Errorf("Open(%q) = %v, want ErrInvalidHash", h, err)
			}
			if _, err := s.Has(ctx, h); !errors.Is(err, blobs.ErrInvalidHash) {
				t.Errorf("Has(%q) = %v, want ErrInvalidHash", h, err)
			}
		}
	})

	t.Run("OpenReportsCorruption", func(t *testing.T) {
		f := newFixture(t)
		h := put(t, f.Store, []byte("original bytes"))
		f.Corrupt(t, h, []byte("changed bytes!"))
		if _, err := read(f.Store, h); !errors.Is(err, blobs.ErrCorrupt) {
			t.Errorf("reading a changed blob = %v, want ErrCorrupt", err)
		}
	})

	t.Run("PutTwiceRefreshesTheGracePeriod", func(t *testing.T) {
		f := newFixture(t)
		h := put(t, f.Store, []byte("same"))
		f.Later(t, time.Hour)
		if put(t, f.Store, []byte("same")) != h {
			t.Fatal("the same contents got a different hash")
		}
		if n, err := f.Store.Sweep(ctx, 10*time.Minute); err != nil || n != 0 {
			t.Errorf("Sweep right after a second Put removed %d, %v; want 0, so the caller can still name it", n, err)
		}
		if ok, _ := f.Store.Has(ctx, h); !ok {
			t.Error("the blob is gone after a second Put and a sweep")
		}
	})

	t.Run("SweepKeepsReferencedAndRecent", func(t *testing.T) {
		f := newFixture(t)
		a, b, c := put(t, f.Store, []byte("a")), put(t, f.Store, []byte("b")), put(t, f.Store, []byte("c"))
		f.Later(t, time.Hour)
		recent := put(t, f.Store, []byte("recent"))
		n, err := f.Store.Sweep(ctx, 10*time.Minute, keep(a), keep(b))
		if err != nil || n != 1 {
			t.Fatalf("Sweep = %d, %v; want 1 (c)", n, err)
		}
		for h, want := range map[blobs.Hash]bool{a: true, b: true, c: false, recent: true} {
			if ok, _ := f.Store.Has(ctx, h); ok != want {
				t.Errorf("after Sweep, Has(%s) = %v, want %v", h, ok, want)
			}
		}
	})

	t.Run("SweepStopsOnReferrerError", func(t *testing.T) {
		f := newFixture(t)
		h := put(t, f.Store, []byte("x"))
		f.Later(t, time.Hour)
		boom := errors.New("store unreachable")
		if _, err := f.Store.Sweep(ctx, 0, func(context.Context, func(blobs.Hash)) error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("Sweep = %v, want the referrer's error", err)
		}
		if ok, _ := f.Store.Has(ctx, h); !ok {
			t.Error("Sweep removed a blob although a referrer failed")
		}
	})

	t.Run("ConcurrentPutsOfTheSameBytes", func(t *testing.T) {
		s := newFixture(t).Store
		data := random(t, 64<<10)
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h, err := s.Put(ctx, bytes.NewReader(data))
				if err == nil && h != blobs.HashOf(data) {
					err = errors.New("wrong hash " + string(h))
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Errorf("a concurrent Put failed: %v", err)
			}
		}
		if got, err := read(s, blobs.HashOf(data)); err != nil || !bytes.Equal(got, data) {
			t.Errorf("after concurrent Puts, Open read %d bytes, %v", len(got), err)
		}
	})
}

func put(t *testing.T, s blobs.Store, data []byte) blobs.Hash {
	t.Helper()
	h, err := s.Put(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func read(s blobs.Store, h blobs.Hash) ([]byte, error) {
	rc, err := s.Open(context.Background(), h)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func keep(hs ...blobs.Hash) blobs.Referrer {
	return func(_ context.Context, keep func(blobs.Hash)) error {
		for _, h := range hs {
			keep(h)
		}
		return nil
	}
}

func random(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}
