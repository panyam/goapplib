---
title: "Mounts"
description: "The design of the storage module's mounts: the record, mount:// URIs, snapshot and live modes, the driver and record-store contracts, the registry, the limits the library enforces, and its errors."
---

Picture agni saving a circuit design that points at `mount://jetson/baseboard.kicad_sch`. A month later someone opens it. If `jetson` reads the repo's latest commit, the design now means different bytes from the ones it was made from. So agni pins a GitHub mount at the commit it resolved to, and keeps that commit's files. notation needs the opposite. A notation's text sits in a repo, a person edits it, and each save should become a new commit on the branch.

A mount is a named place an app's files live: a folder, a GitHub repo, and later an S3 or GCS bucket. The app keeps its own records in its own store, and they point into mounts by `mount://` URI. This page is the design for [#152](https://github.com/panyam/goapplib/issues/152). The `storage/mounts` package holds the record, the URI rules, the interfaces and the errors, and they are rendered at the bottom of the page from the source. The registry and the drivers come in the implementation tickets. [Connections](../connections/) covers how a mount over a private repo signs in, and [the storage module](../storage-module/) covers where the code lives.

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

## The registry

The registry joins the fixed mounts from the server's configuration, the runtime mounts from a `RecordStore`, and the drivers. It is built in the folder-driver ticket. Its planned API:

```go
func NewRegistry(store RecordStore, drivers []Driver, opts ...Option) *Registry
func WithFixed(ms ...Mount) Option        // from flags; a runtime record with the same name is ignored
func WithCredentials(f func(ctx context.Context, connection string) (oauth2.TokenSource, error)) Option
func WithInUse(f func(ctx context.Context, name string) (bool, error)) Option
func WithLimits(l Limits) Option

func (r *Registry) List(ctx context.Context) ([]Mount, error)
func (r *Registry) Get(ctx context.Context, name string) (Mount, error)
func (r *Registry) Create(ctx context.Context, m Mount) (Mount, error) // resolves m.Ref, fills Version
func (r *Registry) Delete(ctx context.Context, name string) error      // ErrFixed, ErrInUse
func (r *Registry) Open(ctx context.Context, u URI) (fs.File, error)
func (r *Registry) FS(ctx context.Context) fs.FS                       // every mount, through mountfs
func (r *Registry) HostPath(ctx context.Context, u URI) (string, error)
func (r *Registry) Put(ctx context.Context, u URI, data []byte, ifMatch FileVersion) (*WriteResult, error)
func (r *Registry) Remove(ctx context.Context, u URI, ifMatch FileVersion) (*WriteResult, error)
```

- `WithCredentials` takes the [connections](../connections/) service's `TokenSource` method, so the registry asks for a mount's token without importing connections.
- `FS(ctx)` keeps the viewer from `ctx`, because a person's connection gives a token only to its owner. A background job that needs a person's mount passes that person's context.
- `WithInUse` is the app's answer to "do records still point into this mount?" A delete it refuses returns `ErrInUse`. agni#1735 wanted this, and agni doesn't check it yet.
- Before using its list, the registry compares the store's `Generation` with the last one it read, at most once a second. It lists again only when they differ, and keeps its `mountfs.FS` in step with `Mount` and `Unmount`. A process that has just created a mount sees it at once.

## Record stores

A `RecordStore` creates and deletes records, and never changes one in place. Several processes can share a store, and `Generation` is how each one notices the others' changes.

- **Folder,** first. One YAML file per mount, written through a temp file and a rename, and read through `os.Root`. `Generation` is agni's stamp of each file's name, size and mtime. A folder's own mtime isn't enough on Windows.
- **GORM and Datastore,** later, the way `services/backends/` does users. `Generation` is a counter row that each create and delete bumps in the same transaction.

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
- `mounts/table.go`: the registry and the folder record store replace it.
- `cmd/agni/mountmaker.go`'s GitHub code: `resolveRef`, `snapshot`, `tarPath` and the HTTP error mapping. The GitHub driver replaces them. agni keeps `CreateMount` and `DeleteMount` as thin calls into the registry.
- `Record.Files`: the manifest blob replaces it. agni's existing records need a one-time rewrite, which the move's ticket in agni covers.

agni keeps its `Workspace` adapter, its `archivefs` reading, and the flags. `make exercise-mounts` should print the same lines before and after the move.

## The package

{{ apiref "storage/mounts" }}
