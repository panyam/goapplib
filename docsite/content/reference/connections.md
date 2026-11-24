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
| `aws` | server | an access key pair, or the AWS default chain, with an optional role to assume | none; the S3 driver signs | agni, designs in S3 (agni#1730) |
| `gcp` | server | a service account key, or Application Default Credentials | `x/oauth2/google` | agni, designs in GCS (agni#1730) |
| `entra` | server | an Entra app registration's client secret or certificate, the Azure host's managed identity, or an AKS workload identity's token file | none; the service talks to Entra | agni, designs in SharePoint (agni#1729) |
| `entra-user` | person | the person's own Entra token, sealed and refreshed | `outbound.UserTokens` | agni, a person's OneDrive (agni#1729) |

- **Server scope.** Anyone the app lets read a mount reads it through the connection. So the app's own sharing decides who sees what. agni#1728 calls this out for links made through the server's identity.
- **Person scope.** The connection gives a token only to a request whose viewer is its `Owner`. The app sets the viewer with `WithViewer` from its own auth, and a request with no viewer, or a different one, gets `ErrNotOwner`. The service refuses before it reads the store, so the error says nothing about whether the owner has a connection with that ID. GitHub then decides what that person can read.

The two cloud kinds sign in as the server alone, since a bucket has no per-person view to defer to:

- **`aws`** keeps `AccessKeyID` in the record, since AWS doesn't treat it as a secret, and the secret access key behind `KeyRef`. Both or neither must be set. With neither, the S3 driver uses the AWS SDK's default chain, such as an EC2 instance role or an IRSA role on Kubernetes. `RoleARN` names a role to assume first, and `Region` sets the region. The service resolves `KeyRef` on each call and returns `mounts.AWSCredentials`, so the S3 driver builds its own provider and the storage module never imports the AWS SDK.
- **`gcp`** keeps a service account key (its JSON file) behind `KeyRef`. With no `KeyRef`, it uses Application Default Credentials, such as workload identity on GKE. Its tokens carry the read-only `devstorage.read_only` scope (`GCSReadScope`), since a mount only reads.

Neither takes a `BaseURL`. A test or an S3-compatible store sets the driver's endpoint instead.

The two Entra kinds sign Microsoft Graph requests for the msgraph mount driver (#206). Both name a tenant (`TenantID`) and an app registration (`ClientID`). `BaseURL` is the Entra authority, and empty means `https://login.microsoftonline.com`, so a national cloud or a test's fake sets it.

- **`entra`** signs in as the server, and `EntraAuth` says how. It never infers the mode from which fields are set.
  - `secret` sends the client secret at `KeyRef`.
  - `certificate` signs a client assertion (RFC 7523) with the RSA key at `KeyRef`. The assertion's `x5t` header is the SHA-1 thumbprint of the certificate at `CertRef`, which is how Entra finds the registered certificate. Each assertion lives five minutes and carries a fresh `jti`. The service checks that the key matches the certificate before it sends anything.
  - `managed-identity` asks the Azure host's identity endpoint for a token, so the record holds no secret. `ClientID`, when set, picks a user-assigned identity. On a VM, a scale set or an AKS node, the endpoint is IMDS. `WithManagedIdentityURL` changes it for a test. It is a service option rather than a record field, so a stored record can't send the host's identity token somewhere else.
  - App Service and Container Apps serve the identity elsewhere. The host sets `IDENTITY_ENDPOINT` and a secret in `IDENTITY_HEADER`. The app passes both to the `WithIdentityEndpoint(endpoint, header)` service option, and the service sends the secret as `X-IDENTITY-HEADER`. The service never reads those variables itself (#214).
  - `workload-identity` is for AKS workload identity. The AKS webhook gives the pod a service account token in a file, and names the file in `AZURE_FEDERATED_TOKEN_FILE`. `KeyRef` must be a `file:` reference to that file. The service sends the token as the client assertion for `ClientID`. The cluster rotates the file, so the service reads it again for each token request. The app registration needs a federated credential that trusts the cluster's OIDC issuer and the service account.
  - Tokens are for `GraphScope`, every Graph application permission an admin granted the app.
- **`entra-user`** belongs to a person, and keeps their token in the user-token store as `github-user` does. oneauth refreshes it at the tenant's token endpoint with `GraphUserScope`. `KeyRef`, when set, is the app registration's client secret for a confidential client. A public client leaves it empty.

## The record

`Connection` keeps the ID, kind, scope and owner, a display name, the API base URL, and the fields for its kind. It also keeps a `Status` and `LastError`, so a settings page can show "connect again" without opening a token. `Validate` checks that the fields fit the kind and scope. It refuses an App installation with a person scope, and a user connection that carries a secret reference.

A connection's key is its owner and ID. A server connection's owner is empty. Say alice keeps a connection named `gh`. bob can still make his own `gh`, and the server can have a `gh` of its own, and they are three records. Every call that names a connection takes the owner beside the ID: `GetConnection(ctx, owner, id)`, `Credentials(ctx, owner, id)`, `Disconnect(ctx, owner, id)`, and a mount's `ConnectionOwner` beside its `Connection` (#197). The service never works out an owner from the viewer, so a call means the same record whoever makes it.

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

A server connection can keep its secret sealed in the same store. Picture a server running in a container. An operator runs `connections add hw --github-token env:HW_TOKEN` on their own machine, and the reference resolves in the container's environment, where `HW_TOKEN` doesn't exist. So the record sets `Sealed` and leaves the reference empty, and `Service.SaveServerSecret(ctx, id, secret)` seals the secret with a key derived from the master key. It works for a token, a GitHub App's private key, an AWS secret access key, a GCP service account key and an Entra client secret or certificate key. A managed identity has no secret, and a workload identity reads a file the cluster rotates, so neither takes `Sealed`. Saving again rotates the secret, and every process sharing the stores uses the new one on its next `Credentials` call. `Disconnect` deletes the sealed secret before the record. Nothing hands the secret back, and a record says only that it is sealed. A sealed connection with nothing saved yet returns `ErrNoSecret`. It needs `WithUserTokens`, and the secret is kept under the provider `goapplib-server-secret:<id>`, so it never meets a person's token for a connection with the same ID.

## Credentials

An app builds one `Service` over its store. `WithUserTokens` gives it oneauth's user-token store, the master key's secret reference, and the OAuth app's config:

```go
svc := connections.NewService(store,
	connections.WithUserTokens(fs.NewFSUserTokenStore(dir), "env:CONNECTIONS_KEY", githubOAuth))
creds, err := svc.Credentials(connections.WithViewer(ctx, subject), subject, "alice-gh")
```

`Credentials` has the signature the mounts registry's `WithCredentials` takes, which passes a mount's `ConnectionOwner` and `Connection`. The GitHub kinds and `gcp` return `mounts.BearerCredentials` over a token source, and `aws` returns `mounts.AWSCredentials`. For each kind:

- `github-app`: one `outbound.GitHubAppInstallation` per connection, built on first use and kept. Every mount on the connection shares its cached token, which oneauth refreshes five minutes before it expires. Installation tokens live only in memory.
- `github-token`: `outbound.NewStatic(TokenRef)`.
- `gcp`: `google.CredentialsFromJSONWithType` over the key at `KeyRef`, or `google.FindDefaultCredentials`.
- `aws`: no token source. Each call resolves `KeyRef` and returns the key pair, or `DefaultChain` set when the record names no key.
- `entra`: a token from the tenant's v2 token endpoint, or from IMDS, kept until it expires.
- `entra-user`: the same `UserTokens` source as `github-user`, refreshed at the tenant's token endpoint. x/oauth2 sends no `scope` on a refresh, and Entra's v2 endpoint expects one, so the service's client adds `GraphUserScope` to each refresh request.
- `github-user`: `UserTokens.TokenSource(owner)` under the connection's provider key. Each `Token` call reads the store, and refreshes with compare-and-swap when the token is close to expiry, so replicas don't fight. When the refresh token is dead, oneauth returns `outbound.ErrReconnect`. The service then sets the record's status to `reconnect`, and returns `ErrReconnect`.

The service reuses each connection's source until the record's sign-in fields change. A new `KeyRef` or `InstallationID` builds a new one on the next call. Any other failure sets the status to `failed`, and the next token that works sets it back to `ok`. `Credentials` stamps `LastUsed` at most once an hour, so a busy mount doesn't write its connection's record on every request.

A bearer driver wraps the source in an `oauth2.Transport` and sends it with every request. The S3 driver builds an AWS credentials provider from `AWSCredentials` instead.

## Can this viewer read it?

Imagine a viewer opening a design that agni linked from a private repo. agni has to decide whether to show it. GitHub knows whether this person can read the repo, so agni asks GitHub with the person's own connections:

```go
svc := connections.NewService(store,
	connections.WithReadCheckFor("github", githubDriver.CanRead, connections.KindGitHubUser, connections.KindGitHubToken),
	connections.WithReadCheckFor("msgraph", graphDriver.CanRead, connections.KindEntraUser))
id, err := svc.CanRead(connections.WithViewer(ctx, subject), "github://acme/board")
```

- `WithReadCheckFor` sets the check for one source scheme and the connection kinds `CanRead` tries with it. Say alice has a GitHub connection and a SharePoint sign-in. When she opens a repo link, only her GitHub connection is tried, so her Entra token never goes to GitHub. `WithReadCheck` still works as the check for any scheme without its own, but it tries every kind. An empty `BaseURL` means github.com to the GitHub check, so through `WithReadCheck` an Entra connection's token reaches GitHub (#236).
- The msgraph driver's `CanRead` asks Graph for the folder with the person's token. A 403 or 404 is a no, since Graph answers 403 for a library the person can't open. A rejected token is an error. It ignores the connection's `BaseURL`, which for Entra is the sign-in authority, and asks the Graph root the driver was built with.

- `CanRead` lists the viewer's connections in ID order. For each one it gets its credentials and calls the read check, and it returns the first connection that can read the source.
- The GitHub driver's `CanRead` is the read check. It asks for the repo, and a 404 counts as a definite no, which is also how GitHub answers a private repo the token can't see. A connection to another host than the driver serves gets a no without a request.
- When no connection can read, or the viewer has none, `CanRead` returns `ErrNoAccess`. A request with no viewer returns `ErrNotOwner`. If a check failed, such as a rejected token, and no connection succeeded, `CanRead` returns that failure rather than a no, since the answer isn't known.
- It keeps each yes or no for a minute per viewer and source (`WithAccessTTL` changes that), so a page with many links to one repo asks GitHub once. It doesn't keep a failure.
- `SaveUserToken`, `SavePastedToken` and `Disconnect` drop the kept answers of the connection's owner. So a person checked before they connect gets a yes on their next call, without waiting out the minute. An app that changes a person's connections through the `Store` calls `ForgetAccess(viewer)` after the write. A `CanRead` that was already running when the answers were dropped still returns its answer, but doesn't keep it.
- It never tries a server connection. A server connection would answer for the server, and a link made through one falls back to the app's own sharing (agni#1176).

The read check is a function rather than code in this package. That keeps every GitHub API call in the GitHub driver, and this package doesn't import mounts.

### Granting a source to everyone the server signs in

Imagine a team's server that reads its boards from a private repo through a server connection. People sign in with local accounts or through Entra, so none of them has a GitHub connection, and `CanRead` tells each of them no. The operator already chose who may sign in, so the server's own access can stand for theirs (agni#1889). A grant says so:

```go
svc := connections.NewService(store,
	connections.WithReadCheckFor("github", githubDriver.CanRead, connections.KindGitHubUser),
	connections.WithSourceGrants(
		connections.SourceGrant{Source: "github://acme/boards", Viewers: []string{"local:*", "oidc:alice"}}))
id, err := svc.CanRead(connections.WithViewer(ctx, "local:carol"), "github://acme/boards")
// id == connections.GrantedBySource, err == nil
```

- `CanRead` checks the grants after it checks for a viewer, and before it looks for a read check or lists connections. A matching grant returns `GrantedBySource` with no error and no call to GitHub. It works on a service with no read check for the scheme.
- `GrantedBySource` is `"grant:"`. No connection can have that ID, since `Validate` refuses the `:` and every store calls `Validate`. A caller compares the ID with it to tell a grant from a connection.
- A grant's `Source` matches the source byte for byte. `CanRead` doesn't fold case, trim a trailing `/` or drop a `.git`. A source spelled another way goes on to the viewer's connections, so a mismatch hides a board and never shows one.
- Each `Viewers` entry is `*`, a prefix with one final `*`, or one exact viewer. `*` matches any viewer. `local:*` matches `local:carol` and any other viewer that starts with `local:` and has at least one more character. Any other entry matches only itself.
- A request with no viewer still gets `ErrNotOwner`, whatever the grants say.
- `SourceGrant.Validate` refuses a source that isn't `scheme://rest` or holds a `*`, a grant with no viewers, and a pattern that is empty or has a `*` before its end, such as `lo*cal:*` or `**`. Each failure wraps `ErrInvalidGrant`. `WithSourceGrants` can't return an error, so an app calls `Validate` on each grant as it loads its config. A pattern that fails it matches nobody.
- The service copies the grants when it is built, and they don't change after that. So `CanRead` doesn't keep grant answers, and the access TTL and `ForgetAccess` apply only to answers from connections.

### Granting a source to a group

Say only one Entra group should see the boards. Listing each `oidc:<sub>` by hand doesn't follow the group, and `oidc:*` admits everyone the sign-in lets in. The app knows the groups from the ID token at sign-in, so it hands them to `CanRead` as claims beside the viewer (agni#1896):

```go
ctx = connections.WithViewer(ctx, "oidc:ada")
ctx = connections.WithViewerClaims(ctx, "oidc:group:hw", "oidc:group:sales")
id, err := svc.CanRead(ctx, "github://acme/boards") // a grant of "oidc:group:hw" admits ada
```

- A pattern matches the viewer's subject or any one claim, by the same rules. A prefix such as `oidc:group:*` matches a claim too.
- Claims count only beside a viewer. A request with claims and no viewer still gets `ErrNotOwner`.
- The app spells each claim so that no subject can equal it. A claim never picks a connection or a token, and `CanRead` keeps no answer for one, since a grant answer is never kept.
- `WithViewerClaims` copies the slice, and `ViewerClaims` reads it back.

## Connecting a person

oneauth's GitHub provider asks for `read:user` and `user:email` at login. To read and write a person's private repos, the app lists `repo` in the provider's `AllowedExtraScopes`, and sends the person to the login path with `?scope=repo`. The provider's `HandleUserFunc` gets the token. The app puts a `github-user` record, then saves the token with `Service.SaveUserToken`, which also sets a `reconnect` status back to `ok`.

`HandleUserFunc` can't tell a connect from a login, and carries no return URL. The app keeps both in its own state, such as a short-lived cookie set before the redirect. A GitHub App's user tokens ignore OAuth scopes, and a person adds repos on the App's install page instead. That flow belongs to the app.

A person can also paste a token, such as a personal access token, instead of going through OAuth. The app saves it with `Service.SavePastedToken`, which seals it the same way and sets the record's `Pasted`. A later `SaveUserToken` clears `Pasted`.

`Disconnect` does one of two things, depending on how the token came in:

- **From the app's OAuth flow.** It revokes the token at GitHub with `outbound.GitHubRevoker`, then deletes the sealed token and the record. When the revoke fails, it keeps both so the app can retry, which is oneauth's rule.
- **Pasted.** It deletes the sealed token and the record, and makes no call to GitHub. GitHub refuses to revoke a token through an app that didn't issue it. The person revokes a pasted token on GitHub's own settings page.

A person signs in to Entra through oneauth's OIDC provider (`oauth2/oidc` in oneauth v0.3.16), with the tenant's v2 issuer (`https://login.microsoftonline.com/<tenant>/v2.0`), the Graph scopes the app needs plus `offline_access`, and the same client ID as the `entra-user` record. `connections.EntraEndpoint(record)` gives the authorize and token URLs the service refreshes at. The provider's `HandleUser` gets the token, and the app saves it with `SaveUserToken`, as for GitHub. `Disconnect` on an `entra-user` connection deletes the sealed token and the record with no call to Entra, since Entra has no endpoint that revokes one token. The person removes the app's consent in their Microsoft account to end it there.

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

`storage/connections/entratest` is a fake Entra. It serves a tenant's token endpoint for client secrets, certificate assertions and refresh tokens, plus an IMDS endpoint and an App Service identity endpoint. `IdentityEndpoint` returns the second one's URL and the header secret it expects. It checks an assertion the way Entra does: the RS256 signature against the registered certificate, `x5t`, `aud`, `iss` and `sub`, a lifetime of at most ten minutes, and a `jti` it hasn't seen. A workload identity's assertion comes from a fake cluster issuer instead. `FederatedToken(subject, expires)` mints one, and `AddFederatedApp(tenant, clientID, subject)` trusts it. The fake refuses a federated token whose signature, issuer, `aud` (`FederatedAudience`), subject or expiry is wrong. `NewCertificate` makes a key and certificate pair, `SignIn` stands in for a person signing in, and `ValidToken` lets the msgraph driver's fake Graph trust the tokens it issues.

## The package

{{ apiref "storage/connections" }}
