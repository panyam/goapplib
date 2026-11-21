---
title: "Connections"
description: "The design of the storage module's connections: server-held sign-ins to GitHub, as the server or for one person, built on oneauth's outbound token sources, with where a secret can and can't end up."
---

Picture an agni server with three private repos mounted from one org. The org installed agni's GitHub App, so one installation covers all three repos, and the server signs in as that installation. Now an engineer mounts a repo of their own with a personal token, and a notation user connects their own GitHub account so notation can commit for them. Each of those is a connection: a sign-in the server holds for a third-party service. A mount names one by its owner and ID, and the driver reads through it.

This page is the design for [#153](https://github.com/panyam/goapplib/issues/153). The `storage/connections` package holds the record, its validation and the store interface, rendered at the bottom of the page from the source. So does `Service`, which builds each connection's token source, and a folder `Store` in `storage/connections/fsstore`. The tokens themselves come from oneauth's `outbound` package, from v0.3.6 ([oneauth#467](https://github.com/panyam/oneauth/issues/467)). This module keeps what oneauth leaves out on purpose: the record, its scope, which mounts use it, and which token source it builds. [Mounts](../mounts/) covers the other side.

## Kinds and scopes

| Kind | Scope | Signs in with | oneauth piece | First user |
|---|---|---|---|---|
| `github-app` | server | a JWT from the App's key, traded for an hour-long installation token | `outbound.GitHubAppInstallation` | agni, an org's repos |
| `github-token` | server or person | a fixed token, such as a personal access token | `outbound.Static` | agni, one engineer's repos |
| `github-user` | person | the person's own OAuth token, sealed and refreshed | `outbound.UserTokens` | notation |

- **Server scope.** Anyone the app lets read a mount reads it through the connection. So the app's own sharing decides who sees what. agni#1728 calls this out for links made through the server's identity.
- **Person scope.** The connection gives a token only to a request whose viewer is its `Owner`. The app sets the viewer with `WithViewer` from its own auth, and a request with no viewer, or a different one, gets `ErrNotOwner`. The service refuses before it reads the store, so the error says nothing about whether the owner has a connection with that ID. GitHub then decides what that person can read.

Later kinds are the server's own cloud identity for S3 and GCS (agni#1730), and Entra for SharePoint (agni#1729). They add a `Kind` and a token source, and the record shape holds them.

## The record

`Connection` keeps the ID, kind, scope and owner, a display name, the API base URL, and the fields for its kind. It also keeps a `Status` and `LastError`, so a settings page can show "connect again" without opening a token. `Validate` checks that the fields fit the kind and scope. It refuses an App installation with a person scope, and a user connection that carries a secret reference.

A connection's key is its owner and ID. A server connection's owner is empty. Say alice keeps a connection named `gh`. bob can still make his own `gh`, and the server can have a `gh` of its own, and they are three records. Every call that names a connection takes the owner beside the ID: `GetConnection(ctx, owner, id)`, `TokenSource(ctx, owner, id)`, `Disconnect(ctx, owner, id)`, and a mount's `ConnectionOwner` beside its `Connection` (#197). The service never works out an owner from the viewer, so a call means the same record whoever makes it.

`Store` gets, lists, puts and deletes records. `ListConnections(owner)` gives a person's connections for a settings page, and the server's with an empty owner. `CanRead`, below, uses the same list to answer whether a viewer can read a source.

`storage/connections/fsstore` keeps one YAML file per connection, written through a temp file and a rename. A server connection is `<id>.yaml`, and a person's is `people/<owner>/<id>.yaml`, with the owner's subject base64url-encoded so no subject can name a path. Several processes can share its folder. `PutConnection` runs `Validate`, so an invalid record never reaches disk. `storage/connections/connectionstest` is the contract suite every `Store` backend runs. For several servers without a shared folder, `gormstore.NewConnectionStore(db)` keeps connections in a `goapplib_connections_v2` table keyed on owner and ID, and `gaestore.NewConnectionStore(client, namespace)` keeps them as `GoapplibConnection` entities, a person's under a `GoapplibConnectionOwner` parent key. Each is in its own module, `storage/backends/gorm` or `storage/backends/gae`, and each runs `connectionstest`.

Before v0.7.23 the key was the ID alone, and each store reads what it wrote then:

- **Folder.** A person's `<id>.yaml` at the top still reads back as its owner's. The next write to that ID moves it under `people/`, so a server connection with the same ID can't overwrite it.
- **GORM.** GORM can't change a table's primary key, so the records moved to a new table. `NewConnectionStore` copies the old `goapplib_connections` table into the new one, once, when the new one is empty. It leaves the old table for servers still on an older version, and you drop it by hand afterwards. The copy doesn't run again, so a record an older server writes to the old table after it isn't seen. Move every server to v0.7.23 before people add connections.
- **Datastore.** A person's entity with no parent still reads back as its owner's, and the next write to that ID moves it under the parent in one transaction.

## Secrets

A record never holds a secret. A secret field holds an oneauth secret reference instead:

- `env:GITHUB_TOKEN` reads an environment variable.
- `file:/run/secrets/gh-app.pem` reads a mounted file. The path must be absolute.

`Validate` parses each reference and refuses anything else, including a pasted token. It never resolves a reference, so a record checks out on a machine without the secret, and its errors never repeat the value. oneauth reads a secret when it builds a token source.

A person's own token is different, since nobody can put it in an env var for them. oneauth's `UserTokens` seals it with AES-GCM under a key derived from one master key, and keeps it in a `UserTokenStore`. That store has folder, GORM and Datastore backends since oneauth v0.3.6. The key is `UserTokenProvider(connection ID)` plus the owner's subject. Keying by connection rather than by "github" lets one person hold two GitHub connections, such as github.com and an Enterprise host. The master key itself comes from a secret reference in the service's configuration.

## Token sources

An app builds one `Service` over its store. `WithUserTokens` gives it oneauth's user-token store, the master key's secret reference, and the OAuth app's config:

```go
svc := connections.NewService(store,
	connections.WithUserTokens(fs.NewFSUserTokenStore(dir), "env:CONNECTIONS_KEY", githubOAuth))
ts, err := svc.TokenSource(connections.WithViewer(ctx, subject), subject, "alice-gh")
```

`TokenSource` has the signature the mounts registry's `WithCredentials` takes, which passes a mount's `ConnectionOwner` and `Connection`. For each kind:

- `github-app`: one `outbound.GitHubAppInstallation` per connection, built on first use and kept. Every mount on the connection shares its cached token, which oneauth refreshes five minutes before it expires. Installation tokens live only in memory.
- `github-token`: `outbound.NewStatic(TokenRef)`.
- `github-user`: `UserTokens.TokenSource(owner)` under the connection's provider key. Each `Token` call reads the store, and refreshes with compare-and-swap when the token is close to expiry, so replicas don't fight. When the refresh token is dead, oneauth returns `outbound.ErrReconnect`. The service then sets the record's status to `reconnect`, and returns `ErrReconnect`.

The service reuses each connection's source until the record's sign-in fields change. A new `KeyRef` or `InstallationID` builds a new one on the next call. Any other failure sets the status to `failed`, and the next token that works sets it back to `ok`. `TokenSource` stamps `LastUsed` at most once an hour, so a busy mount doesn't write its connection's record on every request.

A driver wraps the source in an `oauth2.Transport` and sends it with every request.

## Can this viewer read it?

Imagine a viewer opening a design that agni linked from a private repo. agni has to decide whether to show it. GitHub knows whether this person can read the repo, so agni asks GitHub with the person's own connections:

```go
svc := connections.NewService(store, connections.WithReadCheck(githubDriver.CanRead))
id, err := svc.CanRead(connections.WithViewer(ctx, subject), "github://acme/board")
```

- `CanRead` lists the viewer's connections in ID order. For each one it builds the token source and calls the read check, and it returns the first connection that can read the source.
- The GitHub driver's `CanRead` is the read check. It asks for the repo, and a 404 counts as a definite no, which is also how GitHub answers a private repo the token can't see. A connection to another host than the driver serves gets a no without a request.
- When no connection can read, or the viewer has none, `CanRead` returns `ErrNoAccess`. A request with no viewer returns `ErrNotOwner`. If a check failed, such as a rejected token, and no connection succeeded, `CanRead` returns that failure rather than a no, since the answer isn't known.
- It keeps each yes or no for a minute per viewer and source (`WithAccessTTL` changes that), so a page with many links to one repo asks GitHub once. It doesn't keep a failure.
- `SaveUserToken`, `SavePastedToken` and `Disconnect` drop the kept answers of the connection's owner. So a person checked before they connect gets a yes on their next call, without waiting out the minute. An app that changes a person's connections through the `Store` calls `ForgetAccess(viewer)` after the write. A `CanRead` that was already running when the answers were dropped still returns its answer, but doesn't keep it.
- It never tries a server connection. A server connection would answer for the server, and a link made through one falls back to the app's own sharing (agni#1176).

The read check is a function rather than code in this package. That keeps every GitHub API call in the GitHub driver, and this package doesn't import mounts.

## Connecting a person

oneauth's GitHub provider asks for `read:user` and `user:email` at login. To read and write a person's private repos, the app lists `repo` in the provider's `AllowedExtraScopes`, and sends the person to the login path with `?scope=repo`. The provider's `HandleUserFunc` gets the token. The app puts a `github-user` record, then saves the token with `Service.SaveUserToken`, which also sets a `reconnect` status back to `ok`.

`HandleUserFunc` can't tell a connect from a login, and carries no return URL. The app keeps both in its own state, such as a short-lived cookie set before the redirect. A GitHub App's user tokens ignore OAuth scopes, and a person adds repos on the App's install page instead. That flow belongs to the app.

A person can also paste a token, such as a personal access token, instead of going through OAuth. The app saves it with `Service.SavePastedToken`, which seals it the same way and sets the record's `Pasted`. A later `SaveUserToken` clears `Pasted`.

`Disconnect` does one of two things, depending on how the token came in:

- **From the app's OAuth flow.** It revokes the token at GitHub with `outbound.GitHubRevoker`, then deletes the sealed token and the record. When the revoke fails, it keeps both so the app can retry, which is oneauth's rule.
- **Pasted.** It deletes the sealed token and the record, and makes no call to GitHub. GitHub refuses to revoke a token through an app that didn't issue it. The person revokes a pasted token on GitHub's own settings page.

Each save writes the token and the record's `Pasted` separately, ordered so a failed second write leaves `Pasted` unset. Disconnect then tries the revoke and keeps everything, rather than skip the revoke of a token the app issued (#183).

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
