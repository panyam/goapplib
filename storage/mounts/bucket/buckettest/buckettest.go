// Package buckettest is an in-memory bucket.Bucket, with or without versions, for testing code
// over bucket mounts without a cloud fake.
package buckettest

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G501 -- an S3-style ETag, not a security check
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
)

type version struct {
	id   string
	etag string
	data []byte
}

// Bucket is one in-memory bucket. With versions on, every Put keeps the old contents under their
// version id. With them off, a Put replaces them, as on a bucket without versioning.
type Bucket struct {
	mu       sync.Mutex
	versions bool
	objs     map[string][]version
	next     int
	offline  bool
}

// New returns an empty bucket.
func New(versions bool) *Bucket {
	return &Bucket{versions: versions, objs: map[string][]version{}}
}

// Put writes data under key.
func (b *Bucket) Put(key string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sum := md5.Sum(data) // #nosec G401 -- an ETag
	v := version{etag: `"` + hex.EncodeToString(sum[:]) + `"`, data: append([]byte(nil), data...)}
	if b.versions {
		b.next++
		v.id = "v" + strconv.Itoa(b.next)
		b.objs[key] = append(b.objs[key], v)
		return
	}
	b.objs[key] = []version{v}
}

// Offline makes every later connection fail with mounts.ErrUnavailable, as if the service were
// down, so a test can show what works without the bucket.
func (b *Bucket) Offline() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.offline = true
}

// Connector returns a bucket.Connector serving b under name, and ErrNotFound for any other name.
func (b *Bucket) Connector(name string) bucket.Connector {
	return func(_ context.Context, n string, _ mounts.Credentials) (bucket.Bucket, error) {
		b.mu.Lock()
		offline := b.offline
		b.mu.Unlock()
		if offline {
			return nil, fmt.Errorf("%w: the bucket is offline", mounts.ErrUnavailable)
		}
		if n != name {
			return missing{}, nil
		}
		return b, nil
	}
}

func object(key string, v version) bucket.Object {
	return bucket.Object{Key: key, Version: v.id, ETag: v.etag, Size: int64(len(v.data))}
}

// List implements bucket.Bucket.
func (b *Bucket) List(_ context.Context, prefix string) ([]bucket.Object, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []bucket.Object
	for k, vs := range b.objs {
		if strings.HasPrefix(k, prefix) {
			out = append(out, object(k, vs[len(vs)-1]))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ListDir implements bucket.Bucket.
func (b *Bucket) ListDir(_ context.Context, dir string) ([]bucket.Object, []string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var files []bucket.Object
	seen := map[string]bool{}
	var folders []string
	for k, vs := range b.objs {
		rest, ok := strings.CutPrefix(k, dir)
		if !ok {
			continue
		}
		if f, _, isDir := strings.Cut(rest, "/"); isDir {
			if !seen[f] {
				seen[f] = true
				folders = append(folders, f)
			}
			continue
		}
		files = append(files, object(k, vs[len(vs)-1]))
	}
	return files, folders, nil
}

// Get implements bucket.Bucket.
func (b *Bucket) Get(_ context.Context, key string, pin *bucket.Object) (io.ReadCloser, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	vs, ok := b.objs[key]
	if !ok {
		return nil, 0, fmt.Errorf("%w: %s", mounts.ErrNotFound, key)
	}
	v := vs[len(vs)-1]
	switch {
	case pin == nil:
	case pin.Version != "":
		found := false
		for _, old := range vs {
			if old.id == pin.Version {
				v, found = old, true
			}
		}
		if !found {
			return nil, 0, fmt.Errorf("%w: %s has no version %s", mounts.ErrConflict, key, pin.Version)
		}
	case v.etag != pin.ETag:
		return nil, 0, fmt.Errorf("%w: %s changed since it was listed", mounts.ErrConflict, key)
	}
	return io.NopCloser(bytes.NewReader(v.data)), int64(len(v.data)), nil
}

type missing struct{}

func (missing) List(context.Context, string) ([]bucket.Object, error) {
	return nil, fmt.Errorf("%w: no such bucket", mounts.ErrNotFound)
}
func (missing) ListDir(context.Context, string) ([]bucket.Object, []string, error) {
	return nil, nil, fmt.Errorf("%w: no such bucket", mounts.ErrNotFound)
}
func (missing) Get(context.Context, string, *bucket.Object) (io.ReadCloser, int64, error) {
	return nil, 0, fmt.Errorf("%w: no such bucket", mounts.ErrNotFound)
}
