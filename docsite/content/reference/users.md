---
title: "UsersService"
description: "User profiles for a goapplib app: the UsersService interface, its filesystem, GORM and Datastore backends, and app-specific data in Extras."
---

goapplib ships a user-profile service: who a user is to your app (name, email, image, tags, and whatever else you keep in `Extras`), as distinct from how they log in, which is [oneauth](https://github.com/panyam/oneauth)'s job. The example is `docsite/examples/users` in goapplib's repo, whose test runs it against the filesystem backend in a temporary directory:

```go
{{ includeFileText "examples/users/users.go" }}
```

## The service

`services.UsersService` is a Go interface over the `goapplib.v1.UsersService` protobuf service (`protos/goapplib/v1/users.proto`), so the same methods can be served over Connect or gRPC:

- `CreateUser`, `GetUser`, `GetUsers` (a batch, by id), `ListUsers` (paginated), `UpdateUser` and `DeleteUser` take and return the generated request and response messages.
- `EnsureUser(ctx, id, name, email, imageUrl)` is for right after a login. It creates the profile the first time, and after that updates the name, email and image with whichever of them the login supplied, leaving the rest as they were. The example's test checks both halves.

A `User` has `Id` (the same id your auth system uses), `Name`, `Description`, `Email`, `ImageUrl`, `Tags`, `CreatedAt`, `UpdatedAt`, and `Extras`, a `google.protobuf.Struct` for your app's own fields. The example keeps a theme preference there with `structpb.NewStruct`, which saves the app a user table of its own for small things like that.

`UpdateUser` changes only the fields you set. An empty `Name` leaves the name alone, and a nil `Extras` leaves the extras alone.

## Backends

All three embed `services.BaseUsersService`, which does the reads, `EnsureUser` and an in-memory cache of users by id, and they differ in where users are kept:

- **Filesystem,** `fsusers.NewUsersService(dir)` from `services/backends/fs`, one JSON file per user. It's mostly for development and tests.
- **GORM,** `gormusers.NewUsersService(db)` from `services/backends/gorm`, over any database GORM supports (PostgreSQL, MySQL, SQLite), and it migrates its table.
- **Google Cloud Datastore,** `gaeusers.NewUsersService(client, namespace)` from `services/backends/gae`, with the namespace keeping tenants apart.

A new backend implements `services.UserStorageProvider` (`LoadUser`, `ListAllUsers`, `SaveUser`, `DeleteFromStorage`, `UserExists`) and embeds `BaseUsersService` for the rest. The [filesystem backend](https://github.com/panyam/goapplib/blob/main/services/backends/fs/users_service.go) is the shortest one to copy.

We ran into one catch writing this page, tracked as [#114]. (https://github.com/panyam/goapplib/issues/114) With the cache on, which all three backends turn on, `GetUser` can hand back the cached `User` itself, so changing a field on what you got changes the cache, saved or not. Edit a copy (`proto.Clone`), as the example's `SetTheme` does.

## Auth

`services.AuthService` holds oneauth's stores (users, identities, channels, verification tokens) for an app's login flow. After a login, call `EnsureUser` with the logged-in user's id to keep their profile in step. Its constructors, `NewAuthService(storagePath)` for file-based stores and `NewAuthServiceWithStores(...)` for your own, are deprecated in favor of using oneauth's stores directly. `NewAuthServiceWithAllStores` adds a username store.

To show who's logged in on a page, goapplib's `WithAuth` mixin takes a small `AuthProvider` instead. The [Views and mixins]({{.Site.PathPrefix}}/guide/views/#goapplibs-mixins) page covers that.
