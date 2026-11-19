---
title: "Connections"
description: "The design of the storage module's connections: server-held sign-ins to GitHub, as the server or for one person, built on oneauth's outbound token sources, with where a secret can and can't end up."
---

Picture an agni server with three private repos mounted from one org. The org installed agni's GitHub App, so one installation covers all three repos, and the server signs in as that installation. Now an engineer mounts a repo of their own with a personal token, and a notation user connects their own GitHub account so notation can commit for them. Each of those is a connection: a sign-in the server holds for a third-party service. A mount names one by ID, and the driver reads through it.

This page is the design for [#153](https://github.com/panyam/goapplib/issues/153). The `storage/connections` package holds the record, its validation and the store interface, rendered at the bottom of the page from the source. The service that builds token sources comes in the implementation ticket. The tokens themselves come from oneauth's `outbound` package, from v0.3.6 ([oneauth#467](https://github.com/panyam/oneauth/issues/467)). This module keeps what oneauth leaves out on purpose: the record, its scope, which mounts use it, and which token source it builds. [Mounts](../mounts/) covers the other side.

## Kinds and scopes

| Kind | Scope | Signs in with | oneauth piece | First user |
|---|---|---|---|---|
| `github-app` | server | a JWT from the App's key, traded for an hour-long installation token | `outbound.GitHubAppInstallation` | agni, an org's repos |
| `github-token` | server or person | a fixed token, such as a personal access token | `outbound.Static` | agni, one engineer's repos |
| `github-user` | person | the person's own OAuth token, sealed and refreshed | `outbound.UserTokens` | notation |

- **Server scope.** Anyone the app lets read a mount reads it through the connection. So the app's own sharing decides who sees what. agni#1728 calls this out for links made through the server's identity.
- **Person scope.** The connection gives a token only to a request whose viewer is its `Owner`. The app sets the viewer with `WithViewer` from its own auth, and a request with no viewer, or a different one, gets `ErrNotOwner`. So GitHub decides what that person can read.

Later kinds are the server's own cloud identity for S3 and GCS (agni#1730), and Entra for SharePoint (agni#1729). They add a `Kind` and a token source, and the record shape holds them.

## The record

`Connection` keeps the ID, kind, scope and owner, a display name, the API base URL, and the fields for its kind. It also keeps a `Status` and `LastError`, so a settings page can show "connect again" without opening a token. `Validate` checks that the fields fit the kind and scope. It refuses an App installation with a person scope, and a user connection that carries a secret reference.

`Store` gets, lists, puts and deletes records. `ListConnections(owner)` gives a person's connections for a settings page, and the server's with an empty owner. agni#1728 needs "does this viewer have a connection to this source", and it gets that from the viewer's list plus the mount's source.

## Secrets

A record never holds a secret. A secret field holds an oneauth secret reference instead:

- `env:GITHUB_TOKEN` reads an environment variable.
- `file:/run/secrets/gh-app.pem` reads a mounted file. The path must be absolute.

`Validate` parses each reference and refuses anything else, including a pasted token. It never resolves a reference, so a record checks out on a machine without the secret, and its errors never repeat the value. oneauth reads a secret when it builds a token source.

A person's own token is different, since nobody can put it in an env var for them. oneauth's `UserTokens` seals it with AES-GCM under a key derived from one master key, and keeps it in a `UserTokenStore`. That store has folder, GORM and Datastore backends in oneauth v0.3.6. The key is `UserTokenProvider(connection ID)` plus the owner's subject. Keying by connection rather than by "github" lets one person hold two GitHub connections, such as github.com and an Enterprise host. The master key itself comes from a secret reference in the service's configuration.

## Token sources

The service is built in the implementation ticket. Its planned API:

```go
func NewService(store Store, opts ...Option) *Service
func WithUserTokens(store outbound.UserTokenStore, masterKeyRef string, oauth *oauth2.Config) Option
func WithHTTPClient(c *http.Client) Option

func (s *Service) TokenSource(ctx context.Context, id string) (oauth2.TokenSource, error)
func (s *Service) Disconnect(ctx context.Context, id string) error // revoke, delete the token, delete the record
```

`TokenSource` has the signature the mounts registry's `WithCredentials` takes. For each kind:

- `github-app`: one `outbound.GitHubAppInstallation` per connection, built on first use and kept. Every mount on the connection shares its cached token, which oneauth refreshes five minutes before it expires. Installation tokens live only in memory.
- `github-token`: `outbound.NewStatic(TokenRef)`.
- `github-user`: `UserTokens.TokenSource(owner)` under the connection's provider key. Each `Token` call reads the store, and refreshes with compare-and-swap when the token is close to expiry, so replicas don't fight. When the refresh token is dead, oneauth returns `outbound.ErrReconnect`. The service then sets the record's status to `reconnect`, and returns `ErrReconnect`.

A driver wraps the source in an `oauth2.Transport` and sends it with every request.

## Connecting a person

oneauth's GitHub provider asks for `read:user` and `user:email` at login. To read and write a person's private repos, the app lists `repo` in the provider's `AllowedExtraScopes`, and sends the person to the login path with `?scope=repo`. The provider's `HandleUserFunc` gets the token, and the app saves it with `UserTokens.Save` and puts a `github-user` record.

`HandleUserFunc` can't tell a connect from a login, and carries no return URL. The app keeps both in its own state, such as a short-lived cookie set before the redirect. A GitHub App's user tokens ignore OAuth scopes, and a person adds repos on the App's install page instead. That flow belongs to the app.

`Disconnect` revokes the token at GitHub with `outbound.GitHubRevoker`, then deletes the sealed token and the record. When the revoke fails, it keeps both so the app can retry, which is oneauth's rule.

## Where a secret can and can't end up

- **Records.** Never. `Validate` refuses anything that isn't a reference, and `PutConnection` runs it, so a pasted token never reaches the store. agni#1724 needs this so its `file://` and `sqlite://` stores can be backed up as they are.
- **Errors and logs.** Never. `Validate` doesn't repeat values, oneauth's errors don't repeat secrets, and `LastError` keeps only the message.
- **The user-token store.** Only sealed, and bound to its provider and subject by the seal's associated data. A sealed token copied to another row doesn't open.
- **Memory.** Installation tokens and resolved secrets sit in the process. A heap dump shows them.
- **The browser.** Never. The server makes every GitHub call, so no token goes to the page.
- **One master key** seals every person's token, and the sealed format has no key ID yet, so rotating the key means everyone connects again.

## Testing

oneauth's `outbound/githubtest` is a fake GitHub that checks the App's JWT, answers the installation-token exchange, and refreshes and revokes user tokens the way GitHub does. It doesn't serve repo contents. This module's fake GitHub, from [#155](https://github.com/panyam/goapplib/issues/155), serves a repo's commits, tarballs and contents, and checks the token on every request. A consumer's tests use both.

## The package

{{ apiref "storage/connections" }}
