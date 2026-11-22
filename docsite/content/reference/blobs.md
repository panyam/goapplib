---
title: "Blobs"
description: "The design of the storage module's blob store: files kept once by their SHA-256, never changed, linked into views, and removed only by a sweep that every owner of blobs takes part in."
---

Picture agni snapshotting a repo at a commit, then snapshotting the next commit a week later. Most files didn't change, so most of the second snapshot is bytes the server already has. agni keeps each file once, named by the SHA-256 of its contents, and builds each snapshot's folder out of hard links to those files. A sweep removes a file only once nothing points at it.

This page is the design for [#154](https://github.com/panyam/goapplib/issues/154). The `storage/blobs` package holds the store interface, a folder store, and the manifest that lists a snapshot's files, rendered at the bottom of the page. It starts from agni's `BlobDir` (agni#1739). [Mounts](../mounts/) uses it for GitHub snapshots, and an app can use it for uploads, as agni does for designs.

## Where blobs live

Blobs get a package of their own, rather than sitting inside the GitHub driver, because the driver isn't their only owner. agni's uploaded designs are blobs in the same store, and one sweep has to see both.

A hash is `sha256:` and 64 lowercase hex digits, agni's format. A malformed hash fails `ParseHash`, and nothing that fails it can name a file, so no hash reaches outside a store's folder.

## The interface

`Store` has four methods:

- `Put` streams the contents in and returns their hash. Putting contents the store already holds writes nothing new. It does refresh the blob's age, though. Without that, a sweep could remove an old blob in the moment after a client uploaded it again and before the client's record named it. agni's `BlobDir` has that gap.
- `Open` returns a reader that hashes as it goes. If the bytes no longer match, the last read returns `ErrCorrupt` instead of `io.EOF`. Only a caller who reads to the end gets the check, which is the case for every copy and every hash.
- `Has` reports whether a blob is there.
- `Sweep` removes blobs. The next section covers it.

`Linker` adds `Link`, for a store on the server's disk. It hard-links a blob into a folder, so a snapshot's view takes no extra space, and copies when the filesystem refuses links. A linked file keeps its contents after the sweep removes the blob, because a hard link is a second name for the same file.

## The sweep

Several things own blobs in one store: an app's designs, and the mounts registry's snapshots. No one of them knows the whole set in use. So `Sweep` takes a `Referrer` from each owner, a function that calls `keep` for every hash the owner still names. The store keeps the union. It also keeps anything written within the grace period, which spares a blob a caller has put but not yet named.

If any referrer fails, `Sweep` removes nothing and returns the error. A referrer that couldn't reach its own records doesn't know what is in use, and treating that as "nothing" would delete live files.

The folder store's sweep also removes temp files older than the grace period, which a crashed `Put` left behind.

## Manifests

A snapshot can have 100,000 files, and a mount record can't hold a list that long in Datastore. So a snapshot's file list is a blob too. `Manifest` lists each file's path, hash and size. `PutManifest` sorts it by path before storing it, so the same files give the same hash in any order. It refuses paths that are absolute, unclean, climbing or duplicated.

A mount record keeps one hash, the manifest's. `ManifestRefs` expands that hash into the manifest plus every file it lists. The mounts registry's `BlobRefs` does that for every snapshot mount, and passes the hashes to `Sweep`.

## The folder store

`blobs.Dir` keeps a blob at `<root>/sha256/ab/cdef...`. That is agni's layout, so agni can point `NewDir` at its existing `uploads/blobs` folder and keep every blob it has. Writes go through a temp file and a rename in the same folder, so a reader sees a whole blob or none, and several processes can share one `Dir`. Reads and removals go through `os.Root`, so a link planted in the folder can't lead a sweep outside it.

## Bucket stores

Say agni runs on Kubernetes with no persistent volume. It keeps uploads in a bucket with `s3blobs.New(ctx, "s3://acme-agni/uploads", creds)` or `gcsblobs.New(ctx, "gs://acme-agni/uploads", creds)` (#205). The packages live in the S3 and GCS driver modules, `storage/drivers/s3/s3blobs` and `storage/drivers/gcs/gcsblobs`, so an app that uses a cloud for both mounts and blobs downloads its SDK once. The credentials come from an `aws` or `gcp` connection, through `connections.Service.Credentials`, as the mount drivers' do.

- Each blob is the object `<prefix>/sha256/ab/cdef...`, the same layout `Dir` uses on disk (`blobs.Key`). So `aws s3 sync uploads/blobs s3://acme-agni/uploads` moves an existing `Dir` into a bucket.
- A bucket needs the object's name before the upload, and the name is the hash. So `Put` first copies its input into a local temp file and hashes it (`blobs.Spool`), then uploads once. `WithTempDir` sets where, and it needs room for the largest blob.
- The upload is conditional, with `If-None-Match: *` on S3 and `ifGenerationMatch=0` on GCS. Two servers putting the same bytes never overwrite each other. The loser finds the blob there, and moves its written time forward in the object's metadata, so the sweep's grace period starts again as `Put` promises. On S3 that copies the object onto itself, since S3 can't change metadata in place.
- `Open` checks the bytes against the hash as it reads, through `blobs.Verify`, as `Dir` does.
- `Sweep` lists `<prefix>/sha256/` and ages each unreferenced blob by its written time, or by the object's own time for a blob copied in some other way. On S3 it reads each unreferenced blob's metadata with a HEAD, since a listing leaves metadata out. Objects under the prefix that aren't named like blobs are left alone.
- They implement `Store` without `Linker`, since a bucket can't hard-link. On its own, a snapshot over them serves an `fs.FS` and has no host path. An app that needs files on disk puts `blobs.Cached` in front (see below).

`blobs/blobstest.RunStore` checks any `Store` against this page's contract: round trips, a missing or malformed hash, corruption, the refresh on a second `Put`, the sweep's grace period and referrers, and concurrent `Put`s of the same bytes. `Dir`, `s3blobs` and `gcsblobs` all run it, the bucket stores against gofakes3 and fake-gcs-server. Live tests run with `STORAGE_LIVE_S3=s3://<bucket>/<prefix>` or `STORAGE_LIVE_GCS=gs://<bucket>/<prefix>`, using each cloud's default credentials.

## A local cache in front of a bucket

Say agni keeps its blobs in S3, and its engine opens real files. A view needs blobs on the server's disk, so it needs a `Linker`, and `s3blobs` isn't one. `blobs.NewCached(remote, localDir)` wraps the bucket store with a local `Dir`, and the result is a `Linker` (#219).

- `Put` writes the local copy, then uploads that copy, and returns after both. A pod that dies after `Put` loses nothing.
- `Open` and `Link` serve the local copy. On a miss they fetch the blob from the remote and keep it. The remote's reader checks the hash, so a corrupt remote blob fails with `ErrCorrupt` and is never kept.
- `Has` asks the local copy first, then the remote.
- `Sweep` calls the referrers once. It sweeps the remote with what they named, then the local `Dir` with the same names and grace. A local copy can outlive its remote blob by one sweep, when it was fetched after the remote wrote it. `WithMaxAge(d)` also drops any local copy fetched or put more than `d` ago, since the remote still holds it.
- A pod that restarts with an empty disk refills the local folder on demand.

```go
remote, _ := s3blobs.New(ctx, "s3://acme-agni/uploads", creds)
local, _ := blobs.NewDir("/var/cache/agni/blobs")
store := blobs.NewCached(remote, local, blobs.WithMaxAge(7*24*time.Hour))
```

`blobstest.RunCached` runs `RunStore` and `RunLinker` on a `Cached` over any remote, and then the checks above: a lost local folder, a corrupt remote blob, the sweep and the max age. A `Dir`, `s3blobs` and `gcsblobs` all run it as the remote. `RunLinker` checks `Link` alone, and `Dir` runs it too.

## Blobs and wasmhost's caches

wasmhost's `Cache` (`MemCache`, `DirCache`, the browser cache) looks similar, and the two are kept apart. A cache may lose an entry and rebuild it, but a blob store may not, since a record pointing at a missing blob has lost data. Their keys differ too: a cache key is any string the caller picks, and a blob's name is computed from its contents. The two share no lower layer.

## Aliases

A source can have its own name for a file's contents, such as a git blob id. `Aliases` is an optional interface on a store that remembers which blob such a name points at, in a namespace like `git`. The GitHub driver uses it so a second read of a repo downloads only files whose git blob id it hasn't seen (#172). `Dir` keeps each alias as a file under `<root>/aliases/<ns>/<id>`, beside the blobs, so `Sweep` never walks them. An alias is a hint. Its blob can be swept, so a caller checks `Has` before trusting it.

## The package

{{ apiref "storage/blobs" }}
