package blobs

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestKeyRoundTripsAndMatchesDirLayout(t *testing.T) {
	h := HashOf([]byte("hello"))
	k, err := Key(h)
	x := string(h[len("sha256:"):])
	if err != nil || k != "sha256/"+x[:2]+"/"+x[2:] {
		t.Fatalf("Key = %q, %v", k, err)
	}
	if got, ok := HashOfKey(k); !ok || got != h {
		t.Errorf("HashOfKey(%q) = %q, %v", k, got, ok)
	}
	for _, bad := range []string{"", "sha256/", "sha256/ab", "tmp/" + k, "sha256/" + x, "sha256/" + strings.ToUpper(x[:2]) + "/" + x[2:]} {
		if got, ok := HashOfKey(bad); ok {
			t.Errorf("HashOfKey(%q) = %q, want no blob", bad, got)
		}
	}
	if _, err := Key("sha256:nope"); err == nil {
		t.Error("Key of a malformed hash should fail")
	}
}

func TestSpoolHashesAndRewinds(t *testing.T) {
	f, h, n, err := Spool(strings.NewReader("hello"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	b, _ := io.ReadAll(f)
	if h != HashOf([]byte("hello")) || n != 5 || string(b) != "hello" {
		t.Errorf("Spool = %s, %d, %q", h, n, b)
	}
}
