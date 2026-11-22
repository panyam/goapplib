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

`Mount` keeps the name, the kind, the source, the ref it was asked for, the version that ref resolved to, the mode, the connection and its owner, and for a snapshot the hash of its manifest. `ConnectionOwner` is empty for a server connection. For a person's connection it names the person, and only they can open the mount, since only they get its token. The library owns those fields. An app keeps anything else in its own records, keyed by the mount's name.

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

A `Driver` serves one kind. It resolves a ref to a version, and opens a mount as an `fs.FS`. It gets `Credentials` from the registry, or nil for a public source, and it knows nothing about connections or viewers.

Each service signs in its own way, so `Credentials` is an interface with one method, `CredentialKind`, and a driver type-switches on the value. GitHub and GCS take a bearer token, which `BearerCredentials` carries. `mounts.Bearer(ts)` wraps an `oauth2.TokenSource`, and `TokenSourceOf` and `BearerToken` unwrap it. S3 signs each request with SigV4 and has no bearer token, so it takes `AWSCredentials`: a key pair, or the AWS default chain, with an optional role to assume. That value holds a resolved secret, and its `String` and `GoString` leave the secret out. A driver given a kind it can't sign with returns `ErrInvalidArgument`, naming the kind. Another package can add a kind by implementing `CredentialKind`, so this package imports no cloud SDK.

Optional interfaces sit on top:

- `Writer` adds `Stat`, `Put` and `Delete` for live mounts. A write passes `IfMatch`, the file's version from `Stat`. Empty means create only, and `AnyVersion` means overwrite whatever is there.
- `Localizer` adds `HostPath` for code that needs a real file, such as agni running `kicad-cli`. A folder mount has one. A GitHub, S3, GCS or msgraph snapshot has one when its driver has a views folder and a blob store that can link. A mount with no folder here returns `ErrNotLocal`, so code that needs a host path fails clearly on GAE or in wasm.
- `Preparer` adds `Prepare`, which the registry calls when a mount is made, after `Resolve`. A GitHub snapshot downloads its files there and sets the mount's `Manifest`. If `Prepare` fails, no record is written.
- `VersionReader` reads a mount's source at any version, not only the one a snapshot pins (#172). Say agni links a design's schematic to a repo at a commit, then moves the link to a newer commit. It needs that commit's files, and it shouldn't download the sheets that didn't change. `Moves` says whether a ref moves: a branch or the default does, and a tag or a commit doesn't. `ListAt` lists the regular files at a version under a path, each with a content ID, the git blob id on GitHub. `FetchAt` puts one file in the blob store and says whether it had to download it.
- `RefLister` lists the versions a source has, so an app can offer revisions to link to (#177). `ListRefs` lists tags newest first, or the commits on one branch newest first, a page at a time. Each `Ref` has the name to put in a mount's `Ref`, the commit it points at, the commit's date and the commit's first line. A source with no history returns an empty page.
- `SourceMatcher` says when two sources name one place, where string equality isn't enough. GitHub's owner and repo names ignore case, so `github://Acme/Board` and `github://acme/board` match.
- `WebhookManager` makes a source send the app a webhook on each change, so the app syncs a mount when its branch moves rather than polling it (#185). `EnsureWebhook` makes the hook, or updates the one already sending to the same URL, so calling it twice leaves one hook. `DeleteWebhook` removes the hooks sending to a URL, and a hook that is already gone isn't an error. A hook is named by its URL, so the app keeps no hook id.

The folder driver is `storage/mounts/folder`. Its mounts are live, writable and local, and it refuses a link anywhere in a path, so a link inside the folder can't reach outside it. A file's version is the SHA-256 of its contents. A write goes through a temp file and a rename, under a lock per file. So its `IfMatch` check holds between writers in one process, sharing one `folder.Driver`. Two processes writing one file can both pass the check, and the second rename wins. By default the folder driver refuses every link. `folder.WithLinksInside()` lets reads follow a link whose target stays inside the folder, in listings, `Open`, `HostPath`, `ListAt` and `FetchAt`, so a folder whose libraries are links still reads (#175). A link that leads out, directly or through another link, is still refused and left out of listings, and writes never go through a link. Go's `os.Root` decides what stays inside, and it counts a link with an absolute target as leading out, even when the target is inside the folder. A walk in `ListAt` visits each real folder once, so two links that make a loop don't repeat it. It's a `VersionReader` too, once `folder.WithBlobs` gives it a blob store. A folder has no history, so every ref moves, and `ListAt` reads the folder as it is now.

A checkout made without `git lfs pull` holds a pointer in place of each LFS file, and the folder has no remote to fetch the object from. So `Open`, `FetchAt` and `HostPath` on a file refuse a pointer with `ErrLFS`, naming the path. `HostPath` on a folder returns its path without looking inside, so a native tool given a folder can still meet a pointer.

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
- Say a repo keeps its board files in Git LFS. GitHub's tarball, raw host and Contents API all send each LFS file as a short pointer, and a reader would get that pointer with no error (#203). So the driver checks every file under 1 KB with `mounts.ParseLFSPointer`. A snapshot, `FetchAt` and a live read each swap a pointer for its object. The driver asks the repo's LFS batch API, at `<host>/<owner>/<repo>.git/info/lfs/objects/batch`, with the mount's token as the basic-auth password. The host is `https://github.com` for GitHub's own API, or the API root without `/api/v3` for an Enterprise host, and `WithLFSURL` overrides it. The download goes to the `href` GitHub returns, with only the headers GitHub gives for it. The bytes must match the pointer's size and SHA-256, and a blob's hash is that same SHA-256, so an object the blob store holds already isn't downloaded again. An object's size counts toward the limits, not the pointer's. A pointer that can't be resolved fails the snapshot or the read with `ErrLFS`, naming the path, plus `ErrNotFound` for a missing object or `ErrPermissionDenied` for a 401 or 403. GitHub answers a repo over its LFS quota with 403 too, and the error carries GitHub's message, which tells the two apart. `ListAt` reports an LFS file's object size, not its pointer's (#208). When a `.gitattributes` at the commit has `filter=lfs`, it reads each blob under 1 KB once. Without one, it reads only blobs of 125 to 144 bytes, the size of a pointer with no extension lines. A repo can set its LFS paths in `.git/info/attributes`, outside the tree, and `FetchAt` swaps every pointer it meets, so `ListAt` has to look too (#218). It remembers by git blob id whether each blob was a pointer, so a later commit reads only the small blobs that changed.
- `ListRefs` lists tags through one GraphQL query per page, ordered by `TAG_COMMIT_DATE`. The REST tags API gives names and commits but no dates, so it can't order them. GitHub's GraphQL API refuses a request without a token, so on a mount with no connection the driver lists tags through the REST tags API instead (#182). GitHub orders those by version, newest first, so `v1.10` comes before `v1.9`. They come with no dates, so the driver reads each tag's commit too, and a page holds at most 10 tags, since every request counts against GitHub's limit of 60 an hour without a token. When GitHub refuses for that limit, the error says a connection lifts it. GitHub orders by the commit's day and then by name, so the driver sorts each page by time. Two tags made on one day can still come out of order across a page break. Commits come from the REST commits API, which needs no token for a public repo. The GraphQL endpoint is `<root>/graphql`, or `<host>/api/graphql` for a GitHub Enterprise root ending in `/api/v3`.
- `EnsureWebhook` lists the repo's hooks, then updates the one sending to the URL or makes a new one, with `content_type` set to JSON so `ParsePush` reads each delivery as sent. It always sends the update, since GitHub never shows a hook's secret. Say a person's token can read the repo but isn't an admin on it. GitHub answers 404 from the hooks API, so the driver returns `ErrPermissionDenied` for 404 and 403, with a message naming admin rights or a fine-grained token's webhooks permission. GitHub's 422, such as for a URL it won't send to, becomes `ErrInvalidArgument`. One page of 100 holds every hook, since GitHub allows 20 per event.
- `github.ParsePush(r, secret)` reads a GitHub webhook delivery (#179). It checks `X-Hub-Signature-256` against the raw body in constant time before reading anything from it, reads at most 25 MB, and takes a JSON body or a form-encoded one. It returns a `Push`, which is a `mounts.Change` plus GitHub's delivery id. A bad signature is `ErrBadSignature` (answer 401), a ping is `ErrPing` (answer 200), and any other event is `ErrNotPush`. The app resolves the secret from a secret reference, as connections do.
- 404 becomes `ErrNotFound`, which is also what a private repo answers without a valid token. Rate limits, rejected tokens and server errors become `ErrUnavailable`. An error from the connection's token source, such as a person who has to connect again, passes through unchanged.

## Bucket drivers

Say agni links a design from `s3://designs/boards/jetson`. A bucket has no commit, since no change spans several objects at once. So `Resolve` lists the prefix and records each object's version: its version id where the bucket keeps versions (S3 `VersionId`, GCS `generation`), or its ETag where it doesn't. The listing goes in the blob store, and its hash is the mount's version. A snapshot's `Prepare` points `Manifest` at a one-entry manifest whose entry is the listing, so `BlobRefs` keeps the listing through a sweep. `Open` on a snapshot then reads exactly the listed versions. Where an object is pinned by its ETag and has changed since, opening it fails with an `fs.PathError` naming the file and wrapping `ErrConflict`. A snapshot never serves new bytes as the old version. A live mount reads each object's latest version, and lists a folder each time it is read.

The version doubles as a change check. A poll calls `ResolveAt`, and a new version means an object under the prefix was added, removed or changed. Bucket event notifications can replace the poll (see Bucket events below). A bucket has no refs, so a non-empty ref is `ErrInvalidArgument`, and a prefix with no objects under it is `ErrNotFound`. For `VersionReader`, `ListAt` reads a stored listing, each file's ID is `version:<id>` or `etag:<etag>`, and `FetchAt` copies that pinned version into the blob store.

`storage/mounts/bucket` holds this logic once, over a three-method `Bucket` interface (`List`, `ListDir`, `Get`), and imports no SDK. `bucket/buckettest` is an in-memory `Bucket`, with or without versions. The SDK drivers each live in their own module, so an app that doesn't mount a bucket never downloads an SDK:

```go
s3d := s3.New(blobStore)                 // github.com/panyam/goapplib/storage/drivers/s3
gcsd := gcs.New(blobStore)               // github.com/panyam/goapplib/storage/drivers/gcs
reg := mounts.NewRegistry(store, []mounts.Driver{github, s3d, gcsd}, mounts.WithCredentials(conns.Credentials))
```

- **S3** (`KindS3`, `s3://<bucket>/<prefix>`) signs with `AWSCredentials`, and with no credentials it reads a public bucket unsigned. It lists with `ListObjectVersions` and pins each `VersionId`. On a bucket with versioning off or suspended, the version id is `null`, and it pins the ETag and reads with `If-Match`. A connection without `s3:ListBucketVersions` gets a refusal there, and the driver falls back to `ListObjectsV2` and ETags. Clients are kept per set of credentials, so an assumed role's session is reused until it expires. `WithEndpoint` points it at an S3-compatible store such as MinIO, with path-style addressing.
- **GCS** (`KindGCS`, `gs://<bucket>/<prefix>`) signs with `BearerCredentials`. Every GCS object has a generation, so a snapshot always pins generations. With versioning on, the bucket keeps the old generation, and the snapshot keeps serving it. With versioning off, GCS drops it, and the read fails with `ErrConflict`.

### Views of bucket snapshots

Say agni links a folder in S3 as a snapshot, and its engine opens real files. A bucket snapshot's listing pins each object's version, but the contents stay in the bucket. A view needs them on disk, so `s3.WithViews(dir)`, `gcs.WithViews` and `msgraph.WithViews` (all `bucket.WithViews`) make `Prepare` fetch them (#220).

- With a views folder and a store that is a `blobs.Linker`, `Prepare` downloads every listed object at its pinned version into the store. The manifest lists each one under `files/`, beside the listing, so `BlobRefs` keeps them through a sweep. Creating the mount downloads all of it, up to `WithLimits`.
- `Open` on such a snapshot reads from the blob store and never asks the bucket. A changed object on a bucket without versions no longer fails the read, since the bytes were captured at `Prepare`.
- `HostPath` builds `<views>/<name>/<version hex>/` from links to those blobs the first time it's asked. A restart that lost the folder rebuilds it from the store, without credentials or the bucket.
- An object that changed between `Resolve` and `Prepare` on a bucket without versions fails `Prepare` with `ErrConflict`, and no record is written.
- A live mount, a driver without views, or a store that can't link returns `ErrNotLocal`. So does a snapshot made before the driver had views, since its manifest has no files. Making it again fixes it.

`HostPath` takes no credentials, so it can't reach the bucket. The fetch happens in `Prepare`, which has them. Fetching then also keeps the bytes the listing pinned, where a later fetch would meet `ErrConflict` on a bucket without versions. GitHub and the bucket drivers share `mounts.BuildView`, which builds in a temp folder and renames, so nobody sees a half-built view.

With blobs in a bucket, the store is a `blobs.Cached` (see the blobs page), and the view links to its local copies.

A view downloads the whole source, which suits a tool that opens neighbouring files, as a KiCad project does. Say instead the bucket holds 1000 files and a tool needs one of them on disk. Leave out `WithViews`, so creating the mount downloads nothing, and fetch that one file at the snapshot's version:

```go
files, _ := reg.ListAt(ctx, m.Name, m.Version, "board/main.kicad_sch")
h, _, _ := reg.FetchAt(ctx, m.Name, m.Version, files[0])
_ = store.Link(h, filepath.Join(workDir, "main.kicad_sch")) // store is the driver's blobs.Linker
```

`ListAt` with a file's path lists just that file. `FetchAt` downloads it at the pinned version, so a later change in the bucket doesn't reach it. The app keeps `h` in its own records and passes a referrer to `Sweep`, since the mount's manifest doesn't list the file. A mode where `HostPath` fetches only the files it's asked for waits on a second app ([#225](https://github.com/panyam/goapplib/issues/225)).

### Bucket events

Say agni follows `s3://designs/boards/jetson`, and someone uploads a new `power.kicad_sch` there. Polling `ResolveAt` finds it on the next tick. A bucket event finds it within seconds (#210). The bucket sends an event for one object, and each driver module turns that event into a `mounts.Change` whose `Source` names the object, such as `s3://designs/boards/jetson/power.kicad_sch`. `bucket.Driver` is a `mounts.SourceContainer`, so `Registry.Following` returns every mount whose prefix holds the object. Here that's the `jetson` mount and a mount of `s3://designs`, but not one of `s3://designs/boards/jet`. The change's `Ref` and `Version` are empty, since an event doesn't say what the listing's hash becomes. The app calls `ResolveAt` on each mount it gets back, as it does after a GitHub push.

- `s3.ParseEvent(body)` reads S3's event JSON, as S3 sends it to SQS, or wrapped in SNS's envelope. Created and removed objects become changes. S3's test event and other event types give none. An app polling SQS calls it on each message body.
- An SNS subscription pushes to the app over HTTPS. `s3.SNSVerifier.ParseSNS(r)` checks the message's signature first, SignatureVersion 1 (SHA1withRSA) or 2 (SHA256withRSA), against the certificate at its `SigningCertURL`. It fetches that certificate only over https from a host named `sns.<region>.amazonaws.com` (or `.amazonaws.com.cn`), before any request, and caches it per URL. Anything else is `ErrBadSignature`. A subscription request returns the message with `ErrSubscriptionConfirmation`. The app calls `ConfirmSubscription` to accept it, and that refuses a `SubscribeURL` that isn't on an SNS host too.
- `gcs.ParsePush(r, verifier)` reads a Pub/Sub push of a GCS notification. `OBJECT_FINALIZE`, `OBJECT_DELETE` and `OBJECT_METADATA_UPDATE` become changes, and other event types give none. The push must carry the subscription's OIDC token. `gcs.GoogleVerifier{Audience, Email}` checks it against Google's keys with oneauth's `client.VerifyIDToken`, which fetches the keys on first use and again on an unknown key id. A missing or refused token is `ErrUnauthenticated`, and a nil verifier is `ErrInvalidArgument`, since an open endpoint would let anyone make the app re-resolve its mounts.

Both run `mountstest.RunDriver` against in-process fakes, gofakes3 and fake-gcs-server. gofakes3 ignores `If-Match` on a GET, and the S3 test wraps it to answer 412 as S3 does. Another wrapper refuses `ListObjectVersions` with 403, so a test checks the fallback to ETag pins. `TestLiveS3` and `TestLiveGCS` read a real bucket when `STORAGE_LIVE_S3` or `STORAGE_LIVE_GCS` names a source, signing with the AWS default chain or Application Default Credentials.

### SharePoint and OneDrive

The msgraph driver (`KindMSGraph`, its own module at `storage/drivers/msgraph`) reads a folder in a SharePoint document library or a OneDrive through Microsoft Graph, with plain HTTP and no Graph SDK (#206). A source names a drive and a folder in it:

```
msgraph://sites/<site-id>/drives/<drive-id>/root/Boards
msgraph://drives/<drive-id>/root/Boards
msgraph://me/drive/root/Boards
```

It implements `bucket.Bucket`, so snapshots, `VersionReader` and the contract suite come from the bucket package. The source's first segment is the "bucket", and the rest is the key. A listing walks the folder's `children`, a folder at a time, following `@odata.nextLink`. A next link off Graph's own host fails with `ErrUnavailable`, so a link in an answer can't carry the token elsewhere.

A snapshot pins each file by its item id, its content tag, its size and its last-modified time. The content tag is `cTag`, which Graph changes only when the bytes change, or `eTag` where Graph sends no `cTag`, since `eTag` moves on a rename too. Say a design's README changes after the snapshot. A pinned read finds that the item's content tag moved, then looks in the item's version history for the entry with the pinned modified time and size, and reads that version's content. A library that keeps no versions has no such entry, and the read fails with `ErrConflict` and the path named. A snapshot makes no request per file up front. Listing a folder takes one request per page, and only a changed file needs the version lookup.

Graph tracks changes per drive. `Delta(ctx, m, creds, token)` asks the drive's delta feed what changed since `token`, and returns the files under the mount's folder plus the next token, which the app keeps beside the mount. An empty token returns no items and a token for now. SharePoint leaves the path off a deleted item, so a deletion comes back with its item id and an empty path. A token Graph has forgotten returns `msgraph.ErrResyncRequired` (Graph's 410), and the app reads the mount again in full. Change notifications are the push form. `CreateSubscription` asks Graph to POST to the app's URL when the drive changes, and `RenewSubscription` moves its expiry, which Graph caps at about 30 days. The app owns the endpoint: `AnswerValidation` answers Graph's validation request as `text/plain`, and `ParseNotification` checks each entry's `clientState` in constant time and returns a `mounts.Change` naming the drive root. The driver is a `mounts.SourceContainer` that turns the bucket rule around: a change to a drive holds every mount in it, so `Registry.Following` returns each of the app's mounts in that drive. The app then calls `Delta` on each to learn which files changed.

The driver signs with `BearerCredentials` from an `entra` or `entra-user` connection. `msgraph/graphtest` is a fake Graph with drives, paged children, content behind a redirect to a download URL, version history, the delta feed and subscriptions. `TestLiveMSGraph` reads a real drive when `STORAGE_LIVE_MSGRAPH` names a source and `STORAGE_LIVE_MSGRAPH_TOKEN` holds a Graph token.

`mountstest.RunDriver` is the contract suite for drivers, as `RunRecordStore` is for stores. A fixture gives a driver, a source holding the files the suite asks for, a missing source, credentials, and a way to change a file. For each mode the driver lists, it checks that `Open` passes `fstest.TestFS`, refuses paths that climb, and, after a change, that a live mount shows it and a snapshot doesn't. A fixture whose snapshots can't keep old bytes sets `SnapshotFailsOnChange`, and the suite then wants the read to fail with the path named. `HostPaths` lists the modes that get a folder on disk. The suite then reads every file through `HostPath`, refuses paths that climb, and wants `ErrNotLocal` for the other modes. With `Views` and `Offline` set, it removes the views, takes the source away, and still wants the view and `Open` to give the pinned bytes. The folder, GitHub, S3, GCS and msgraph drivers all run it, and the bucket drivers run it once more with views.

`storage/mounts/github/githubtest` is a fake GitHub for tests, the library's and an app's own. It serves a repo's commits, branches, tags, trees, blobs, tarballs and contents from memory, the GraphQL tags query, and its raw host at `RawURL()`, and each write makes a real commit. `Tag` and `SetCommitTime` set up a repo's tags and dates. `PushRequest` builds a signed push for a branch's head, and `SignedRequest` builds any other event, so an app tests its webhook handler end to end. A private repo answers only an accepted token, and every request is recorded with the token it carried. `Repo.Grant(tokens...)` limits a private repo to those tokens. Any other token gets 404 from the REST routes, the raw host and GraphQL, as GitHub answers for a repo the token can't see. So a test can check that one person can't read another person's repo. A private repo with no grants answers any accepted token. It serves the hooks API too. `Repo.Hooks()` returns each hook with its secret, content type, events and the token that last changed it. `Repo.HookManagers(tokens...)` limits who may manage the hooks, and gives any other token 404, as GitHub does without admin rights. `Repo.Push(branch, put, del)` commits, then delivers a signed push to each active push hook before it returns, so a test runs push, webhook and sync with no hand-built request. GitHub delivers in the background, so an app's own retry logic isn't exercised here. `Repo.CommitLFS(branch, path, content)` commits an LFS pointer and keeps the object behind the fake's batch API at `LFSURL()`. `RemoveLFSObject`, `FailLFS` and `CorruptLFS` set up a missing object, a refused batch and a download that doesn't match its pointer. oneauth's `outbound/githubtest` mints and refreshes tokens, and `AcceptToken` lets this fake trust the ones it mints.

`New` starts the fake on a fresh `127.0.0.1` port. A test whose app runs in a container can't reach that port, and can't follow the absolute URLs the fake hands out. So `WithListener(l)` serves the fake on a listener the test opens, such as one on `0.0.0.0:0`. `WithBaseURL(u)` sets the origin of every URL the fake hands out: `URL`, `RawURL()`, `LFSURL()`, the LFS batch hrefs, pagination `Link` headers and the device flow's `verification_uri`. Pick an origin the container can reach, such as `http://host.docker.internal:<port>`. The test process may not resolve that name, so it calls the fake at `ListenURL()`.

A real GitHub App can do less than the person who signs in through it. `AddApp` returns the `App`, and `SetPermissions` and `Install(account, repos...)` model those limits. A sign-in token through the App then reads a private repo only where the App is installed and the person can read it, and gets 404 anywhere else. A file route needs the `contents` permission, a write needs it at `write`, and the org membership check needs `members`. Without the permission the API answers 403 "Resource not accessible by integration", as GitHub does. `POST /app/installations/{id}/access_tokens` takes a JWT signed with the App's key (`KeyPEM()`), so oneauth's `outbound.GitHubAppInstallation` gets an installation token from the fake, and that token reads the installation's repos. `SetWebhook` gives the App a webhook, and `Repo.Push` delivers to it with the installation in the body. An App with no permissions or installations signs people in and limits nothing, as before.

An exercise that runs the app outside the Go test process can start a seeded fake as a command. `go run github.com/panyam/goapplib/storage/mounts/github/githubtest/cmd/fakegithub --seed seed.json` builds the users, App, tokens and repos the seed file names, as `githubtest.Seed` describes. `--listen` and `--base-url` place it for a container. Its control address answers `GET /` with the fake's addresses, each tag's commit and the App's key and installations. `POST /approve` approves a device code, and `POST /push` commits a folder and delivers the push. A Go test gets the same fake from `LoadSeed` and `Seed.Apply`.

A test against real GitHub reads `octocat/Hello-World`, and runs with `STORAGE_LIVE_GITHUB=1`. With `STORAGE_LIVE_GITHUB_TOKEN` set too, it lists `golang/go`'s tags. `STORAGE_LIVE_GITHUB_LFS=owner/repo:path` reads one LFS file through a snapshot and a live mount.

The fake also serves a GitHub App's sign-in, so an app tests its GitHub login against the same host its repos live on. Point oneauth's `GithubOAuth2` at it with `SetEnterpriseURL(fake.URL)`. Every API route also answers under `/api/v3`, as on a GitHub Enterprise Server host.

- `AddApp(clientID, secret)` registers the App. Any other client id, or a wrong secret, gets GitHub's `incorrect_client_credentials`.
- `AddUser(login, orgs...)` makes a person. `/user` answers their `login`, `id` and `name`, and `/orgs/{org}/members/{login}` answers 204 for each of `orgs` and 404 otherwise.
- In the web flow, `SignInAs(login)` picks who is signed in at the authorize step. Without it, authorize redirects back with `error=access_denied`. A code works once, and only with its PKCE verifier and redirect URI.
- In the device flow, `User.Approve(userCode)` stands in for the person typing the code. Until then a poll gets `authorization_pending`, and a poll sooner than the interval gets `slow_down`. The interval is one second, and `SetDeviceInterval` changes it.
- Each sign-in issues an access token and a refresh token. A refresh issues a new pair and retires both old tokens. `ExpireTokens(login)` makes the person's access tokens get 401, so a test can watch the app refresh.
- `Repo.GrantUser(logins...)` and `Repo.HookManagerUsers(logins...)` name a person rather than a token, so the grant holds across refreshes. They add to `Grant` and `HookManagers`.

GitHub answers every OAuth error with status 200 and an `error` field, and so does the fake. `x/oauth2` reads it as a `*oauth2.RetrieveError`. The error names come from GitHub's docs, since `TestLiveGitHub` can't sign in without a person at a browser.

## The registry

`Registry` joins the fixed mounts from the server's configuration, the runtime mounts from a `RecordStore`, and the drivers, and an app goes through it for everything. A nil store serves the fixed mounts alone, for an app with no store of its own, such as agni's CLI, and `Create` and `Delete` then return `ErrInvalidArgument`:

```go
store, _ := fsstore.New(filepath.Join(storeDir, "mounts"))
reg := mounts.NewRegistry(store, []mounts.Driver{folder.New(), github},
	mounts.WithFixed(mounts.Mount{Name: "boards", Kind: mounts.KindFolder, Source: "/srv/boards", Mode: mounts.ModeLive}),
	mounts.WithCredentials(conns.Credentials),
	mounts.WithInUse(designs.PointsInto),
)
```

- `Create` resolves the ref, calls `Prepare`, sets `Version` and `Created`, and writes the record. It refuses `KindFolder`, a mode the driver doesn't serve, a name a fixed mount has, and a `ConnectionOwner` with no `Connection`.
- `Delete` refuses a fixed mount with `ErrFixed`. It asks the app's `WithInUse` check whether records still point into the mount, and refuses with `ErrInUse` when they do. agni#1735 wanted this, and agni doesn't check it yet.
- `Open`, `Stat`, `HostPath`, `Put` and `Remove` work on one file. A write to a snapshot mount returns `ErrReadOnly` before it reaches the driver, and one over `WithLimits`' `MaxBytes` returns `ErrTooLarge`.
- `FS(ctx)` returns every mount as one `fs.FS` through goutils' `mountfs`, with each mount's driver opened when a path first reaches it. It keeps `ctx` for those opens, because a person's connection gives a token only to its owner. An app calls `FS` per request. A background job that needs a person's mount passes that person's context.
- `WithCredentials` takes the [connections](../connections/) service's `Credentials` method, so the registry asks for a mount's credentials without importing connections. It passes the mount's `ConnectionOwner` and `Connection`.
- Before using its list, the registry compares the store's `Generation` with the last one it read, at most once a second (`WithRecheck` changes that). It lists again only when they differ. A registry sees its own creates and deletes at once. If the store fails after a good read, the registry keeps serving the last good list.
- `ResolveAt`, `ListAt` and `FetchAt` call a `VersionReader` driver with the mount's credentials, so an app reads a private repo at any commit through the mount's connection. A driver that can't read versions returns `ErrInvalidArgument`.
- `ListRefs` calls a `RefLister` driver the same way.
- `EnsureWebhook(ctx, name, req)` and `DeleteWebhook(ctx, name, url)` call a `WebhookManager` driver the same way. They refuse with `ErrInvalidArgument` a URL that isn't absolute http or https, an empty secret (an app can't verify an unsigned delivery), a mount with no connection (a host doesn't let an anonymous caller manage hooks), and a kind whose driver has none. An empty `Events` means push.
- `Following(ctx, kind, change)` returns the mounts a `Change` moves. A change says that a ref in a source moved, such as a push to a branch. A mount matches when its source names the same place and its ref is the changed one, or is empty and the change is to the default branch. For a `SourceContainer` driver, such as a bucket's, a mount matches when the change's source lies under its own, and the bucket change's empty ref matches the bucket mount's. A mount pinned to a commit never matches. An app's webhook handler turns the host's event into a `Change`, as `github.ParsePush` does, and then calls `ResolveAt` on each mount it gets back. A folder fits the same shape: a poller that sees a folder's version move can make a `Change` too.
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
| `ErrPermissionDenied` | `ErrPermissionDenied` |
| `ErrLFS` | the sentinel it also wraps, or `ErrFailedPrecondition` for a folder |

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
