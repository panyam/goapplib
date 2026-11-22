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

	// KindAWS signs S3 requests with SigV4. With KeyRef set, it uses the key pair of
	// AccessKeyID and the secret access key at KeyRef. With neither, it uses the AWS SDK's default
	// chain, such as an EC2 instance role or an IRSA role on Kubernetes. RoleARN, when set, is a
	// role to assume first. It is a server connection only.
	KindAWS Kind = "aws"

	// KindGCP signs Google Cloud Storage requests with an OAuth token. With KeyRef set, the token
	// comes from the service account key at KeyRef. Without it, from Application Default
	// Credentials, such as workload identity on GKE. It is a server connection only.
	KindGCP Kind = "gcp"

	// KindEntra signs Microsoft Graph requests as an app registration in Microsoft Entra ID,
	// TenantID and ClientID, with an OAuth token for GraphScope. EntraAuth says how it proves
	// itself: a client secret at KeyRef, a certificate (its private key at KeyRef and the
	// certificate at CertRef), or the managed identity of the Azure host it runs on. It is a
	// server connection only.
	KindEntra Kind = "entra"

	// KindEntraUser signs Microsoft Graph requests as a person who signed in to the app's Entra
	// app registration, TenantID and ClientID. The person's token is sealed in oneauth's
	// user-token store under UserTokenProvider(ID) and their subject, and refreshed there, as for
	// KindGitHubUser. KeyRef, when set, is the app registration's client secret, which a
	// confidential client sends with each refresh.
	KindEntraUser Kind = "entra-user"
)

// holdsUserToken reports whether a connection of kind k keeps a person's token in the user-token
// store.
func (k Kind) holdsUserToken() bool { return k == KindGitHubUser || k == KindEntraUser }

// EntraAuth says how a KindEntra connection proves it is its app registration.
type EntraAuth string

const (
	// EntraSecret sends the client secret at KeyRef.
	EntraSecret EntraAuth = "secret"

	// EntraCertificate signs a client assertion (RFC 7523) with the private key at KeyRef. The
	// certificate at CertRef names the key to Entra, through the assertion's x5t header.
	EntraCertificate EntraAuth = "certificate"

	// EntraManagedIdentity asks the Azure host's managed identity endpoint for a token, so the
	// record holds no secret. ClientID, when set, picks a user-assigned identity.
	EntraManagedIdentity EntraAuth = "managed-identity"
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
	// ID names the connection in mount records, beside Owner. It follows the mount name rule.
	// IDs are unique per owner, so two people can each keep a connection named "gh", and a server
	// connection can share an ID with a person's.
	ID string

	Kind  Kind
	Scope Scope

	// Owner is the subject of the person a ScopePerson connection belongs to, as the app's
	// auth knows them. It is empty for ScopeServer. A connection's key is Owner and ID together.
	Owner string

	// Name is what a settings page shows, such as "acme org (App)".
	Name string

	// BaseURL is the service's API root. Empty means https://api.github.com. GitHub Enterprise
	// and tests set it. For KindEntra and KindEntraUser it is the Entra authority host instead,
	// and empty means https://login.microsoftonline.com.
	BaseURL string

	// AppID and InstallationID identify a KindGitHubApp installation.
	AppID          string
	InstallationID int64

	// KeyRef is a secret reference to a KindGitHubApp's private key, a KindAWS secret access
	// key, a KindGCP service account key (its JSON file), or a KindEntra or KindEntraUser
	// client secret, or a KindEntra certificate's private key.
	KeyRef string

	// AccessKeyID is a KindAWS access key's ID. AWS doesn't treat it as a secret, so the record
	// keeps it as it is. It is set exactly when KeyRef is.
	AccessKeyID string

	// RoleARN is a role a KindAWS connection assumes before signing, such as
	// "arn:aws:iam::123456789012:role/reader". Empty means none.
	RoleARN string

	// Region is a KindAWS connection's region, such as "us-east-1". Empty leaves it to the
	// driver.
	Region string

	// TenantID is a KindEntra or KindEntraUser connection's Entra tenant, as a GUID or a domain
	// such as "contoso.onmicrosoft.com".
	TenantID string

	// ClientID is a KindEntra or KindEntraUser connection's app registration (its application
	// ID), or a user-assigned managed identity's client ID.
	ClientID string

	// EntraAuth says how a KindEntra connection signs in.
	EntraAuth EntraAuth

	// CertRef is where a KindEntra EntraCertificate connection's certificate is, as a secret
	// reference to its PEM. The certificate isn't secret, but it sits beside the key it names.
	CertRef string

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

// UserTokenProvider is the provider key a KindGitHubUser or KindEntraUser connection's token is
// kept under in oneauth's user-token store, beside the owner's subject. Keying by connection, not
// by "github", lets one person hold two GitHub connections, such as github.com and an Enterprise
// host.
func UserTokenProvider(connectionID string) string {
	return "goapplib-connection:" + connectionID
}

// Store keeps connection records, keyed by owner and ID. An empty owner names a ScopeServer
// connection. Several processes may share one store.
type Store interface {
	// GetConnection returns owner's record with that ID, or ErrNotFound. Another owner's record
	// with the same ID is also ErrNotFound.
	GetConnection(ctx context.Context, owner, id string) (Connection, error)

	// ListConnections returns the connections owned by owner, in ID order. An empty owner lists
	// the ScopeServer ones.
	ListConnections(ctx context.Context, owner string) ([]Connection, error)

	// PutConnection creates c, or replaces the record with c.Owner and c.ID. It calls Validate
	// first.
	PutConnection(ctx context.Context, c Connection) error

	// DeleteConnection removes owner's record with that ID. Deleting a missing record is not an
	// error. Revoking a person's token and deleting it from the user-token store is the caller's
	// job, before this.
	DeleteConnection(ctx context.Context, owner, id string) error
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
