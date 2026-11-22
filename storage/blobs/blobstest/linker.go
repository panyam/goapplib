package blobstest

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/blobs"
)

// RunLinker runs the Linker checks against stores from newFixture, whose Store must be a
// blobs.Linker. A Linker runs RunStore too.
func RunLinker(t *testing.T, newFixture Factory) {
	linker := func(t *testing.T) blobs.Linker {
		l, ok := newFixture(t).Store.(blobs.Linker)
		if !ok {
			t.Fatal("the store isn't a blobs.Linker")
		}
		return l
	}

	t.Run("LinkGivesTheBlobsBytes", func(t *testing.T) {
		l := linker(t)
		for _, data := range [][]byte{[]byte("hello"), {}, random(t, 1<<20)} {
			h := put(t, l, data)
			to := filepath.Join(t.TempDir(), "file")
			if err := l.Link(h, to); err != nil {
				t.Fatalf("Link of %d bytes: %v", len(data), err)
			}
			if got, err := os.ReadFile(to); err != nil || !bytes.Equal(got, data) {
				t.Errorf("the linked file of %d bytes read %d bytes, %v", len(data), len(got), err)
			}
		}
	})

	t.Run("LinkMissingIsNotFound", func(t *testing.T) {
		l := linker(t)
		to := filepath.Join(t.TempDir(), "file")
		if err := l.Link(blobs.HashOf([]byte("never stored")), to); !errors.Is(err, blobs.ErrNotFound) {
			t.Errorf("Link of a missing blob = %v, want ErrNotFound", err)
		}
		if _, err := os.Stat(to); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Link of a missing blob left a file: %v", err)
		}
	})

	t.Run("LinkMalformedHashIsInvalid", func(t *testing.T) {
		l := linker(t)
		for _, h := range []blobs.Hash{"", "sha256:../../etc/passwd", "sha256:" + blobs.Hash(strings.Repeat("A", 64))} {
			if err := l.Link(h, filepath.Join(t.TempDir(), "file")); !errors.Is(err, blobs.ErrInvalidHash) {
				t.Errorf("Link(%q) = %v, want ErrInvalidHash", h, err)
			}
		}
	})
}
