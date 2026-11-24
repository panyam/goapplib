package mounts

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"sync"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goutils/mountfs"
)

// Registry is an app's one view of its mounts. It joins the fixed mounts from the server's
// configuration, the runtime mounts in a RecordStore, and the drivers that read them, and it asks
// for each mount's credentials when a driver needs them.
//
// Several registries, in one process or several, can share a RecordStore. Each checks the store's
// Generation at most once per recheck interval (a second by default), and lists the store again
// only when it changed. A registry sees its own creates and deletes at once.
//
// A Registry is safe for concurrent use.
type Registry struct {
	store   RecordStore
	drivers map[Kind]Driver
	fixed   map[string]Mount
	creds   CredentialsFunc
	inUse   func(ctx context.Context, name string) (bool, error)
	limits  Limits
	every   time.Duration
	now     func() time.Time

	mu      sync.Mutex
	runtime []Mount
	gen     string
	loaded  bool
	checked time.Time
}

// Option configures a Registry.
type Option func(*Registry)

// Preparer is a Driver with work to do when a mount is made, after Resolve. A GitHub snapshot
// downloads the commit's files into the blob store there, and sets the mount's Manifest. The
// registry stores the Mount that Prepare returns. A Prepare error fails the Create, and no record
// is written.
type Preparer interface {
	Driver
	Prepare(ctx context.Context, m Mount, creds Credentials) (Mount, error)
}

// NewRegistry returns a registry over store, serving the kinds of drivers. Two drivers of one kind
// keep the later one. A nil store serves the fixed mounts alone, for an app with no store of its
// own, such as agni's CLI. Create and Delete then return ErrInvalidArgument.
func NewRegistry(store RecordStore, drivers []Driver, opts ...Option) *Registry {
	r := &Registry{store: store, drivers: map[Kind]Driver{}, fixed: map[string]Mount{}, every: time.Second, now: time.Now}
	for _, d := range drivers {
		r.drivers[d.Kind()] = d
	}
	for _, o := range opts {
		o(r)
	}
	if r.limits.MaxBytes <= 0 {
		r.limits.MaxBytes = DefaultLimits.MaxBytes
	}
	if r.limits.MaxFiles <= 0 {
		r.limits.MaxFiles = DefaultLimits.MaxFiles
	}
	return r
}

// WithFixed adds mounts from the server's configuration, such as agni's --mount flags. They can't
// be deleted, and a runtime record with the same name is ignored. They are the only way to get a
// KindFolder mount.
func WithFixed(ms ...Mount) Option {
	return func(r *Registry) {
		for _, m := range ms {
			r.fixed[m.Name] = m
		}
	}
}

// WithCredentials sets how the registry gets a mount's Credentials from its ConnectionOwner and
// Connection. The connections service's Credentials method fits it. It is called with the
// request's context, so a person's connection sees who is asking. A mount with an empty
// Connection gets nil Credentials without a call.
func WithCredentials(f CredentialsFunc) Option {
	return func(r *Registry) { r.creds = f }
}

// WithInUse sets the app's check for whether its records still point into a mount. Delete calls
// it, and returns ErrInUse when it says true.
func WithInUse(f func(ctx context.Context, name string) (bool, error)) Option {
	return func(r *Registry) { r.inUse = f }
}

// WithLimits caps writes through the registry. A zero field keeps DefaultLimits' value. Drivers
// that snapshot take their own Limits.
func WithLimits(l Limits) Option {
	return func(r *Registry) { r.limits = l }
}

// WithRecheck sets how often the registry asks the store whether another process changed it. The
// default is a second. Tests set it lower.
func WithRecheck(d time.Duration) Option {
	return func(r *Registry) { r.every = d }
}

// List returns every mount, fixed and runtime, in name order. When the store fails after a good
// read, List keeps returning the last good list rather than dropping every runtime mount.
func (r *Registry) List(ctx context.Context) ([]Mount, error) {
	runtime, err := r.refresh(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Mount, 0, len(r.fixed)+len(runtime))
	for _, m := range r.fixed {
		out = append(out, m)
	}
	for _, m := range runtime {
		if _, ok := r.fixed[m.Name]; !ok {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns the mount named name, or ErrNotFound.
func (r *Registry) Get(ctx context.Context, name string) (Mount, error) {
	if m, ok := r.fixed[name]; ok {
		return m, nil
	}
	runtime, err := r.refresh(ctx)
	if err != nil {
		return Mount{}, err
	}
	for _, m := range runtime {
		if m.Name == name {
			return m, nil
		}
	}
	return Mount{}, fmt.Errorf("%w: mount %q", ErrNotFound, name)
}

// Create makes a runtime mount from m's Name, Kind, Source, Ref, Mode, Connection and
// ConnectionOwner. An empty Mode takes the driver's first. Create resolves Ref through the driver,
// calls Prepare when the driver has it, sets Version and Created, and writes the record. It
// returns ErrInvalidArgument for a bad name, an unknown kind, a mode the driver doesn't serve,
// KindFolder, or a ConnectionOwner with no Connection. A name taken by a fixed or runtime mount
// returns ErrExists. Driver errors pass through, such as ErrNotFound for a
// source that doesn't exist.
func (r *Registry) Create(ctx context.Context, m Mount) (Mount, error) {
	if r.store == nil {
		return Mount{}, fmt.Errorf("%w: this registry has no record store, so it keeps no runtime mounts", ErrInvalidArgument)
	}
	if err := ValidName(m.Name); err != nil {
		return Mount{}, err
	}
	if m.Kind == KindFolder {
		return Mount{}, fmt.Errorf("%w: folder mounts come from the server's configuration", ErrInvalidArgument)
	}
	if m.ConnectionOwner != "" && m.Connection == "" {
		return Mount{}, fmt.Errorf("%w: mount %q names a connection owner but no connection", ErrInvalidArgument, m.Name)
	}
	d, err := r.driver(m)
	if err != nil {
		return Mount{}, err
	}
	if m.Mode == "" && len(d.Modes()) > 0 {
		m.Mode = d.Modes()[0]
	}
	if !serves(d, m.Mode) {
		return Mount{}, fmt.Errorf("%w: %s mounts don't support mode %q", ErrInvalidArgument, m.Kind, m.Mode)
	}
	if _, ok := r.fixed[m.Name]; ok {
		return Mount{}, fmt.Errorf("%w: mount %q comes from the server's configuration", ErrExists, m.Name)
	}
	creds, err := r.credentials(ctx, m)
	if err != nil {
		return Mount{}, err
	}
	if m.Version, err = d.Resolve(ctx, m.Source, m.Ref, creds); err != nil {
		return Mount{}, err
	}
	m.Created = r.now().UTC()
	m.Manifest = ""
	if p, ok := d.(Preparer); ok {
		if m, err = p.Prepare(ctx, m, creds); err != nil {
			return Mount{}, err
		}
	}
	if err := r.store.CreateMount(ctx, m); err != nil {
		return Mount{}, err
	}
	r.invalidate()
	return m, nil
}

// Delete removes the runtime mount named name. A fixed mount returns ErrFixed, a missing one
// ErrNotFound, and one the in-use check says is still referenced ErrInUse. Delete removes only the
// record. Blobs a snapshot used stay until a blob sweep finds nothing naming them.
func (r *Registry) Delete(ctx context.Context, name string) error {
	if _, ok := r.fixed[name]; ok {
		return fmt.Errorf("%w: mount %q comes from the server's configuration", ErrFixed, name)
	}
	if _, err := r.Get(ctx, name); err != nil {
		return err
	}
	if r.inUse != nil {
		used, err := r.inUse(ctx, name)
		if err != nil {
			return err
		}
		if used {
			return fmt.Errorf("%w: records still point into mount %q", ErrInUse, name)
		}
	}
	if r.store == nil {
		return fmt.Errorf("%w: this registry has no record store, so it keeps no runtime mounts", ErrInvalidArgument)
	}
	if err := r.store.DeleteMount(ctx, name); err != nil {
		return err
	}
	r.invalidate()
	return nil
}

// Open opens the file or folder u names.
func (r *Registry) Open(ctx context.Context, u URI) (fs.File, error) {
	fsys, err := r.open(ctx, u.Mount)
	if err != nil {
		return nil, err
	}
	name := u.Path
	if name == "" {
		name = "."
	}
	return fsys.Open(name)
}

// FS returns every mount as one fs.FS, each under its name, through goutils' mountfs. It lists the
// mounts once, when called, and opens each mount's driver when a path first reaches it. The context
// is kept for those opens, so the viewer it carries is the one whose connections are used. An app
// calls FS per request, or per job with that job's context.
func (r *Registry) FS(ctx context.Context) fs.FS {
	ms, _ := r.List(ctx)
	out := mountfs.New()
	for _, m := range ms {
		out.Mount(m.Name, &lazyFS{r: r, ctx: ctx, name: m.Name})
	}
	return out
}

// HostPath returns the host path of the file u names, for code that needs a real file. A mount
// whose driver isn't a Localizer returns ErrNotLocal. A FetchingLocalizer that Fetches the mount
// gets its credentials, since it may download the files first.
func (r *Registry) HostPath(ctx context.Context, u URI) (string, error) {
	m, err := r.Get(ctx, u.Mount)
	if err != nil {
		return "", err
	}
	d, err := r.driver(m)
	if err != nil {
		return "", err
	}
	if fl, ok := d.(FetchingLocalizer); ok && fl.Fetches(m) {
		creds, err := r.credentials(ctx, m)
		if err != nil {
			return "", err
		}
		return fl.HostPathWith(ctx, m, creds, u.Path)
	}
	l, ok := d.(Localizer)
	if !ok {
		return "", fmt.Errorf("%w: %s mount %q", ErrNotLocal, m.Kind, m.Name)
	}
	return l.HostPath(m, u.Path)
}

// Stat returns the current version of the file u names, for a later Put or Remove. A mount that
// can't be written returns ErrReadOnly.
func (r *Registry) Stat(ctx context.Context, u URI) (FileVersion, error) {
	m, w, creds, err := r.writer(ctx, u.Mount)
	if err != nil {
		return "", err
	}
	return w.Stat(ctx, m, creds, u.Path)
}

// Put writes a file in the mount named name, with req.Path cleaned by CleanPath. A snapshot mount,
// or one whose driver isn't a Writer, returns ErrReadOnly. Data over the registry's MaxBytes
// returns ErrTooLarge. The IfMatch rules are mounts.PutRequest's.
func (r *Registry) Put(ctx context.Context, name string, req PutRequest) (*WriteResult, error) {
	m, w, creds, err := r.writer(ctx, name)
	if err != nil {
		return nil, err
	}
	if req.Path, err = CleanPath(req.Path); err != nil {
		return nil, err
	}
	if req.Path == "" {
		return nil, fmt.Errorf("%w: the mount's root isn't a file", ErrInvalidPath)
	}
	if int64(len(req.Data)) > r.limits.MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes, the limit is %d", ErrTooLarge, len(req.Data), r.limits.MaxBytes)
	}
	return w.Put(ctx, m, creds, req)
}

// Remove deletes a file in the mount named name, under the same rules as Put.
func (r *Registry) Remove(ctx context.Context, name string, req DeleteRequest) (*WriteResult, error) {
	m, w, creds, err := r.writer(ctx, name)
	if err != nil {
		return nil, err
	}
	if req.Path, err = CleanPath(req.Path); err != nil {
		return nil, err
	}
	if req.Path == "" {
		return nil, fmt.Errorf("%w: the mount's root isn't a file", ErrInvalidPath)
	}
	return w.Delete(ctx, m, creds, req)
}

func (r *Registry) refresh(ctx context.Context) ([]Mount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.store == nil {
		return nil, nil
	}
	if r.loaded && r.now().Sub(r.checked) < r.every {
		return r.runtime, nil
	}
	g, err := r.store.Generation(ctx)
	if err == nil && r.loaded && g == r.gen {
		r.checked = r.now()
		return r.runtime, nil
	}
	var list []Mount
	if err == nil {
		list, err = r.store.ListMounts(ctx)
	}
	if err != nil {
		if r.loaded {
			return r.runtime, nil
		}
		return nil, err
	}
	r.runtime, r.gen, r.loaded, r.checked = list, g, true, r.now()
	return r.runtime, nil
}

func (r *Registry) invalidate() {
	r.mu.Lock()
	r.checked = time.Time{}
	r.mu.Unlock()
}

func (r *Registry) driver(m Mount) (Driver, error) {
	d, ok := r.drivers[m.Kind]
	if !ok {
		return nil, fmt.Errorf("%w: no driver for kind %q", ErrInvalidArgument, m.Kind)
	}
	return d, nil
}

func (r *Registry) credentials(ctx context.Context, m Mount) (Credentials, error) {
	if m.Connection == "" {
		return nil, nil
	}
	if r.creds == nil {
		return nil, fmt.Errorf("%w: mount %q names connection %q, and the registry has no credentials", ErrInvalidArgument, m.Name, m.Connection)
	}
	return r.creds(ctx, m.ConnectionOwner, m.Connection)
}

func (r *Registry) open(ctx context.Context, name string) (fs.FS, error) {
	m, err := r.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	d, err := r.driver(m)
	if err != nil {
		return nil, err
	}
	creds, err := r.credentials(ctx, m)
	if err != nil {
		return nil, err
	}
	return d.Open(ctx, m, creds)
}

func (r *Registry) writer(ctx context.Context, name string) (Mount, Writer, Credentials, error) {
	m, err := r.Get(ctx, name)
	if err != nil {
		return Mount{}, nil, nil, err
	}
	if m.Mode != ModeLive {
		return Mount{}, nil, nil, fmt.Errorf("%w: mount %q is a snapshot", ErrReadOnly, name)
	}
	d, err := r.driver(m)
	if err != nil {
		return Mount{}, nil, nil, err
	}
	w, ok := d.(Writer)
	if !ok {
		return Mount{}, nil, nil, fmt.Errorf("%w: %s mounts can't be written", ErrReadOnly, m.Kind)
	}
	creds, err := r.credentials(ctx, m)
	if err != nil {
		return Mount{}, nil, nil, err
	}
	return m, w, creds, nil
}

func serves(d Driver, mode Mode) bool {
	for _, m := range d.Modes() {
		if m == mode {
			return true
		}
	}
	return false
}

// lazyFS opens a mount's driver on first use, so FS doesn't open every mount up front.
type lazyFS struct {
	r    *Registry
	ctx  context.Context
	name string

	once sync.Once
	fsys fs.FS
	err  error
}

func (l *lazyFS) Open(name string) (fs.File, error) {
	l.once.Do(func() { l.fsys, l.err = l.r.open(l.ctx, l.name) })
	if l.err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: l.err}
	}
	return l.fsys.Open(name)
}

// BlobRefs returns a blobs.Referrer that names, for every snapshot mount, its manifest and every
// file the manifest lists, so a sweep of store keeps them. Pass it to store's Sweep beside the
// app's own referrers. A manifest that can't be read fails the referrer, and so the sweep, rather
// than letting the sweep remove a snapshot's files. The registry rereads its records at most once
// per recheck interval, so a sweep's grace period should be longer than that.
func (r *Registry) BlobRefs(store blobs.Store) blobs.Referrer {
	return func(ctx context.Context, keep func(blobs.Hash)) error {
		ms, err := r.List(ctx)
		if err != nil {
			return err
		}
		for _, m := range ms {
			if m.Manifest == "" {
				continue
			}
			if err := blobs.ManifestRefs(ctx, store, blobs.Hash(m.Manifest), keep); err != nil {
				return fmt.Errorf("mount %q: %w", m.Name, err)
			}
		}
		return nil
	}
}
