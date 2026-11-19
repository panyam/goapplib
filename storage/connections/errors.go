package connections

import "errors"

// The errors a store and the token-source call return, wrapped with detail. An app maps them
// onto its own transport with errors.Is. None of them carries a secret.
var (
	// ErrNotFound means no connection has that ID.
	ErrNotFound = errors.New("connections: not found")

	// ErrInvalid means a record fails Validate: a missing field, a field its Kind doesn't use,
	// or a secret reference that doesn't parse.
	ErrInvalid = errors.New("connections: invalid record")

	// ErrNotOwner means a ScopePerson connection was asked for a token by a request whose
	// viewer isn't its Owner, or by a request with no viewer.
	ErrNotOwner = errors.New("connections: viewer does not own this connection")

	// ErrReconnect means a person's token can't be refreshed, so they have to connect again.
	// It wraps oneauth's outbound.ErrReconnect.
	ErrReconnect = errors.New("connections: connect again")
)
