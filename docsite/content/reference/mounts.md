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
- The empty path is the mount's root, and `String` writes it as `mount://<name>`.

`Resolve(ref)` reads a reference against a URI the way a browser reads an href against its page, which is how agni's readers find a schematic's child sheets and a design's libraries (#175). A full `mount://` URI replaces the whole thing, `/x` names `x` from the mount's root, and anything else resolves against the URI's folder. A result that climbs out of the mount is `ErrInvalidPath`. RFC 3986 would drop a leading `..` on `/../x` and read `/x`, and `Resolve` refuses it instead, since quietly fixing a traversal attempt hides it. `Dir`, `Base` and `IsZero` round the type out.

A runtime mount's name, the one a record store keeps, is 1 to 64 characters, a-z and 0-9 first, then also `-`, `_` and `.`. `ValidName` checks it, and it keeps a name safe as a folder name, a URL path element and a YAML file name on any OS. A URI takes any name that isn't empty, `.` or `..` and holds no separator. An app names a fixed mount after a folder, such as agni's `--mount-root`, and a folder can have capitals or spaces in its name.

## Drivers

A `Driver` serves one kind. It resolves a ref to a version, and opens a mount as an `fs.FS`. It gets an `oauth2.TokenSource` from the registry, or nil for a public source, and it knows nothing about connections or viewers. Optional interfaces sit on top:

- `Writer` adds `Stat`, `Put` and `Delete` for live mounts. A write passes `IfMatch`, the file's version from `Stat`. Empty means create only, and `AnyVersion` means overwrite whatever is there.
- `Localizer` adds `HostPath` for code that needs a real file, such as agni running `kicad-cli`. A folder mount has one. A GitHub snapshot has one when its view is hard links on the server's disk. A mount with no folder here returns `ErrNotLocal`, so code that needs a host path fails clearly on GAE or in wasm.
- `Preparer` adds `Prepare`, which the registry calls when a mount is made, after `Resolve`. A GitHub snapshot downloads its files there and sets the mount's `Manifest`. If `Prepare` fails, no record is written.
- `VersionReader` reads a mount's source at any version, not only the one a snapshot pins (#172). Say agni links a design's schematic to a repo at a commit, then moves the link to a newer commit. It needs that commit's files, and it shouldn't download the sheets that didn't change. `Moves` says whether a ref moves: a branch or the default does, and a tag or a commit doesn't. `ListAt` lists the regular files at a version under a path, each with a content ID, the git blob id on GitHub. `FetchAt` puts one file in the blob store and says whether it had to download it.
- `RefLister` lists the versions a source has, so an app can offer revisions to link to (#177). `ListRefs` lists tags newest first, or the commits on one branch newest first, a page at a time. Each `Ref` has the name to put in a mount's `Ref`, the commit it points at, the commit's date and the commit's first line. A source with no history returns an empty page.
- `SourceMatcher` says when two sources name one place, where string equality isn't enough. GitHub's owner and repo names ignore case, so `github://Acme/Board` and `github://acme/board` match.

The folder driver is `storage/mounts/folder`. Its mounts are live, writable and local, and it refuses a link anywhere in a path, so a link inside the folder can't reach outside it. A file's version is the SHA-256 of its contents. A write goes through a temp file and a rename, under a lock per file. So its `IfMatch` check holds between writers in one process, sharing one `folder.Driver`. Two processes writing one file can both pass the check, and the second rename wins. By default the folder driver refuses every link. `folder.WithLinksInside()` lets reads follow a link whose target stays inside the folder, in listings, `Open`, `HostPath`, `ListAt` and `FetchAt`, so a folder whose libraries are links still reads (#175). A link that leads out, directly or through another link, is still refused and left out of listings, and writes never go through a link. Go's `os.Root` decides what stays inside, and it counts a link with an absolute target as leading out, even when the target is inside the folder. A walk in `ListAt` visits each real folder once, so two links that make a loop don't repeat it. It's a `VersionReader` too, once `folder.WithBlobs` gives it a blob store. A folder has no history, so every ref moves, and `ListAt` reads the folder as it is now.

A folder's version is `mtime:` and a SHA-256 over each regular file's path, size and modification time, in path order (#178). `Resolve` computes it with a walk and reads no file's contents. Say agni follows a board folder and polls it every few minutes. It calls `ResolveAt`, and when the version didn't move it reads nothing more. A touch moves the version without changing any bytes, so a moved version only means "go and look". A write that keeps both the size and the mtime doesn't move it, and the driver accepts that. `ListAt` and `FetchAt` take `""` or an `mtime:` version and read the folder as it is now, since a folder can't be read as of an older version. `ListRefs` returns an empty page.

The GitHub driver is `storage/mounts/github`, built with a blob store and options for the API root, the HTTP client, the limits and the views folder:

```go
gh := github.New(blobStore, github.WithViews(filepath.Join(storeDir, "views")))
```

- `Resolve` reads the repo's default branch when the ref is empty, then asks GitHub which commit the ref points at.
- For a snapshot, `Prepare` streams the commit's tarball into the blob store. It keeps regular files only, refuses a path that is absolute, unclean or climbs out, and stops at the limits with `ErrTooLarge`. Then it writes the manifest. `Open` serves the snapshot from the manifest and the blob store without asking GitHub, after a restart too.
- `HostPath` on a snapshot builds a folder of hard links to its blobs, at `<views>/<name>/<commit>`, the first time it is asked. A restart that lost the folder rebuilds it from the manifest.
- For a live mount, `Prepare` fills in the default branch when `Ref` is empty, since a write commits onto a branch. `Open` reads the branch through the Contents API on every open. `Stat` returns a file's git blob sha, and `Put` and `Delete` each make one commit. GitHub refuses a stale sha with 409, and an update without a sha with 422, and the driver returns `ErrConflict` for both.
- For `VersionReader`, `ListAt` reads the trees API once per commit and caches the list per commit and token, so one connection's list never reaches a caller with another token or none. It leaves out links and submodules, and stops at the limits with `ErrTooLarge`. `FetchAt` downloads a file from the raw host by commit and path, which doesn't count against the API's hourly limit of 60 requests without a token. `WithRawURL` sets the host. With a non-default API root, such as GitHub Enterprise, and no raw URL, it uses the git blobs API. Each download is checked against its git blob id before it's kept. When the blob store is a `blobs.Aliases`, as `blobs.Dir` is, the driver records each git blob id against its blob, and skips the download of a file it already holds. That's what makes a second commit's read cheap.
- `ListRefs` lists tags through one GraphQL query per page, ordered by `TAG_COMMIT_DATE`. The REST tags API gives names and commits but no dates, so it can't order them. GitHub's GraphQL API refuses a request without a token, so listing tags on a mount with no connection returns `ErrInvalidArgument`. GitHub orders by the commit's day and then by name, so the driver sorts each page by time. Two tags made on one day can still come out of order across a page break. Commits come from the REST commits API, which needs no token for a public repo. The GraphQL endpoint is `<root>/graphql`, or `<host>/api/graphql` for a GitHub Enterprise root ending in `/api/v3`.
- `github.ParsePush(r, secret)` reads a GitHub webhook delivery (#179). It checks `X-Hub-Signature-256` against the raw body in constant time before reading anything from it, reads at most 25 MB, and takes a JSON body or a form-encoded one. It returns a `Push`, which is a `mounts.Change` plus GitHub's delivery id. A bad signature is `ErrBadSignature` (answer 401), a ping is `ErrPing` (answer 200), and any other event is `ErrNotPush`. The app resolves the secret from a secret reference, as connections do.
- 404 becomes `ErrNotFound`, which is also what a private repo answers without a valid token. Rate limits, rejected tokens and server errors become `ErrUnavailable`. An error from the connection's token source, such as a person who has to connect again, passes through unchanged.

`storage/mounts/github/githubtest` is a fake GitHub for tests, the library's and an app's own. It serves a repo's commits, branches, tags, trees, blobs, tarballs and contents from memory, the GraphQL tags query, and its raw host at `RawURL()`, and each write makes a real commit. `Tag` and `SetCommitTime` set up a repo's tags and dates. `PushRequest` builds a signed push for a branch's head, and `SignedRequest` builds any other event, so an app tests its webhook handler end to end. A private repo answers only an accepted token, and every request is recorded with the token it carried. oneauth's `outbound/githubtest` mints and refreshes tokens, and `AcceptToken` lets this fake trust the ones it mints. A test against real GitHub reads `octocat/Hello-World`, and runs with `STORAGE_LIVE_GITHUB=1`. With `STORAGE_LIVE_GITHUB_TOKEN` set too, it lists `golang/go`'s tags.

## The registry

`Registry` joins the fixed mounts from the server's configuration, the runtime mounts from a `RecordStore`, and the drivers, and an app goes through it for everything. A nil store serves the fixed mounts alone, for an app with no store of its own, such as agni's CLI, and `Create` and `Delete` then return `ErrInvalidArgument`:

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
- `ResolveAt`, `ListAt` and `FetchAt` call a `VersionReader` driver with the mount's token, so an app reads a private repo at any commit through the mount's connection. A driver that can't read versions returns `ErrInvalidArgument`.
- `ListRefs` calls a `RefLister` driver the same way.
- `Following(ctx, kind, change)` returns the mounts a `Change` moves. A change says that a ref in a source moved, such as a push to a branch. A mount matches when its source names the same place and its ref is the changed one, or is empty and the change is to the default branch. A mount pinned to a commit never matches. An app's webhook handler turns the host's event into a `Change`, as `github.ParsePush` does, and then calls `ResolveAt` on each mount it gets back. A folder fits the same shape: a poller that sees a folder's version move can make a `Change` too.
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

## What agni deleted when it moved over

agni moved onto the library in agni#1762, at storage v0.7.15. It deleted these:


- `mounts/mounts.go`'s `Mount`, `Kind`, `SafeResolve` and `Resolve`, and `artifact/uri.go`. `ParseURI`, `URI.Resolve`, `Dir` and `Base`, and `Registry.HostPath` replace them. A registry with no store covers agni's CLI, and `folder.WithLinksInside` keeps its `--mount` folders with links working.
- `mounts/table.go`: `mounts.Registry` and `fsstore` replace it. agni's records gained `version` and `mode`, and its `commit` key became `version`.
- `cmd/agni/mountmaker.go`'s GitHub code: `resolveRef`, `snapshot`, `tarPath` and the HTTP error mapping. The GitHub driver replaces them. agni kept `CreateMount` and `DeleteMount` as thin calls into the registry.
- `cmd/agni/linkreaders.go`, from agni's linked artifacts: `Registry.ResolveAt`, `ListAt` and `FetchAt` replace its GitHub and folder readers, its tree cache and its git blob id index. agni's `exercise-linked-sources` shows a move to a newer commit downloading only the 17 changed files.
- `Record.Files`: the manifest blob replaces it. At startup, agni rewrites each record an older agni wrote once, and its file list becomes a manifest blob.

agni kept its `Workspace` adapter, its `archivefs` reading, and the flags, in a package now called `mountset`. `make exercise-mounts` printed the same lines before and after the move, except the uploads size `du` reports. The new manifest blob adds 6.7 KB to the uploads.

## The package

{{ apiref "storage/mounts" }}
