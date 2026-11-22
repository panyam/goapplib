package bucket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
)

// Driver serves one kind of bucket mount. It implements mounts.Driver, mounts.Preparer and
// mounts.VersionReader. It can't write, so a live mount is read-only.
type Driver struct {
	kind    mounts.Kind
	scheme  string
	connect Connector
	blobs   blobs.Store
	limits  mounts.Limits
}

// Option configures a Driver.
type Option func(*Driver)

// WithLimits caps how many objects and bytes a source may hold. A zero field keeps
// mounts.DefaultLimits' value. Past either, Resolve returns mounts.ErrTooLarge.
func WithLimits(l mounts.Limits) Option { return func(d *Driver) { d.limits = l } }

// New returns a driver for kind, whose sources look like "<scheme>://<bucket>/<prefix>". connect
// opens a bucket for each call. store keeps each Resolve's listing, which a snapshot mount and
// the VersionReader methods read back. With a nil store, live mounts work, Resolve still returns
// a version, and a snapshot mount returns mounts.ErrInvalidArgument.
func New(kind mounts.Kind, scheme string, connect Connector, store blobs.Store, opts ...Option) *Driver {
	d := &Driver{kind: kind, scheme: scheme, connect: connect, blobs: store}
	for _, o := range opts {
		o(d)
	}
	if d.limits.MaxBytes <= 0 {
		d.limits.MaxBytes = mounts.DefaultLimits.MaxBytes
	}
	if d.limits.MaxFiles <= 0 {
		d.limits.MaxFiles = mounts.DefaultLimits.MaxFiles
	}
	return d
}

// Kind returns the kind New was given.
func (d *Driver) Kind() mounts.Kind { return d.kind }

// Modes returns snapshot and live.
func (d *Driver) Modes() []mounts.Mode { return []mounts.Mode{mounts.ModeSnapshot, mounts.ModeLive} }

// listing is what Resolve records: every object under the source with the version it had.
type listing struct {
	Source  string   `json:"source"`
	Objects []Object `json:"objects"`
}

// Resolve lists the source and returns the hash of the listing as the version, keeping the
// listing in the blob store when the driver has one. A bucket has no refs, so a non-empty ref
// returns mounts.ErrInvalidArgument. A missing bucket, or a prefix with no objects under it,
// returns mounts.ErrNotFound.
func (d *Driver) Resolve(ctx context.Context, source, ref string, creds mounts.Credentials) (string, error) {
	if ref != "" {
		return "", fmt.Errorf("%w: a bucket has no refs, got %q", mounts.ErrInvalidArgument, ref)
	}
	src, b, err := d.open(ctx, source, creds)
	if err != nil {
		return "", err
	}
	all, err := b.List(ctx, src.keyPrefix())
	if err != nil {
		return "", err
	}
	l := listing{Source: source}
	var total int64
	for _, o := range all {
		if src.relPath(o.Key) == "" {
			continue
		}
		l.Objects = append(l.Objects, o)
		total += o.Size
	}
	if len(l.Objects) == 0 && src.Prefix != "" {
		return "", fmt.Errorf("%w: no objects under %s", mounts.ErrNotFound, source)
	}
	if len(l.Objects) > d.limits.MaxFiles || total > d.limits.MaxBytes {
		return "", fmt.Errorf("%w: %s holds %d objects and %d bytes", mounts.ErrTooLarge, source, len(l.Objects), total)
	}
	sort.Slice(l.Objects, func(i, j int) bool { return l.Objects[i].Key < l.Objects[j].Key })
	body, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	if d.blobs == nil {
		return string(blobs.HashOf(body)), nil
	}
	h, err := d.blobs.Put(ctx, bytes.NewReader(body))
	return string(h), err
}

// Prepare pins a snapshot mount to the listing its Resolve kept, by setting Manifest to a
// blobs.Manifest whose one entry is that listing. A live mount comes back unchanged.
func (d *Driver) Prepare(ctx context.Context, m mounts.Mount, creds mounts.Credentials) (mounts.Mount, error) {
	if m.Mode != mounts.ModeSnapshot {
		return m, nil
	}
	l, err := d.listing(ctx, m.Version)
	if err != nil {
		return m, err
	}
	if l.Source != m.Source {
		return m, fmt.Errorf("%w: version %s lists %s, not %s", mounts.ErrInvalidArgument, m.Version, l.Source, m.Source)
	}
	h, err := blobs.PutManifest(ctx, d.blobs, blobs.Manifest{{Path: listingPath, Hash: blobs.Hash(m.Version)}})
	if err != nil {
		return m, err
	}
	m.Manifest = string(h)
	return m, nil
}

// Open returns a snapshot mount's files at the versions its listing pinned, or a live mount's
// latest objects. Opening a snapshot file whose pinned version is gone, or whose ETag changed on
// a bucket without versions, fails with an fs.PathError naming the file and wrapping
// mounts.ErrConflict.
func (d *Driver) Open(ctx context.Context, m mounts.Mount, creds mounts.Credentials) (fs.FS, error) {
	src, b, err := d.open(ctx, m.Source, creds)
	if err != nil {
		return nil, err
	}
	switch m.Mode {
	case mounts.ModeLive:
		return liveFS{ctx: ctx, b: b, src: src}, nil
	case mounts.ModeSnapshot:
		h, err := blobs.ParseHash(m.Manifest)
		if err != nil {
			return nil, fmt.Errorf("%w: snapshot mount %q has no manifest", mounts.ErrInvalidArgument, m.Name)
		}
		if d.blobs == nil {
			return nil, fmt.Errorf("%w: %s", mounts.ErrInvalidArgument, errNoBlobs)
		}
		man, err := blobs.GetManifest(ctx, d.blobs, h)
		if err != nil {
			return nil, err
		}
		if len(man) != 1 || man[0].Path != listingPath {
			return nil, fmt.Errorf("%w: snapshot mount %q's manifest isn't a bucket listing", mounts.ErrInvalidArgument, m.Name)
		}
		l, err := d.listing(ctx, string(man[0].Hash))
		if err != nil {
			return nil, err
		}
		return newSnapshotFS(ctx, b, src, l.Objects), nil
	}
	return nil, fmt.Errorf("%w: %s mounts don't support mode %q", mounts.ErrInvalidArgument, d.kind, m.Mode)
}

// Moves reports true for the empty ref, the only one a bucket has, since the latest objects can
// change at any time.
func (d *Driver) Moves(ctx context.Context, source, ref string, creds mounts.Credentials) (bool, error) {
	if ref != "" {
		return false, fmt.Errorf("%w: a bucket has no refs, got %q", mounts.ErrInvalidArgument, ref)
	}
	return true, nil
}

// ListAt lists the files of the listing version names, under the path under. Each File's ID is
// "version:" and the object's version id, or "etag:" and its ETag on a bucket without versions.
func (d *Driver) ListAt(ctx context.Context, m mounts.Mount, creds mounts.Credentials, version, under string) ([]mounts.File, error) {
	src, err := ParseSource(d.scheme, m.Source)
	if err != nil {
		return nil, err
	}
	l, err := d.listing(ctx, version)
	if err != nil {
		return nil, err
	}
	var out []mounts.File
	for _, o := range l.Objects {
		p := src.relPath(o.Key)
		if p == "" || under != "" && p != under && !hasDir(p, under) {
			continue
		}
		out = append(out, mounts.File{Path: p, ID: fileID(o), Size: o.Size})
	}
	return out, nil
}

// FetchAt copies f's pinned version into the blob store and returns its hash. The object must be
// in the listing version names, with the same ID.
func (d *Driver) FetchAt(ctx context.Context, m mounts.Mount, creds mounts.Credentials, version string, f mounts.File) (blobs.Hash, bool, error) {
	src, b, err := d.open(ctx, m.Source, creds)
	if err != nil {
		return "", false, err
	}
	l, err := d.listing(ctx, version)
	if err != nil {
		return "", false, err
	}
	for _, o := range l.Objects {
		if src.relPath(o.Key) != f.Path {
			continue
		}
		if fileID(o) != f.ID {
			return "", false, fmt.Errorf("%w: %s is %s in version %s, not %s", mounts.ErrInvalidArgument, f.Path, fileID(o), version, f.ID)
		}
		if o.Size > d.limits.MaxBytes {
			return "", false, fmt.Errorf("%w: %s is %d bytes", mounts.ErrTooLarge, f.Path, o.Size)
		}
		rc, _, err := b.Get(ctx, o.Key, &o)
		if err != nil {
			return "", false, fmt.Errorf("%s: %w", f.Path, err)
		}
		defer rc.Close()
		h, err := d.blobs.Put(ctx, io.LimitReader(rc, o.Size))
		return h, err == nil, err
	}
	return "", false, fmt.Errorf("%w: %s isn't in version %s", mounts.ErrNotFound, f.Path, version)
}

func fileID(o Object) string {
	if o.Version != "" {
		return "version:" + o.Version
	}
	return "etag:" + o.ETag
}

func hasDir(p, dir string) bool {
	return len(p) > len(dir) && p[len(dir)] == '/' && p[:len(dir)] == dir
}

func (d *Driver) open(ctx context.Context, source string, creds mounts.Credentials) (Source, Bucket, error) {
	src, err := ParseSource(d.scheme, source)
	if err != nil {
		return Source{}, nil, err
	}
	b, err := d.connect(ctx, src.Bucket, creds)
	return src, b, err
}

var errNoBlobs = errors.New("this bucket driver has no blob store for snapshot listings")

// listing reads the listing stored under version.
func (d *Driver) listing(ctx context.Context, version string) (listing, error) {
	if d.blobs == nil {
		return listing{}, fmt.Errorf("%w: %s", mounts.ErrInvalidArgument, errNoBlobs)
	}
	h, err := blobs.ParseHash(version)
	if err != nil {
		return listing{}, fmt.Errorf("%w: version %q isn't a listing hash", mounts.ErrInvalidArgument, version)
	}
	rc, err := d.blobs.Open(ctx, h)
	if err != nil {
		return listing{}, fmt.Errorf("version %s: %w", version, err)
	}
	defer rc.Close()
	var l listing
	if err := json.NewDecoder(rc).Decode(&l); err != nil {
		return listing{}, fmt.Errorf("%w: version %s isn't a bucket listing: %v", mounts.ErrInvalidArgument, version, err)
	}
	return l, nil
}

var (
	_ mounts.Preparer      = (*Driver)(nil)
	_ mounts.VersionReader = (*Driver)(nil)
)

// ContainsSource reports whether changed, an object's source from a bucket event such as
// "s3://designs/boards/jetson/README.md", is mountSource or lies under it. Both must be this
// driver's scheme and name the same bucket, and the prefix must match whole path segments.
func (d *Driver) ContainsSource(mountSource, changed string) bool {
	m, err := ParseSource(d.scheme, mountSource)
	if err != nil {
		return false
	}
	rest, ok := strings.CutPrefix(changed, d.scheme+"://")
	if !ok {
		return false
	}
	name, key, _ := strings.Cut(rest, "/")
	if name != m.Bucket {
		return false
	}
	return m.Prefix == "" || key == m.Prefix || strings.HasPrefix(key, m.Prefix+"/")
}

var _ mounts.SourceContainer = (*Driver)(nil)
