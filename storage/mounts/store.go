package mounts

import "context"

// RecordStore keeps the runtime mounts, the ones made by requests rather than by the server's
// configuration. Records are created and deleted, never changed in place. A live mount's latest
// version comes from its driver, not from its record.
//
// Several processes may share one store, such as a server and a worker. Each notices the others'
// changes through Generation.
type RecordStore interface {
	// ListMounts returns every record, in name order.
	ListMounts(ctx context.Context) ([]Mount, error)

	// GetMount returns the record named name, or ErrNotFound.
	GetMount(ctx context.Context, name string) (Mount, error)

	// CreateMount adds m. A record with m.Name returns ErrExists, so of two processes creating
	// the same name, one wins.
	CreateMount(ctx context.Context, m Mount) error

	// DeleteMount removes the record named name. Deleting a missing record is not an error.
	DeleteMount(ctx context.Context, name string) error

	// Generation returns a value that changes whenever any process creates or deletes a record.
	// The registry compares it with the last one it read, and lists again only when they differ.
	// The format is the store's own.
	Generation(ctx context.Context) (string, error)
}
