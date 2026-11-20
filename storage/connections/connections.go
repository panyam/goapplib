// Package connections names the sign-ins a server holds for third-party services, so a mount over
// a private repo can read it. Several mounts can share one connection, such as one GitHub App
// installation over many repos. A connection signs in either as the server or for one person.
//
// A Connection record never holds a secret. It names where the secret is, as an oneauth secret
// reference ("env:NAME" or "file:/abs/path"), or, for a person's own token, keeps it sealed in
// oneauth's user-token store. The design and its threat note are at
// https://panyam.github.io/goapplib/reference/connections/.
package connections

import (
	"context"
	"time"
)

// Kind says how a connection signs in.
type Kind string

const (
	// KindGitHubApp signs in as a GitHub App installation. The server mints a JWT from the
	// App's private key (KeyRef) and trades it for an hour-long installation token.
	KindGitHubApp Kind = "github-app"

	// KindGitHubToken signs in with a fixed token, such as a personal access token, read from
	// TokenRef.
	KindGitHubToken Kind = "github-token"

	// KindGitHubUser signs in as a person who connected their own GitHub account through OAuth.
	// The token is sealed in oneauth's user-token store under UserTokenProvider(ID) and the
	// person's subject, and refreshed there.
	KindGitHubUser Kind = "github-user"
)

// Scope says whose sign-in a connection is.
type Scope string

const (
	// ScopeServer signs in as the server. Anyone the app lets read a mount reads it through the
	// connection, so the app's own sharing rules decide who sees what.
	ScopeServer Scope = "server"

	// ScopePerson signs in for Owner alone. Only a request whose viewer is Owner gets a token,
	// so GitHub decides what that person can read.
	ScopePerson Scope = "person"
)

// Status records the last thing the server learned about a connection's sign-in.
type Status string

const (
	// StatusOK means the last token request worked, or none has been made.
	StatusOK Status = "ok"

	// StatusReconnect means the person's token can't be refreshed, so they have to connect
	// again. It follows oneauth's outbound.ErrReconnect.
	StatusReconnect Status = "reconnect"

	// StatusFailed means the last token request failed for another reason, in LastError.
	StatusFailed Status = "failed"
)

// Connection is the record of one sign-in. Which fields apply depends on Kind, and Validate
// checks them.
type Connection struct {
	// ID names the connection in mount records. It follows the mount name rule.
	ID string

	Kind  Kind
	Scope Scope

	// Owner is the subject of the person a ScopePerson connection belongs to, as the app's
	// auth knows them. It is empty for ScopeServer.
	Owner string

	// Name is what a settings page shows, such as "acme org (App)".
	Name string

	// BaseURL is the service's API root. Empty means https://api.github.com. GitHub Enterprise
	// and tests set it.
	BaseURL string

	// AppID and InstallationID identify a KindGitHubApp installation.
	AppID          string
	InstallationID int64

	// KeyRef is where a KindGitHubApp's private key is, as a secret reference.
	KeyRef string

	// TokenRef is where a KindGitHubToken's token is, as a secret reference.
	TokenRef string

	// Pasted is set on a KindGitHubUser connection when the person pasted the stored token, so
	// the app's OAuth flow never issued it. Disconnect then deletes the token without revoking
	// it, since GitHub refuses to revoke a token through an app that didn't issue it.
	// SavePastedToken sets it and SaveUserToken clears it.
	Pasted bool

	Status Status

	// LastError is the last failure's message, with no secret in it. It is empty when Status
	// is StatusOK.
	LastError string

	Created  time.Time
	LastUsed time.Time
}

// UserTokenProvider is the provider key a KindGitHubUser connection's token is kept under in
// oneauth's user-token store, beside the owner's subject. Keying by connection, not by "github",
// lets one person hold two GitHub connections, such as github.com and an Enterprise host.
func UserTokenProvider(connectionID string) string {
	return "goapplib-connection:" + connectionID
}

// Store keeps connection records. Several processes may share one.
type Store interface {
	// GetConnection returns the record with that ID, or ErrNotFound.
	GetConnection(ctx context.Context, id string) (Connection, error)

	// ListConnections returns the connections owned by owner, in ID order. An empty owner lists
	// the ScopeServer ones.
	ListConnections(ctx context.Context, owner string) ([]Connection, error)

	// PutConnection creates c, or replaces the record with c.ID. It calls Validate first.
	PutConnection(ctx context.Context, c Connection) error

	// DeleteConnection removes the record with that ID. Deleting a missing record is not an
	// error. Revoking a person's token and deleting it from the user-token store is the
	// caller's job, before this.
	DeleteConnection(ctx context.Context, id string) error
}

type viewerKey struct{}

// WithViewer returns ctx carrying the subject of the person a request is for. The app sets it
// from its auth, and a ScopePerson connection gives a token only when it matches Owner.
func WithViewer(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, viewerKey{}, subject)
}

// Viewer returns the subject WithViewer set, or "" for a request with no person, such as a
// background job.
func Viewer(ctx context.Context) string {
	s, _ := ctx.Value(viewerKey{}).(string)
	return s
}
