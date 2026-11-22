package bucket

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
)

// Driver serves one kind of bucket mount. It implements mounts.Driver, mounts.Preparer,
// mounts.VersionReader and mounts.Localizer. It can't write, so a live mount is read-only.
type Driver struct {
	kind    mounts.Kind
	scheme  string
	connect Connector
	blobs   blobs.Store
	limits  mounts.Limits
	views   string
	viewMu  sync.Mutex
}

// Option configures a Driver.
type Option func(*Driver)

// WithLimits caps how many objects and bytes a source may hold. A zero field keeps
// mounts.DefaultLimits' value. Past either, Resolve returns mounts.ErrTooLarge.
func WithLimits(l mounts.Limits) Option { return func(d *Driver) { d.limits = l } }

// WithViews sets the folder where HostPath builds each snapshot's view, as links to the blob store.
// It takes effect only when the store is a blobs.Linker, such as a blobs.Dir, or a blobs.Cached in
// front of a bucket store. Then Prepare fetches every file of a new snapshot into the store, so
// creating a snapshot mount downloads all of it, and Open reads the snapshot from the store
// instead of the bucket. Without it, HostPath returns mounts.ErrNotLocal.
func WithViews(dir string) Option { return func(d *Driver) { d.views = dir } }

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
// blobs.Manifest that lists the listing. With WithViews and a linking store, it also fetches each
// object at its pinned version into the store and lists it in the manifest, so the registry's
// BlobRefs keeps the files and a view can be rebuilt without the bucket. An object whose pinned
// version is gone fails as Open would, with mounts.ErrConflict. A live mount comes back
// unchanged.
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
	man := blobs.Manifest{{Path: listingPath, Hash: blobs.Hash(m.Version)}}
	if d.fetches() {
		src, b, err := d.open(ctx, m.Source, creds)
		if err != nil {
			return m, err
		}
		for _, o := range l.Objects {
			p := src.relPath(o.Key)
			if p == "" {
				continue
			}
			fh, _, err := d.fetch(ctx, src, b, p, o)
			if err != nil {
				return m, err
			}
			man = append(man, blobs.ManifestEntry{Path: filesDir + p, Hash: fh, Size: o.Size})
		}
	}
	h, err := blobs.PutManifest(ctx, d.blobs, man)
	if err != nil {
		return m, err
	}
	m.Manifest = string(h)
	return m, nil
}

// Open returns a snapshot mount's files at the versions its listing pinned, or a live mount's
// latest objects. A snapshot whose files Prepare fetched reads them from the blob store and never
// asks the bucket. Otherwise, opening a snapshot file whose pinned version is gone, or whose ETag
// changed on a bucket without versions, fails with an fs.PathError naming the file and wrapping
// mounts.ErrConflict.
func (d *Driver) Open(ctx context.Context, m mounts.Mount, creds mounts.Credentials) (fs.FS, error) {
	switch m.Mode {
	case mounts.ModeLive:
		src, b, err := d.open(ctx, m.Source, creds)
		if err != nil {
			return nil, err
		}
		return liveFS{ctx: ctx, b: b, src: src}, nil
	case mounts.ModeSnapshot:
		src, err := ParseSource(d.scheme, m.Source)
		if err != nil {
			return nil, err
		}
		l, files, err := d.pinned(ctx, m)
		if err != nil {
			return nil, err
		}
		if files != nil {
			s := newSnapshotFS(ctx, nil, src, l.Objects)
			s.store, s.hashes = d.blobs, files
			return s, nil
		}
		src, b, err := d.open(ctx, m.Source, creds)
		if err != nil {
			return nil, err
		}
		return newSnapshotFS(ctx, b, src, l.Objects), nil
	}
	return nil, fmt.Errorf("%w: %s mounts don't support mode %q", mounts.ErrInvalidArgument, d.kind, m.Mode)
}

// HostPath returns where p of a snapshot mount is on disk, in the folder WithViews set. The first
// call for a mount builds its view there, as links to the files Prepare fetched, so a restart that
// lost the folder rebuilds it from the blob store without asking the bucket. A live mount, a
// driver without WithViews or a linking store, or a snapshot made before views were set, returns
// mounts.ErrNotLocal.
func (d *Driver) HostPath(m mounts.Mount, p string) (string, error) {
	clean, err := mounts.CleanPath(p)
	if err != nil {
		return "", err
	}
	linker, ok := d.blobs.(blobs.Linker)
	if m.Mode != mounts.ModeSnapshot || d.views == "" || !ok {
		return "", fmt.Errorf("%w: %s mount %q", mounts.ErrNotLocal, d.kind, m.Name)
	}
	if err := mounts.ValidName(m.Name); err != nil {
		return "", err
	}
	v, err := blobs.ParseHash(m.Version)
	if err != nil {
		return "", fmt.Errorf("%w: snapshot version %q isn't a listing hash", mounts.ErrInvalidArgument, m.Version)
	}
	view := filepath.Join(d.views, m.Name, strings.TrimPrefix(string(v), "sha256:"))
	d.viewMu.Lock()
	defer d.viewMu.Unlock()
	err = mounts.BuildView(view, linker, func() (blobs.Manifest, error) {
		_, files, err := d.pinned(context.Background(), m)
		if err != nil {
			return nil, err
		}
		if files == nil {
			return nil, fmt.Errorf("%w: %s mount %q was made before the driver had views, so make it again", mounts.ErrNotLocal, d.kind, m.Name)
		}
		man := make(blobs.Manifest, 0, len(files))
		for p, h := range files {
			man = append(man, blobs.ManifestEntry{Path: p, Hash: h})
		}
		return man, nil
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(view, filepath.FromSlash(clean)), nil
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
// in the listing version names, with the same ID. When the store is a blobs.Aliases that maps the
// object's version id or ETag to a blob it holds, nothing is downloaded and fetched is false.
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
		return d.fetch(ctx, src, b, f.Path, o)
	}
	return "", false, fmt.Errorf("%w: %s isn't in version %s", mounts.ErrNotFound, f.Path, version)
}

// bucketAliases is the namespace an object's alias is recorded under in a blobs.Aliases store.
const bucketAliases = "bucket"

// fetch copies o's pinned version, the file at p in src, into the blob store, and reports whether
// it downloaded anything. A store that is a blobs.Aliases keeps an alias from the object's
// identity to its blob, so a later fetch of the same version skips the download.
func (d *Driver) fetch(ctx context.Context, src Source, b Bucket, p string, o Object) (blobs.Hash, bool, error) {
	al, _ := d.blobs.(blobs.Aliases)
	id := d.aliasID(src, o)
	if al != nil {
		if h, ok, err := al.Alias(ctx, bucketAliases, id); err == nil && ok {
			if has, err := d.blobs.Has(ctx, h); err == nil && has {
				return h, false, nil
			}
		}
	}
	if o.Size > d.limits.MaxBytes {
		return "", false, fmt.Errorf("%w: %s is %d bytes", mounts.ErrTooLarge, p, o.Size)
	}
	rc, _, err := b.Get(ctx, o.Key, &o)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", p, err)
	}
	defer rc.Close()
	h, err := d.blobs.Put(ctx, io.LimitReader(rc, o.Size))
	if err != nil {
		return "", false, err
	}
	if al != nil {
		if err := al.SetAlias(ctx, bucketAliases, id, h); err != nil {
			return "", false, err
		}
	}
	return h, true, nil
}

// aliasID names o's contents across every bucket the driver reads. An ETag or a version id is
// only unique within one bucket and key, so the ID hashes the scheme, the bucket and the key with
// it. Hashing also keeps the ID to the characters an alias allows.
func (d *Driver) aliasID(src Source, o Object) string {
	sum := sha256.Sum256([]byte(d.scheme + "\x00" + src.Bucket + "\x00" + o.Key + "\x00" + fileID(o)))
	return hex.EncodeToString(sum[:])
}

// fetches says whether Prepare fetches a snapshot's files, which it does when HostPath can use
// them.
func (d *Driver) fetches() bool {
	_, ok := d.blobs.(blobs.Linker)
	return d.views != "" && ok
}

// pinned reads a snapshot mount's manifest. It returns the listing, and each file's blob when
// Prepare fetched them. files is nil for a snapshot whose files stay in the bucket.
func (d *Driver) pinned(ctx context.Context, m mounts.Mount) (l listing, files map[string]blobs.Hash, err error) {
	h, err := blobs.ParseHash(m.Manifest)
	if err != nil {
		return l, nil, fmt.Errorf("%w: snapshot mount %q has no manifest", mounts.ErrInvalidArgument, m.Name)
	}
	if d.blobs == nil {
		return l, nil, fmt.Errorf("%w: %s", mounts.ErrInvalidArgument, errNoBlobs)
	}
	man, err := blobs.GetManifest(ctx, d.blobs, h)
	if err != nil {
		return l, nil, err
	}
	notListing := fmt.Errorf("%w: snapshot mount %q's manifest isn't a bucket listing", mounts.ErrInvalidArgument, m.Name)
	var lh blobs.Hash
	files = map[string]blobs.Hash{}
	for _, e := range man {
		if e.Path == listingPath {
			lh = e.Hash
			continue
		}
		p, ok := strings.CutPrefix(e.Path, filesDir)
		if !ok {
			return l, nil, notListing
		}
		files[p] = e.Hash
	}
	if lh == "" {
		return l, nil, notListing
	}
	if l, err = d.listing(ctx, string(lh)); err != nil {
		return l, nil, err
	}
	src, err := ParseSource(d.scheme, l.Source)
	if err != nil {
		return l, nil, err
	}
	listed := 0
	for _, o := range l.Objects {
		if p := src.relPath(o.Key); p != "" {
			listed++
			if _, ok := files[p]; !ok && len(files) > 0 {
				return l, nil, fmt.Errorf("%w: snapshot mount %q's manifest has no blob for %s", mounts.ErrInvalidArgument, m.Name, p)
			}
		}
	}
	if len(files) == 0 && listed > 0 {
		return l, nil, nil
	}
	return l, files, nil
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
	_ mounts.Localizer     = (*Driver)(nil)
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
