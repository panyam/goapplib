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

GCS and S3 stores come later, in their own modules under `storage/drivers/`, as objects named by hash. They implement `Store` without `Linker`, so a snapshot over them serves an `fs.FS` and has no host path.

## Blobs and wasmhost's caches

wasmhost's `Cache` (`MemCache`, `DirCache`, the browser cache) looks similar, and the two are kept apart. A cache may lose an entry and rebuild it, but a blob store may not, since a record pointing at a missing blob has lost data. Their keys differ too: a cache key is any string the caller picks, and a blob's name is computed from its contents. The two share no lower layer.

## The package

{{ apiref "storage/blobs" }}
