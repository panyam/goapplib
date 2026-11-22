package mounts

import (
	"context"
	"fmt"

	"github.com/panyam/goapplib/storage/blobs"
)

// VersionReader is a Driver that reads a mount's source at any version, not only the one a
// snapshot mount pins (goapplib issue 172). An app links a record to a file at a commit with it,
// and later moves the link to a newer commit, downloading only the files that changed. agni's
// linked artifacts are the first user.
type VersionReader interface {
	Driver

	// Moves reports whether ref names something that moves, so a later sync should follow it: an
	// empty ref, a branch, or any ref of a source with no history. A tag or a commit doesn't move.
	Moves(ctx context.Context, source, ref string, creds Credentials) (bool, error)

	// ListAt returns the regular files at version that are at or under the path under, sorted by
	// path. An empty under is the mount's root. It leaves out links, submodules and anything else
	// that isn't a regular file, and returns none for a path that names nothing. Past the
	// driver's Limits it returns ErrTooLarge. version is what Resolve returned, empty for a source
	// with no history.
	ListAt(ctx context.Context, m Mount, creds Credentials, version, under string) ([]File, error)

	// FetchAt puts f's contents at version in the driver's blob store and returns their hash.
	// fetched is false when the store already held them and nothing was downloaded. A download
	// that doesn't match f.ID returns ErrUnavailable and keeps nothing.
	FetchAt(ctx context.Context, m Mount, creds Credentials, version string, f File) (h blobs.Hash, fetched bool, err error)
}

// File is one file a VersionReader lists. Path is relative to the mount's root, clean and
// slash-separated. ID is the source's own name for the contents, such as a git blob id, and is
// empty when the source has none. Size is in bytes.
type File struct {
	Path string
	ID   string
	Size int64
}

// ResolveAt resolves ref against the source of the mount named name, through its connection. It
// returns the version, empty for a source with no history, and whether ref moves.
func (r *Registry) ResolveAt(ctx context.Context, name, ref string) (version string, moves bool, err error) {
	m, vr, creds, err := r.versions(ctx, name)
	if err != nil {
		return "", false, err
	}
	if version, err = vr.Resolve(ctx, m.Source, ref, creds); err != nil {
		return "", false, err
	}
	if moves, err = vr.Moves(ctx, m.Source, ref, creds); err != nil {
		return "", false, err
	}
	return version, moves, nil
}

// ListAt lists the files at version under the path under, in the mount named name.
func (r *Registry) ListAt(ctx context.Context, name, version, under string) ([]File, error) {
	m, vr, creds, err := r.versions(ctx, name)
	if err != nil {
		return nil, err
	}
	if under, err = CleanPath(under); err != nil {
		return nil, err
	}
	return vr.ListAt(ctx, m, creds, version, under)
}

// FetchAt keeps f's contents at version, in the mount named name, in the driver's blob store.
func (r *Registry) FetchAt(ctx context.Context, name, version string, f File) (blobs.Hash, bool, error) {
	m, vr, creds, err := r.versions(ctx, name)
	if err != nil {
		return "", false, err
	}
	if f.Path, err = CleanPath(f.Path); err != nil {
		return "", false, err
	}
	if f.Path == "" {
		return "", false, fmt.Errorf("%w: the mount's root isn't a file", ErrInvalidPath)
	}
	return vr.FetchAt(ctx, m, creds, version, f)
}

// versions finds the mount named name, its driver as a VersionReader, and its credentials. A
// driver that can't read versions returns ErrInvalidArgument.
func (r *Registry) versions(ctx context.Context, name string) (Mount, VersionReader, Credentials, error) {
	m, err := r.Get(ctx, name)
	if err != nil {
		return Mount{}, nil, nil, err
	}
	d, err := r.driver(m)
	if err != nil {
		return Mount{}, nil, nil, err
	}
	vr, ok := d.(VersionReader)
	if !ok {
		return Mount{}, nil, nil, fmt.Errorf("%w: %s mounts can't be read at a version", ErrInvalidArgument, m.Kind)
	}
	creds, err := r.credentials(ctx, m)
	if err != nil {
		return Mount{}, nil, nil, err
	}
	return m, vr, creds, nil
}
