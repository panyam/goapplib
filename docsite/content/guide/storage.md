---
title: "Records, files and sign-ins"
description: "Keeping an app's records in its own store, the files they point at in mounts (a folder or a GitHub repo), and the sign-ins that reach a private repo in connections, through a small notes app."
---

Picture a notes app. Each note has a title, an author and a date, and the app keeps those in its own store, the way it keeps anything else. The note's text is a different matter. One team wants its notes in a folder on the server. Another wants them in a private GitHub repo, where each save is a commit they can review. So the app keeps the text in a mount, and the record says which mount and which path.

goapplib's storage module splits things three ways:

- Records live in the app's own store. The library never sees them.
- Files live in mounts. A mount is a named place: a folder, a GitHub repo, and later an S3 or GCS bucket. A file in one is `mount://<name>/<path>`.
- Sign-ins live in connections. A connection is a sign-in the server holds for a service, such as a GitHub token. Its record names where the token is, such as the environment variable `env:NOTES_GITHUB_TOKEN`, and never holds the token.

The storage module is its own Go module, so an app that wants it adds a second `go get`:

```sh
go get github.com/panyam/goapplib/storage
```

## The app

Here is the whole app, `docsite/examples/notes` in goapplib's repo. Its test and the storage exercise compile and run it, so this is the code that runs:

```go
{{ includeFileText "examples/notes/notes.go" }}
```

`Open` builds everything from folders under `StoreDir`, so a restart over the same `Config` sees the same notes, mounts and connections. It makes three stores and a registry:

- `mountstore` keeps the runtime mounts' records, one YAML file each.
- `connstore` keeps the connection records, one YAML file each.
- `blobs.NewDir` keeps GitHub snapshots' files, each stored once under its SHA-256.

The registry joins them with two drivers, `folder` and `github`. `WithFixed` adds the `local` folder mount, which comes from the server's configuration and never from a request. `WithCredentials` hands the registry the connections service's `Credentials`, so the GitHub driver sends the mount's connection token with each request.

## Writing and reading a note

`createNote` writes the text into the mount first, then saves the record. If the write fails, there's no record pointing at nothing. In the `local` folder mount, the write is a file. In a GitHub mount, the write is a commit on the branch.

`readNote` reads the record, then reads the text through `FS`, which serves every mount as one `fs.FS`, with each mount as a folder at the top. So `local/notes/1a2b3c4d.md` and `shared/notes/5e6f7a8b.md` read the same way, whichever kind of mount each one is.

## Pinned or live

A GitHub mount has one of two modes, and the app picks one when it makes the mount:

- A snapshot pins the commit the ref pointed at when the mount was made. The driver downloads that commit's files into the blob store once. From then on, the mount serves those bytes, after a restart too, without asking GitHub again.
- A live mount reads the branch's latest commit on every open, and each write is one commit on it.

Pin a mount when a record has to keep meaning the same bytes. agni saves circuit designs that point into a repo, and a design made last month should still open the files it was made from, even after the repo moves on. Use live when the app edits the files, as this notes app does.

A live write passes the version of the file it read. If someone changed the file since, GitHub refuses the commit, and the registry returns `mounts.ErrConflict`. So two people saving the same note can't silently overwrite each other.

## Connections and secrets

The app saves a connection through `PutConnection`, and the store validates the record first. A token field has to hold a reference such as `env:NOTES_GITHUB_TOKEN` or `file:/run/secrets/github`. A pasted token fails validation, and the app answers 400. So the token never reaches the store, and the store's folder can be backed up as it is.

Three kinds of connection exist. `github-token` is a fixed token, such as a personal access token. `github-app` is a GitHub App installation, which mints an hour-long token from the App's private key. `github-user` is a person's own GitHub account, connected through OAuth, whose token oneauth keeps sealed and refreshes. [Connections](../../reference/connections/) covers each one, and where a secret can and can't end up.

## Errors

The storage packages return errors that wrap sentinels such as `mounts.ErrNotFound` and `mounts.ErrConflict`. `fail` maps them onto HTTP statuses with `errors.Is`, the way any app maps them onto its own transport. A private repo the token can't read answers `ErrNotFound`, because GitHub answers 404 rather than reveal the repo exists.

## Deleting a mount

`WithInUse` gives the registry the app's answer to "do any records still point into this mount?" The notes app checks its notes. So deleting a mount that holds a note's text answers 409, and so does deleting the `local` mount, which comes from the configuration.

## Adding a driver

A driver serves one kind of mount. It implements `mounts.Driver`, which resolves a ref to a version and opens a mount as an `fs.FS`. Three optional interfaces sit on top. `Writer` adds `Stat`, `Put` and `Delete`. `Localizer` adds `HostPath`, for code that needs a real file on disk. `Preparer` adds a step when a mount is made, such as downloading a snapshot. The app passes the driver to `NewRegistry` beside the others.

A driver gets `mounts.Credentials` for each call, or nil for a public source, and knows nothing about connections. GitHub takes a bearer token, and S3 takes an AWS key pair or the default chain. One that needs a big SDK goes in its own module under `storage/drivers/`, as the S3 and GCS drivers do, so apps that don't use it never download it. [Mounts](../../reference/mounts/) has the contracts, and how the GitHub driver meets them.

## Testing against a fake GitHub

`storage/mounts/github/githubtest` is a fake GitHub that serves a repo's commits, tarballs and contents from memory, and makes a real commit for each write. A private repo answers only an accepted token, and the fake records the token each request carried. The app's test uses it, and so does the storage exercise:

```sh
make exercise-storage
```

The exercise writes a note into the folder mount, restarts the app over the same folders, and reads the note back. Then it makes a token connection, writes a note into a private repo on the fake, and pins a snapshot before changing the repo. After a second restart, the snapshot still reads the old commit with no GitHub requests, and the live mount reads the new one. Last, it checks that every request carried the connection's token, and that the token appears in no file on disk:

```
exercise-storage: ok   wrote note eb431dc0 into the folder mount at notes/eb431dc0.md
exercise-storage: ok   restarted the app over the same folders
exercise-storage: ok   read note eb431dc0 back after the restart: "eggs, flour, butter"
exercise-storage: ok   made connection acme, whose record names env:NOTES_GITHUB_TOKEN
exercise-storage: ok   wrote note e1289fec to the private repo as commit 1b2349a
exercise-storage: ok   made snapshot mount pinned at commit 1b2349a, then changed the repo
exercise-storage: ok   after a restart, pinned still reads commit 1b2349a with no GitHub requests, and shared reads the latest
exercise-storage: ok   the app holds mounts local (folder live), pinned (github snapshot), shared (github live) and connection acme (github-token)
exercise-storage: ok   every one of 9 requests to GitHub carried the connection's token
exercise-storage: ok   no secret in any of the 8 files on disk
exercise-storage: ok
```

With `GITHUB_TOKEN` and `STORAGE_EXERCISE_REPO=owner/repo` set, it also snapshots a private repo of yours on github.com through a token connection, and lists its top folder.

## Further reading

- [The storage module](../../reference/storage-module/) covers why storage is its own module, and how it's versioned.
- [Mounts](../../reference/mounts/), [Connections](../../reference/connections/) and [Blobs](../../reference/blobs/) cover the design of each package, with its API.
