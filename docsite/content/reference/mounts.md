---
title: "Mounts"
description: "The design of the storage module's mounts: the record, mount:// URIs, snapshot and live modes, the driver and record-store contracts, the registry, the limits the library enforces, and its errors."
---

Picture agni saving a circuit design that points at `mount://jetson/baseboard.kicad_sch`. A month later someone opens it. If `jetson` reads the repo's latest commit, the design now means different bytes from the ones it was made from. So agni pins a GitHub mount at the commit it resolved to, and keeps that commit's files. notation needs the opposite. A notation's text sits in a repo, a person edits it, and each save should become a new commit on the branch.

A mount is a named place an app's files live: a folder, a GitHub repo, and later an S3 or GCS bucket. The app keeps its own records in its own store, and they point into mounts by `mount://` URI. This page is the design for [#152](https://github.com/panyam/goapplib/issues/152). The `storage/mounts` package holds the record, the URI rules, the interfaces, the registry and the errors, and they are rendered at the bottom of the page from the source. The folder driver, the GitHub driver and the folder record store are in packages beside it. [Connections](../connections/) covers how a mount over a private repo signs in, and [the storage module](../storage-module/) covers where the code lives.

## How agni and notation use it

| | agni | notation |
|---|---|---|
| Kinds | folder (from flags), GitHub | GitHub, and later Bitbucket and S3 |
| Mode | snapshot, pinned at a commit | live, at the branch's head |
| Writes | folder mounts only, through its own code | `Put` and `Delete` of one file, each a commit |
| Views | a host path, since it runs `kicad-cli` and `os.Open` | an `fs.FS` |
| Connection | server-held: an App installation, or a token (agni#1724) | the person's own GitHub account |
| Record store | one YAML file per mount, shared by `agni serve` and `agnids` | Datastore |

## The record and the two modes

`Mount` keeps the name, the kind, the source, the ref it was asked for, the version that ref resolved to, the mode, the connection, and for a snapshot the hash of its manifest. The library owns those fields. An app keeps anything else in its own records, keyed by the mount's name.

- **Snapshot.** The driver resolves the ref to a version when the mount is made, then serves the files at that version for as long as the mount exists. A GitHub snapshot downloads the commit's tarball into the blob store ([#154](https://github.com/panyam/goapplib/issues/154)). It then writes a manifest of paths and hashes, and keeps the manifest's hash in the record. After a restart, the manifest rebuilds the view. agni keeps its file list in the record today, and that list can run to 100,000 entries, past Datastore's 1 MB entity limit. So the list moves into a blob.
- **Live.** Each open reads the source's latest version. A driver that implements `Writer` can change it. A GitHub write goes through the Contents API, and each `Put` or `Delete` is one commit. GitHub refuses a write whose file sha isn't the current one, and the driver returns `ErrConflict`, so two people saving the same notation can't silently overwrite each other.

Each driver says which modes it supports. GitHub supports both. A folder is live, and later may also snapshot by hashing its files the way agni's uploads do.

`Version` is a string in the driver's own format. agni checks for 40 hex characters, which fits SHA-1 commits and nothing else. S3 version IDs and SHA-256 git repos don't fit, so the library leaves the check to the driver.

## URIs and names

A file is `mount://<name>/<path>`. `ParseURI` takes agni's rules:

- The path is relative. A leading `/` is refused, `\` becomes `/`, and `path.Clean` runs before any check.
- A path that climbs out with `..` is refused, so resolving a URI against a mount's root can't reach outside it.
- The empty path is the mount's root.

A name is 1 to 64 characters, a-z and 0-9 first, then also `-`, `_` and `.`. That is agni's rule, and it keeps a name safe as a folder name, a URL path element and a YAML file name on any OS. goutils' `mountfs` takes any single path element, which is looser, and the registry only ever hands it names that pass `ValidName`.

## Drivers

A `Driver` serves one kind. It resolves a ref to a version, and opens a mount as an `fs.FS`. It gets an `oauth2.TokenSource` from the registry, or nil for a public source, and it knows nothing about connections or viewers. Two optional interfaces sit on top:

- `Writer` adds `Stat`, `Put` and `Delete` for live mounts. A write passes `IfMatch`, the file's version from `Stat`. Empty means create only, and `AnyVersion` means overwrite whatever is there.
- `Localizer` adds `HostPath` for code that needs a real file, such as agni running `kicad-cli`. A folder mount has one. A GitHub snapshot has one when its view is hard links on the server's disk. A mount with no folder here returns `ErrNotLocal`, so code that needs a host path fails clearly on GAE or in wasm.
- `Preparer` adds `Prepare`, which the registry calls when a mount is made, after `Resolve`. A GitHub snapshot downloads its files there and sets the mount's `Manifest`. If `Prepare` fails, no record is written.

The folder driver is `storage/mounts/folder`. Its mounts are live, writable and local, and it refuses a link anywhere in a path, so a link inside the folder can't reach outside it. A file's version is the SHA-256 of its contents. A write goes through a temp file and a rename, under a lock per file. So its `IfMatch` check holds between writers in one process, sharing one `folder.Driver`. Two processes writing one file can both pass the check, and the second rename wins.

The GitHub driver is `storage/mounts/github`, built with a blob store and options for the API root, the HTTP client, the limits and the views folder:

```go
gh := github.New(blobStore, github.WithViews(filepath.Join(storeDir, "views")))
```

- `Resolve` reads the repo's default branch when the ref is empty, then asks GitHub which commit the ref points at.
- For a snapshot, `Prepare` streams the commit's tarball into the blob store. It keeps regular files only, refuses a path that is absolute, unclean or climbs out, and stops at the limits with `ErrTooLarge`. Then it writes the manifest. `Open` serves the snapshot from the manifest and the blob store without asking GitHub, after a restart too.
- `HostPath` on a snapshot builds a folder of hard links to its blobs, at `<views>/<name>/<commit>`, the first time it is asked. A restart that lost the folder rebuilds it from the manifest.
- For a live mount, `Prepare` fills in the default branch when `Ref` is empty, since a write commits onto a branch. `Open` reads the branch through the Contents API on every open. `Stat` returns a file's git blob sha, and `Put` and `Delete` each make one commit. GitHub refuses a stale sha with 409, and an update without a sha with 422, and the driver returns `ErrConflict` for both.
- 404 becomes `ErrNotFound`, which is also what a private repo answers without a valid token. Rate limits, rejected tokens and server errors become `ErrUnavailable`. An error from the connection's token source, such as a person who has to connect again, passes through unchanged.

`storage/mounts/github/githubtest` is a fake GitHub for tests, the library's and an app's own. It serves a repo's commits, tarballs and contents from memory, and each write makes a real commit. A private repo answers only an accepted token, and every request is recorded with the token it carried. oneauth's `outbound/githubtest` mints and refreshes tokens, and `AcceptToken` lets this fake trust the ones it mints. A test against real GitHub reads `octocat/Hello-World`, and runs with `STORAGE_LIVE_GITHUB=1`.

## The registry

`Registry` joins the fixed mounts from the server's configuration, the runtime mounts from a `RecordStore`, and the drivers, and an app goes through it for everything:

```go
store, _ := fsstore.New(filepath.Join(storeDir, "mounts"))
reg := mounts.NewRegistry(store, []mounts.Driver{folder.New(), github},
	mounts.WithFixed(mounts.Mount{Name: "boards", Kind: mounts.KindFolder, Source: "/srv/boards", Mode: mounts.ModeLive}),
	mounts.WithCredentials(conns.TokenSource),
	mounts.WithInUse(designs.PointsInto),
)
```

- `Create` resolves the ref, calls `Prepare`, sets `Version` and `Created`, and writes the record. It refuses `KindFolder`, a mode the driver doesn't serve, and a name a fixed mount has.
- `Delete` refuses a fixed mount with `ErrFixed`. It asks the app's `WithInUse` check whether records still point into the mount, and refuses with `ErrInUse` when they do. agni#1735 wanted this, and agni doesn't check it yet.
- `Open`, `Stat`, `HostPath`, `Put` and `Remove` work on one file. A write to a snapshot mount returns `ErrReadOnly` before it reaches the driver, and one over `WithLimits`' `MaxBytes` returns `ErrTooLarge`.
- `FS(ctx)` returns every mount as one `fs.FS` through goutils' `mountfs`, with each mount's driver opened when a path first reaches it. It keeps `ctx` for those opens, because a person's connection gives a token only to its owner. An app calls `FS` per request. A background job that needs a person's mount passes that person's context.
- `WithCredentials` takes the [connections](../connections/) service's `TokenSource` method, so the registry asks for a mount's token without importing connections.
- Before using its list, the registry compares the store's `Generation` with the last one it read, at most once a second (`WithRecheck` changes that). It lists again only when they differ. A registry sees its own creates and deletes at once. If the store fails after a good read, the registry keeps serving the last good list.
- `BlobRefs(store)` gives a blob sweep the manifest and files of every snapshot mount. An app passes it to `Sweep` beside its own referrers. A manifest it can't read fails the sweep, so a snapshot never loses its files to it. The sweep's grace period should be longer than the recheck interval, since a mount another process just made reaches this registry's list only at the next check.

## Record stores

A `RecordStore` creates and deletes records, and never changes one in place. Several processes can share a store, and `Generation` is how each one notices the others' changes. `storage/mounts/mountstest` checks a backend against this contract, the way oneauth's `usertokentest` checks user-token stores. A backend's test calls `mountstest.RunRecordStore`, and `RunShared` when two of its stores can share data.

- **Folder,** `storage/mounts/fsstore`. One YAML file per mount, read through `os.Root`. A create writes a temp file and hard-links it to `<name>.yaml`. The link fails when the name exists, so of two processes creating one name, one wins. `Generation` is agni's stamp of each file's name, size and mtime. A folder's own mtime isn't enough on Windows.
- **GORM,** `gormstore.NewMountStore(db)` from the module `github.com/panyam/goapplib/storage/backends/gorm`, over any database GORM supports. It keeps records in the `goapplib_mounts` table. A create inserts with "on conflict do nothing", and an insert that adds no row returns `ErrExists`, so of two servers creating one name, one wins. `Generation` is a counter in `goapplib_mount_generation` that each create and delete bumps in the same transaction.
- **Datastore,** `gaestore.NewMountStore(client, namespace)` from the module `github.com/panyam/goapplib/storage/backends/gae`. Each record is a `GoapplibMount` entity keyed by the mount's name. A create reads the key and writes it in one transaction, and so does the generation counter. Every create and delete writes that one counter entity, so writers contend on it, and the store retries a contended transaction up to 32 times rather than Datastore's default 3.

Both backends are modules of their own, so an app that keeps records in a folder never downloads GORM or the Datastore SDK. Both run `mountstest`. GORM runs on SQLite in CI. Datastore runs with `make test-datastore`, which starts the local emulator, and CI skips it.

## What the library enforces

These checks live in the library, so an app doesn't write them again:

- URI and name rules, as above.
- Regular files and folders only. A snapshot skips links, devices and anything else, and refuses a tar entry that is absolute, climbs, or isn't clean.
- `Limits` on bytes and files, defaulting to agni's 256 MB and 100,000 files. Past either, the driver returns `ErrTooLarge`.
- Folder mounts come from the server's configuration only. `Create` refuses `KindFolder`, so a client can't make a mount over a server folder of its choosing.

## Errors

Each error wraps one of the package's sentinels, and an app maps them with `errors.Is`. agni maps them onto its service errors one to one:

| mounts | agni's `service` |
|---|---|
| `ErrNotFound` | `ErrNotFound` |
| `ErrExists` | `ErrAlreadyExists` |
| `ErrInvalidArgument` | `ErrInvalidArgument` |
| `ErrInvalidPath` | `ErrInvalidPath` |
| `ErrFixed`, `ErrInUse`, `ErrReadOnly` | `ErrFailedPrecondition` |
| `ErrTooLarge` | `ErrResourceExhausted` |
| `ErrUnavailable` | `ErrUnavailable` |
| `ErrConflict` | `ErrAborted`, which agni doesn't have yet |
| `ErrNotLocal` | `ErrFailedPrecondition` |

## What agni deletes when it moves over

- `mounts/mounts.go`'s `Mount`, `Kind`, `SafeResolve` and `Resolve`, and `artifact/uri.go`. `ParseURI` and `Registry.HostPath` replace them.
- `mounts/table.go`: `mounts.Registry` and `fsstore` replace it. agni's records gain `version` and `mode`, and its `commit` key becomes `version`.
- `cmd/agni/mountmaker.go`'s GitHub code: `resolveRef`, `snapshot`, `tarPath` and the HTTP error mapping. The GitHub driver replaces them. agni keeps `CreateMount` and `DeleteMount` as thin calls into the registry.
- `Record.Files`: the manifest blob replaces it. agni's existing records need a one-time rewrite, which the move's ticket in agni covers.

agni keeps its `Workspace` adapter, its `archivefs` reading, and the flags. `make exercise-mounts` should print the same lines before and after the move.

## The package

{{ apiref "storage/mounts" }}
