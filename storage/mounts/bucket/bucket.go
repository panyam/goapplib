// Package bucket serves mounts over an object store bucket, such as S3 or Google Cloud Storage,
// through the small Bucket interface. The SDK-specific drivers in storage/drivers/s3 and
// storage/drivers/gcs implement Bucket and hand it to New, so this package holds the listing,
// pinning and fs.FS logic once and never imports a cloud SDK.
//
// A bucket has no commit, since no change spans several objects atomically. So Resolve lists the
// source's prefix and records each object's version: its version id where the bucket keeps
// versions (S3 VersionId, GCS generation), or its ETag where it doesn't. The listing is stored as
// a blob, and its hash is the mount's version. A snapshot mount then reads exactly the listed
// versions. Where an object is pinned by ETag and has changed since, reading it fails with the path
// named and mounts.ErrConflict, rather than serving the new bytes as the old version.
//
// The same version works as a change check. Resolve again, and a new version means some object
// under the prefix was added, removed or changed.
package bucket

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/panyam/goapplib/storage/mounts"
)

// Object is one object under a source's prefix.
type Object struct {
	// Key is the object's full key in the bucket.
	Key string `json:"key"`

	// Version is the bucket's version id for this object's contents, such as an S3 VersionId or
	// a GCS generation. It is empty when the bucket doesn't keep versions.
	Version string `json:"version,omitempty"`

	// ETag is the object's entity tag, as the bucket reports it.
	ETag string `json:"etag,omitempty"`

	Size int64 `json:"size"`
}

// Bucket is one bucket, signed in with one set of credentials. Methods return errors wrapping the
// mounts sentinels: ErrNotFound for a missing bucket or key, ErrConflict when a pinned version is
// gone or its ETag no longer matches, ErrPermissionDenied for a refusal, and ErrUnavailable for a
// network or server failure.
type Bucket interface {
	// List returns every object under prefix, at any depth, in key order. A prefix of "" lists
	// the whole bucket. It reports versions where the bucket keeps them.
	List(ctx context.Context, prefix string) ([]Object, error)

	// ListDir returns the objects directly under dir, and the names of the folders under it
	// (the common prefixes, without dir or the trailing "/"). dir is "" or ends in "/".
	ListDir(ctx context.Context, dir string) (files []Object, folders []string, err error)

	// Get reads an object. With pin nil it reads key's latest version. Otherwise it reads
	// pin.Version, or, when that is empty, the latest version only if its ETag is still
	// pin.ETag. The returned size is the object's length.
	Get(ctx context.Context, key string, pin *Object) (io.ReadCloser, int64, error)
}

// Connector opens the bucket named name with creds. A nil creds means a public bucket. A kind of
// mounts.Credentials the service can't sign with returns mounts.ErrInvalidArgument.
type Connector func(ctx context.Context, name string, creds mounts.Credentials) (Bucket, error)

// Source is a parsed bucket source, such as "s3://designs/boards/jetson".
type Source struct {
	Bucket string
	// Prefix is the folder inside the bucket, clean, with no leading or trailing "/". Empty means
	// the whole bucket.
	Prefix string
}

// keyPrefix is the prefix every key under s starts with: Prefix and "/", or "" for the bucket.
func (s Source) keyPrefix() string {
	if s.Prefix == "" {
		return ""
	}
	return s.Prefix + "/"
}

// ParseSource parses "<scheme>://<bucket>/<prefix>". The prefix may be empty. A prefix that is
// unclean, or climbs, returns mounts.ErrInvalidArgument.
func ParseSource(scheme, source string) (Source, error) {
	rest, ok := strings.CutPrefix(source, scheme+"://")
	if !ok {
		return Source{}, fmt.Errorf("%w: %q isn't a %s:// source", mounts.ErrInvalidArgument, source, scheme)
	}
	name, prefix, _ := strings.Cut(rest, "/")
	prefix = strings.TrimSuffix(prefix, "/")
	if name == "" || strings.ContainsAny(name, `\ `) {
		return Source{}, fmt.Errorf("%w: %q names no bucket", mounts.ErrInvalidArgument, source)
	}
	if prefix != "" {
		if p, err := mounts.CleanPath(prefix); err != nil || p != prefix {
			return Source{}, fmt.Errorf("%w: prefix %q in %q isn't a clean path", mounts.ErrInvalidArgument, prefix, source)
		}
	}
	return Source{Bucket: name, Prefix: prefix}, nil
}

// relPath turns key into a path inside s, or "" when the key can't be a file there: a folder
// placeholder ending in "/", or a key whose rest isn't a clean relative path.
func (s Source) relPath(key string) string {
	p, ok := strings.CutPrefix(key, s.keyPrefix())
	if !ok || p == "" || strings.HasSuffix(p, "/") {
		return ""
	}
	if clean, err := mounts.CleanPath(p); err != nil || clean != p {
		return ""
	}
	return p
}

// listingPath is where the listing sits in a snapshot's blobs.Manifest. The manifest lists the
// listing as its one file, so the registry's BlobRefs keeps the listing through a sweep.
const listingPath = ".bucket-listing.json"

// filesDir is where Prepare lists each fetched file in a snapshot's manifest, as filesDir and the
// file's path, so no object's path can collide with listingPath.
const filesDir = "files/"
