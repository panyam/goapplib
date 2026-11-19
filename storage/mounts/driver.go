package mounts

import (
	"context"
	"io/fs"

	"golang.org/x/oauth2"
)

// Driver reads one Kind of source. The registry calls it with the token source of the mount's
// connection, or nil for a public or local source. A driver doesn't know about connections or
// viewers. It sends the token it is given.
type Driver interface {
	// Kind is the Kind of mount this driver serves.
	Kind() Kind

	// Modes lists the modes the driver supports. A request for any other returns
	// ErrInvalidArgument.
	Modes() []Mode

	// Resolve checks source and turns ref into a version: a commit for GitHub, an empty string
	// for a folder. An empty ref means the source's default, such as the default branch. A
	// source or ref that doesn't exist returns ErrNotFound.
	Resolve(ctx context.Context, source, ref string, ts oauth2.TokenSource) (version string, err error)

	// Open returns the mount's files. For a snapshot mount they are the files at m.Version, and
	// the same call returns the same bytes for the life of the mount. For a live mount they are
	// the source's latest. Paths in the fs.FS are clean, relative and slash-separated, so a
	// URI's Path opens directly. Only regular files and folders appear in it, never links.
	Open(ctx context.Context, m Mount, ts oauth2.TokenSource) (fs.FS, error)
}

// Writer is a Driver that can change a live mount. A write to a snapshot mount returns
// ErrReadOnly before reaching the driver.
type Writer interface {
	Driver

	// Stat returns the current version of the file at p, which a later Put or Delete passes as
	// IfMatch. For GitHub it is the file's blob sha. A missing file returns ErrNotFound.
	Stat(ctx context.Context, m Mount, ts oauth2.TokenSource, p string) (FileVersion, error)

	// Put creates or replaces the file at req.Path. A GitHub Put is one commit.
	Put(ctx context.Context, m Mount, ts oauth2.TokenSource, req PutRequest) (*WriteResult, error)

	// Delete removes the file at req.Path. A GitHub Delete is one commit.
	Delete(ctx context.Context, m Mount, ts oauth2.TokenSource, req DeleteRequest) (*WriteResult, error)
}

// FileVersion identifies one version of one file, in the driver's own format.
type FileVersion string

// AnyVersion as IfMatch lets a write replace whatever version is there, or create the file.
const AnyVersion FileVersion = "*"

// PutRequest writes Data to Path. IfMatch guards against overwriting a change the caller hasn't
// seen. Empty means the file must not exist yet. AnyVersion means write regardless. Anything
// else must equal the file's current version. A mismatch returns ErrConflict.
type PutRequest struct {
	Path    string
	Data    []byte
	IfMatch FileVersion

	// Message is the commit message, for drivers that commit. Others ignore it.
	Message string
}

// DeleteRequest removes Path. IfMatch must be the file's current version, or AnyVersion.
type DeleteRequest struct {
	Path    string
	IfMatch FileVersion
	Message string
}

// WriteResult is what a write produced.
type WriteResult struct {
	// File is the file's new version. It is empty after a Delete.
	File FileVersion

	// Mount is the mount's new version, such as the commit a GitHub write made. Drivers whose
	// sources have no version across files leave it empty.
	Mount string
}

// Localizer is a Driver whose mounts can have a folder on this server's disk, for code that
// needs a host path, such as a native tool. A snapshot driver's view counts too.
type Localizer interface {
	Driver

	// HostPath returns the host path of p inside m. It returns ErrNotLocal when m has no
	// folder here, and ErrInvalidPath when p would leave the mount.
	HostPath(m Mount, p string) (string, error)
}

// Limits caps what a snapshot or a write may hold. A driver returns ErrTooLarge past either.
// Zero means the default.
type Limits struct {
	MaxBytes int64
	MaxFiles int
}

// DefaultLimits are agni's caps for a GitHub snapshot: 256 MB and 100,000 files.
var DefaultLimits = Limits{MaxBytes: 256 << 20, MaxFiles: 100_000}
