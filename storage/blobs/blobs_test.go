package blobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newDir(t *testing.T) (*Dir, string) {
	t.Helper()
	root := t.TempDir()
	d, err := NewDir(root)
	if err != nil {
		t.Fatal(err)
	}
	return d, root
}

func put(t *testing.T, d *Dir, s string) Hash {
	t.Helper()
	h, err := d.Put(context.Background(), strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func read(t *testing.T, d *Dir, h Hash) ([]byte, error) {
	t.Helper()
	r, err := d.Open(context.Background(), h)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func age(t *testing.T, p string, by time.Duration) {
	t.Helper()
	old := time.Now().Add(-by)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestPutOpenRoundTrip(t *testing.T) {
	d, root := newDir(t)
	h := put(t, d, "hello")
	if h != HashOf([]byte("hello")) {
		t.Fatalf("Put returned %s, want %s", h, HashOf([]byte("hello")))
	}
	if !strings.HasPrefix(string(h), "sha256:") || len(h) != len("sha256:")+64 {
		t.Fatalf("hash %q is not sha256:<64 hex>", h)
	}
	got, err := read(t, d, h)
	if err != nil || string(got) != "hello" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	hex := string(h[len("sha256:"):])
	if _, err := os.Stat(filepath.Join(root, "sha256", hex[:2], hex[2:])); err != nil {
		t.Errorf("blob isn't at agni's layout <root>/sha256/ab/cdef...: %v", err)
	}
	if ok, err := d.Has(context.Background(), h); !ok || err != nil {
		t.Errorf("Has = %v, %v", ok, err)
	}
}

func TestOpenMissingAndMalformed(t *testing.T) {
	d, _ := newDir(t)
	if _, err := read(t, d, HashOf([]byte("never stored"))); !errors.Is(err, ErrNotFound) {
		t.Errorf("Open of a missing blob = %v, want ErrNotFound", err)
	}
	if ok, err := d.Has(context.Background(), HashOf([]byte("never stored"))); ok || err != nil {
		t.Errorf("Has of a missing blob = %v, %v", ok, err)
	}
	for _, h := range []Hash{"", "sha256:", "sha256:../../etc/passwd", "md5:" + Hash(strings.Repeat("a", 64)), "sha256:" + Hash(strings.Repeat("A", 64)), "sha256:" + Hash(strings.Repeat("a", 63))} {
		if _, err := read(t, d, h); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("Open(%q) = %v, want ErrInvalidHash", h, err)
		}
		if _, err := d.Path(h); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("Path(%q) = %v, want ErrInvalidHash", h, err)
		}
	}
}

func TestOpenReportsCorruption(t *testing.T) {
	d, _ := newDir(t)
	h := put(t, d, "original bytes")
	p, err := d.Path(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("changed bytes!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := read(t, d, h); !errors.Is(err, ErrCorrupt) {
		t.Errorf("reading a changed blob = %v, want ErrCorrupt", err)
	}
}

func TestPutTwiceKeepsOneCopyAndRefreshesItsAge(t *testing.T) {
	d, _ := newDir(t)
	h := put(t, d, "same")
	p, _ := d.Path(h)
	age(t, p, time.Hour)
	if put(t, d, "same") != h {
		t.Fatal("the same contents got a different hash")
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(fi.ModTime()) > time.Minute {
		t.Errorf("a second Put left the blob's mtime %v old, so a sweep could remove it before the caller names it", time.Since(fi.ModTime()))
	}
	n, err := d.Sweep(context.Background(), 10*time.Minute)
	if err != nil || n != 0 {
		t.Errorf("Sweep right after a re-Put removed %d, %v; want 0", n, err)
	}
}

func TestSweepKeepsReferencedAndRecent(t *testing.T) {
	d, root := newDir(t)
	ctx := context.Background()
	a, b, c, recent := put(t, d, "a"), put(t, d, "b"), put(t, d, "c"), put(t, d, "recent")
	for _, h := range []Hash{a, b, c} {
		p, _ := d.Path(h)
		age(t, p, time.Hour)
	}
	stale := filepath.Join(root, "sha256", ".put-crashed")
	if err := os.WriteFile(stale, []byte("half"), 0o600); err != nil {
		t.Fatal(err)
	}
	age(t, stale, time.Hour)

	n, err := d.Sweep(ctx, 10*time.Minute,
		func(_ context.Context, keep func(Hash)) error { keep(a); return nil },
		func(_ context.Context, keep func(Hash)) error { keep(b); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("Sweep removed %d blobs, want 1 (c)", n)
	}
	for h, want := range map[Hash]bool{a: true, b: true, c: false, recent: true} {
		if ok, _ := d.Has(ctx, h); ok != want {
			t.Errorf("after Sweep, Has(%s) = %v, want %v", h, ok, want)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("Sweep left an old temp file from a crashed Put: %v", err)
	}
}

func TestSweepStopsOnReferrerError(t *testing.T) {
	d, _ := newDir(t)
	h := put(t, d, "x")
	p, _ := d.Path(h)
	age(t, p, time.Hour)
	boom := errors.New("store unreachable")
	if _, err := d.Sweep(context.Background(), 0, func(context.Context, func(Hash)) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Sweep = %v, want the referrer's error", err)
	}
	if ok, _ := d.Has(context.Background(), h); !ok {
		t.Error("Sweep removed a blob although a referrer failed, so it couldn't know what was in use")
	}
}

func TestLink(t *testing.T) {
	d, _ := newDir(t)
	h := put(t, d, "linked")
	to := filepath.Join(t.TempDir(), "view.txt")
	if err := d.Link(h, to); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(to); err != nil || string(b) != "linked" {
		t.Errorf("linked file = %q, %v", b, err)
	}
	if err := d.Link("sha256:nope", filepath.Join(t.TempDir(), "x")); !errors.Is(err, ErrInvalidHash) {
		t.Errorf("Link of a malformed hash = %v, want ErrInvalidHash", err)
	}
	if err := d.Link(HashOf([]byte("absent")), filepath.Join(t.TempDir(), "x")); !errors.Is(err, ErrNotFound) {
		t.Errorf("Link of a missing blob = %v, want ErrNotFound", err)
	}
}

func TestManifestRoundTripAndRefs(t *testing.T) {
	d, _ := newDir(t)
	ctx := context.Background()
	x, y := put(t, d, "x"), put(t, d, "y")
	m := Manifest{{Path: "b/y.txt", Hash: y, Size: 1}, {Path: "a/x.txt", Hash: x, Size: 1}}
	mh, err := PutManifest(ctx, d, m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetManifest(ctx, d, mh)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "a/x.txt" || got[1].Path != "b/y.txt" {
		t.Errorf("GetManifest = %+v, want both entries sorted by path", got)
	}
	again, _ := PutManifest(ctx, d, Manifest{m[1], m[0]})
	if again != mh {
		t.Error("the same files in another order gave a different manifest hash")
	}
	var refs []Hash
	if err := ManifestRefs(ctx, d, mh, func(h Hash) { refs = append(refs, h) }); err != nil {
		t.Fatal(err)
	}
	if want := []Hash{mh, x, y}; !bytes.Equal([]byte(join(refs)), []byte(join(want))) {
		t.Errorf("ManifestRefs = %v, want %v", refs, want)
	}
	if _, err := PutManifest(ctx, d, Manifest{{Path: "../escape", Hash: x}}); !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("a climbing path = %v, want ErrInvalidManifest", err)
	}
	if _, err := PutManifest(ctx, d, Manifest{{Path: "a", Hash: x}, {Path: "a", Hash: y}}); !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("a duplicate path = %v, want ErrInvalidManifest", err)
	}
}

func join(hs []Hash) string {
	var b strings.Builder
	for _, h := range hs {
		b.WriteString(string(h) + "\n")
	}
	return b.String()
}
