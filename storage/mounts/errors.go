package mounts

import "errors"

// The errors a driver, a record store and the registry return, wrapped with detail. An app maps
// them onto its own transport with errors.Is. The mounts design page has the table agni uses.
var (
	// ErrNotFound means no mount has that name, or no file has that path in the mount, or the
	// source (a repo, a ref) doesn't exist upstream.
	ErrNotFound = errors.New("mounts: not found")

	// ErrExists means a mount with that name exists already, fixed or runtime.
	ErrExists = errors.New("mounts: already exists")

	// ErrInvalidArgument means a name, a source or a ref is malformed, or a request asked for
	// something the driver doesn't do, such as a mode it doesn't support.
	ErrInvalidArgument = errors.New("mounts: invalid argument")

	// ErrInvalidPath means a path is absolute, climbs out of its mount, or names a link.
	ErrInvalidPath = errors.New("mounts: invalid path")

	// ErrFixed means the request would change a mount that comes from the server's
	// configuration, which only a restart with new configuration changes.
	ErrFixed = errors.New("mounts: mount is fixed")

	// ErrTooLarge means a snapshot or a write would pass a Limits cap.
	ErrTooLarge = errors.New("mounts: over the size or file-count limit")

	// ErrUnavailable means the source failed or refused for now: a network error, a 5xx, or a
	// rate limit. Retrying later may work.
	ErrUnavailable = errors.New("mounts: source unavailable")

	// ErrConflict means a write's IfMatch didn't match the file's current version, because
	// someone else changed or created it first. Read it again and retry.
	ErrConflict = errors.New("mounts: version conflict")

	// ErrReadOnly means the mount can't be written: it is a snapshot, or its driver has no
	// Writer.
	ErrReadOnly = errors.New("mounts: mount is read-only")

	// ErrNotLocal means the mount has no folder on this server's disk, so it has no host path.
	ErrNotLocal = errors.New("mounts: no host path")

	// ErrInUse means the app's in-use check says records still point into the mount, so it
	// can't be deleted.
	ErrInUse = errors.New("mounts: mount is in use")
)
