package blobs

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Cached is a Linker that keeps a local Dir in front of a remote Store, such as a bucket store.
// An app whose blobs live in a bucket can then still build folder views, since views need blobs on
// this server's disk.
//
// The remote holds every blob, and the local Dir holds copies. Put writes both before it returns,
// so a server that dies after Put loses nothing. Open and Link serve the local copy, and fetch it
// from the remote on a miss, so a local folder lost on restart refills on demand. Several servers
// may share one remote, each with its own Dir.
type Cached struct {
	remote Store
	local  *Dir
	maxAge time.Duration
}

// CachedOption configures a Cached.
type CachedOption func(*Cached)

// WithMaxAge lets Sweep drop any local copy fetched or put more than d ago, whether or not a
// referrer names it, since the remote still holds the blob. The next Open or Link fetches it
// again. Zero, the default, keeps a referenced local copy for as long as the remote keeps it.
func WithMaxAge(d time.Duration) CachedOption { return func(c *Cached) { c.maxAge = d } }

// NewCached returns a Cached that keeps every blob in remote and copies in local.
func NewCached(remote Store, local *Dir, opts ...CachedOption) *Cached {
	c := &Cached{remote: remote, local: local}
	for _, o := range opts {
		o(c)
	}
	return c
}

var (
	_ Linker  = (*Cached)(nil)
	_ Aliases = (*Cached)(nil)
)

// Alias implements Aliases from the local Dir, so each server keeps its own aliases. A server
// whose folder was lost finds none, and its next fetch records them again. Callers check Has
// before using a hash, and Has asks the remote, so an alias never hands out a blob that is gone.
func (c *Cached) Alias(ctx context.Context, ns, id string) (Hash, bool, error) {
	return c.local.Alias(ctx, ns, id)
}

// SetAlias implements Aliases in the local Dir.
func (c *Cached) SetAlias(ctx context.Context, ns, id string, h Hash) error {
	return c.local.SetAlias(ctx, ns, id, h)
}

// Put implements Store. It writes the local copy first, then sends that copy to the remote, and
// returns only after both hold the blob. A remote that stores the bytes under some other hash
// fails the Put with ErrCorrupt.
func (c *Cached) Put(ctx context.Context, r io.Reader) (Hash, error) {
	h, err := c.local.Put(ctx, r)
	if err != nil {
		return "", err
	}
	f, err := c.local.Open(ctx, h)
	if err != nil {
		return "", err
	}
	defer f.Close()
	got, err := c.remote.Put(ctx, f)
	if err != nil {
		return "", err
	}
	if got != h {
		return "", fmt.Errorf("%w: the remote stored %s as %s", ErrCorrupt, h, got)
	}
	return h, nil
}

// Open implements Store. On a local miss it fetches the blob from the remote and keeps it. A
// remote blob whose bytes don't match h fails with ErrCorrupt and is never kept locally. A blob
// neither holds returns ErrNotFound.
func (c *Cached) Open(ctx context.Context, h Hash) (io.ReadCloser, error) {
	if err := c.fill(ctx, h); err != nil {
		return nil, err
	}
	return c.local.Open(ctx, h)
}

// Has implements Store. It asks the local Dir first, then the remote.
func (c *Cached) Has(ctx context.Context, h Hash) (bool, error) {
	if ok, err := c.local.Has(ctx, h); err != nil || ok {
		return ok, err
	}
	return c.remote.Has(ctx, h)
}

// Sweep implements Store, and returns how many blobs it removed from the remote. It calls each
// referrer once and sweeps the remote and then the local Dir with what they named, so a local copy
// goes when its blob is unreferenced and older than grace. A local copy can outlive its remote
// blob by one sweep, when it was fetched after the remote wrote it. With WithMaxAge, it then drops
// every local copy older than the max age.
func (c *Cached) Sweep(ctx context.Context, grace time.Duration, refs ...Referrer) (int, error) {
	keep := map[Hash]bool{}
	for _, ref := range refs {
		if err := ref(ctx, func(h Hash) { keep[h] = true }); err != nil {
			return 0, err
		}
	}
	kept := func(_ context.Context, k func(Hash)) error {
		for h := range keep {
			k(h)
		}
		return nil
	}
	n, err := c.remote.Sweep(ctx, grace, kept)
	if err != nil {
		return n, err
	}
	if _, err := c.local.Sweep(ctx, grace, kept); err != nil {
		return n, err
	}
	if c.maxAge > 0 {
		if _, err := c.local.Sweep(ctx, c.maxAge); err != nil {
			return n, err
		}
	}
	return n, nil
}

// Link implements Linker. It fetches the blob from the remote first when the local Dir lacks it,
// and fails as Open does when it can't.
func (c *Cached) Link(h Hash, to string) error {
	if err := c.fill(context.Background(), h); err != nil {
		return err
	}
	return c.local.Link(h, to)
}

// fill makes sure the local Dir holds h. The remote's reader checks the hash, so a corrupt blob
// fails the local Put, which then removes its temp file.
func (c *Cached) fill(ctx context.Context, h Hash) error {
	if ok, err := c.local.Has(ctx, h); err != nil || ok {
		return err
	}
	rc, err := c.remote.Open(ctx, h)
	if err != nil {
		return err
	}
	defer rc.Close()
	got, err := c.local.Put(ctx, rc)
	if err != nil {
		return err
	}
	if got != h {
		return fmt.Errorf("%w: the remote returned %s for %s", ErrCorrupt, got, h)
	}
	return nil
}
