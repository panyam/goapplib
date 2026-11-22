// Package blobs keeps files by the SHA-256 of their contents. Each file is stored once, never
// changed, and removed only by a sweep that finds nothing pointing at it. A GitHub snapshot lands
// here, and so do an app's uploads.
//
// A blob store differs from a cache such as wasmhost's: a cache may lose an entry and rebuild it,
// and a blob store may not. The design is at https://panyam.github.io/goapplib/reference/blobs/.
package blobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"time"
)

// Hash names a blob: "sha256:" and 64 lowercase hex digits. It is agni's format.
type Hash string

// HashOf returns the Hash of b.
func HashOf(b []byte) Hash {
	sum := sha256.Sum256(b)
	return Hash("sha256:" + hex.EncodeToString(sum[:]))
}

// hexOf returns h's 64 hex digits, or ErrInvalidHash. Nothing that fails it can name a path, so a
// malformed hash can't reach outside a store's folder.
func hexOf(h Hash) (string, error) {
	x, ok := strings.CutPrefix(string(h), "sha256:")
	if !ok || len(x) != 64 || strings.Trim(x, "0123456789abcdef") != "" {
		return "", fmt.Errorf("%w: %q", ErrInvalidHash, h)
	}
	return x, nil
}

// ParseHash checks that s is a Hash, and returns ErrInvalidHash if not.
func ParseHash(s string) (Hash, error) {
	if _, err := hexOf(Hash(s)); err != nil {
		return "", err
	}
	return Hash(s), nil
}

var (
	// ErrNotFound means the store holds no blob with that hash.
	ErrNotFound = errors.New("blobs: not found")

	// ErrInvalidHash means a hash isn't "sha256:" and 64 lowercase hex digits.
	ErrInvalidHash = errors.New("blobs: invalid hash")

	// ErrCorrupt means a blob's bytes no longer hash to its name, because something changed or
	// truncated the stored file. Open's reader returns it at the end of the blob, in place of
	// io.EOF.
	ErrCorrupt = errors.New("blobs: contents don't match the hash")

	// ErrInvalidManifest means a manifest has a path that is absolute, unclean or climbs out,
	// a duplicate path, or a malformed hash.
	ErrInvalidManifest = errors.New("blobs: invalid manifest")
)

// Store keeps blobs. Several processes may share one, and Put is safe to call from all of them at
// once.
type Store interface {
	// Put stores what r yields and returns its hash. Putting contents the store already holds
	// writes nothing new, and counts as a fresh write for Sweep's grace period, so a sweep can't
	// remove the blob before the caller names it.
	Put(ctx context.Context, r io.Reader) (Hash, error)

	// Open returns the blob's contents. The reader checks the hash as it goes. If the bytes
	// don't match, the final Read returns ErrCorrupt in place of io.EOF, so only a caller who
	// reads to the end gets the check. A missing blob returns ErrNotFound.
	Open(ctx context.Context, h Hash) (io.ReadCloser, error)

	// Has reports whether the store holds h.
	Has(ctx context.Context, h Hash) (bool, error)

	// Sweep removes each blob that no referrer names and that was written more than grace ago,
	// and returns how many it removed. The grace period spares a blob that a caller has put but
	// not yet named in a record. If any referrer fails, Sweep removes nothing and returns that
	// error, since it can't know what is in use.
	Sweep(ctx context.Context, grace time.Duration, refs ...Referrer) (int, error)
}

// Referrer lists the hashes one owner still names, by calling keep for each. Every owner of blobs
// in a store passes one to Sweep, such as an app's designs and the mounts registry's snapshots, and
// Sweep keeps the union.
type Referrer func(ctx context.Context, keep func(Hash)) error

// Linker is a Store on this server's disk, whose blobs can appear as files in a folder view.
type Linker interface {
	Store

	// Link makes the file at to hold h's contents. It hard-links where the filesystem allows,
	// so the view takes no extra space, and copies where it doesn't. A linked file keeps its
	// contents after Sweep removes the blob. It returns ErrInvalidHash or ErrNotFound for a bad
	// or missing h.
	Link(h Hash, to string) error
}

// Verify wraps rc, the contents a store read for h, so that reading checks them against h. When
// the bytes don't match, the final Read returns ErrCorrupt in place of io.EOF, as Store.Open
// promises. A store outside this package returns it from Open.
func Verify(rc io.ReadCloser, h Hash) io.ReadCloser {
	return &verifier{rc: rc, want: h, sum: sha256.New()}
}

type verifier struct {
	rc   io.ReadCloser
	want Hash
	sum  hash.Hash
}

func (v *verifier) Read(p []byte) (int, error) {
	n, err := v.rc.Read(p)
	v.sum.Write(p[:n])
	if err == io.EOF && Hash("sha256:"+hex.EncodeToString(v.sum.Sum(nil))) != v.want {
		return n, fmt.Errorf("%w: %s", ErrCorrupt, v.want)
	}
	return n, err
}

func (v *verifier) Close() error { return v.rc.Close() }
