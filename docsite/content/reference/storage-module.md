---
title: "The storage module"
description: "Where mounts, connections and blobs live: a Go module of their own inside goapplib's repo, versioned with goapplib, with heavy cloud SDKs kept in modules of their own."
---

Picture a notes app on goapplib that never touches GitHub or S3. It shouldn't have to download a GitHub client or the AWS SDK because goapplib grew mounts. So mounts, connections and blobs live in their own Go module, `github.com/panyam/goapplib/storage`, in the `storage/` directory of goapplib's repo. An app that wants them adds a second `go get`. An app that doesn't gets nothing new in its `go.sum`.

This page records the layout, the version policy and the dependency rule. The storage mission ([#150](https://github.com/panyam/goapplib/issues/150)) has the rest of the design. Its tickets decide what goes inside the module.

## Layout

```
storage/                  module github.com/panyam/goapplib/storage
  doc.go                  package storage
  mounts/                 records, the registry, folder and GitHub drivers
  connections/            server-held sign-ins
  blobs/                  files kept once by hash, for snapshots and uploads
  backends/gorm/          its own module: mount and connection stores over GORM
  backends/gae/           its own module: mount and connection stores over Datastore
  drivers/s3/             its own module, when it exists
  drivers/gcs/            its own module, when it exists
```

[Mounts](../mounts/), [Connections](../connections/) and [Blobs](../blobs/) have the design of each package, and say which parts are built so far.

## The dependency rule

- goapplib's root module never imports `storage`.
- `storage` depends on the standard library, goutils and oneauth. Importing goapplib's root module would pull Datastore and gorm into every storage user, so it does that only when nothing else works.
- The folder and GitHub drivers live in `storage` itself. GitHub goes through `net/http`, with tokens from oneauth's token sources ([oneauth#467](https://github.com/panyam/oneauth/issues/467)).
- A driver that needs a third-party SDK gets its own module under `storage/drivers/<name>`. An app that uses folders and GitHub then never downloads the AWS or GCS SDKs.
- A record store that needs a database library gets its own module under `storage/backends/<name>`, for the same reason. An app that keeps its records in a folder never downloads GORM or the Datastore SDK.

The root module already depends on Datastore and gorm for `UsersService`. Those came first, and moving them would break every app's imports. The rule stops the list from growing.

A dependency that both modules have moves in both at once. An app that imports goapplib and `storage` gets one version of oneauth, the higher of the two, so the root has to build against whatever `storage` requires. Every module in the repo that uses oneauth requires v0.3.7.

## Versions

Every module in the repo moves in lock-step with goapplib's `VERSION`, as the TypeScript packages do. A release tags `vX.Y.Z` and `storage/vX.Y.Z` on the same commit, and later `storage/drivers/s3/vX.Y.Z` too. `go get github.com/panyam/goapplib/storage@v0.7.13` is then the same release as goapplib `v0.7.13` and `@panyam/tsappkit` `0.7.13`.

`scripts/release.sh` makes the tags. `scripts/go-modules.sh` lists every `go.mod` below the root except `docsite/`, and the release script tags each one. A new driver module gets tagged without anyone editing the release script.

When only goapplib changed, a storage release is identical to the one before it. We're fine with an extra tag, since nobody then has to work out which storage version goes with which goapplib.

Picture an app that runs `go get github.com/panyam/goapplib/storage/backends/gorm@v0.7.13`. The gorm module requires `storage`, so Go fetches `storage` at whatever version the gorm module's `go.mod` names. That version has to exist as a tag. So a module that requires another of this repo's modules requires it at the version in `VERSION`, the one the next release tags, and keeps a `replace` pointing at the copy in this repo until then. A consumer ignores the `replace`. The release tags both modules on one commit, so the version the gorm module names exists by the time anyone fetches it.

`scripts/check-versions.sh` fails a PR whose nested module requires another of this repo's modules at a version other than `VERSION`. When `release.sh` sets a new version, it moves those requires too. The same rule holds if `storage` ever imports the root module.

## Tests and the API reference

- `make test` runs `go test ./...` in the root, which skips nested modules, and then `go vet` and `go test` in each module `go-modules.sh` lists. CI runs `make test`, so a storage test fails a PR.
- The [Go API](../api/) page reads packages from source by directory, so a package in another module reads the same way. A storage package joins `apiPackages` in `docsite/apiref.go` along with its first exported name. The reference test then checks it against its source like the root packages. The test also fails a package with nothing exported, so an empty package can't join the list.
- pkg.go.dev indexes `storage/vX.Y.Z` tags as a module of their own.

## Working against an unreleased copy

Say agni needs a storage change that isn't tagged yet. Use a Go workspace in the consumer's checkout rather than a `replace` in its `go.mod`:

```sh
cd agni
go work init .
go work use ../goapplib/storage    # and ../goapplib, if storage imports it
```

`go.work` and `go.work.sum` go in the consumer's `.gitignore`. A `replace` sits in `go.mod`, which a PR commits along with everything else, and a fresh clone or CI then fails on a path that doesn't exist. A workspace never leaves the machine. `GOWORK=off go build ./...` checks the build against the released versions before a PR goes up. goapplib's own `.gitignore` lists `go.work` too, for working across the root and `storage` at once.
