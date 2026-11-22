package blobs

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

// Key returns where a store keeps h, relative to its root: "sha256/ab/cdef...", split after the
// first two hex digits. Dir uses this layout on disk, and a bucket store uses it under its prefix,
// so a tree of blobs copies between them unchanged. It returns ErrInvalidHash for a malformed h.
func Key(h Hash) (string, error) {
	r, err := rel(h)
	if err != nil {
		return "", err
	}
	return "sha256/" + r, nil
}

// HashOfKey is the inverse of Key. It reports false for a key that names no blob, such as a stray
// object a person put under the store's prefix.
func HashOfKey(key string) (Hash, bool) {
	rest, ok := strings.CutPrefix(key, "sha256/")
	if !ok || len(rest) != 65 || rest[2] != '/' {
		return "", false
	}
	h := Hash("sha256:" + rest[:2] + rest[3:])
	if _, err := hexOf(h); err != nil {
		return "", false
	}
	return h, true
}

// Spool copies r into a new temp file in dir (os.TempDir when empty), hashing as it goes, and
// returns the file rewound to its start with its hash and size. A store whose backend needs the
// key before the upload, such as a bucket, spools first. The caller closes and removes the file.
func Spool(r io.Reader, dir string) (*os.File, Hash, int64, error) {
	f, err := os.CreateTemp(dir, "blob-spool-*")
	if err != nil {
		return nil, "", 0, err
	}
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, sum), r)
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, "", 0, err
	}
	return f, Hash("sha256:" + hex.EncodeToString(sum.Sum(nil))), n, nil
}
