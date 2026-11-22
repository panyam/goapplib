---
title: "Go API"
description: "Every exported type, function and method of goapplib, page, wasmhost, and the storage module's mounts, connections, blobs, drivers and backends, read from the source on main when the site is built."
---

This page is generated. When the site builds, it reads goapplib's Go source on `main` with `go/doc` and writes out each package's exported API: every type, function, method, constant and variable, with its declaration and doc comment. A test fails the build if an exported name is missing, so the page can't drift from the code the way the hand-kept list it replaces did. (We kept that list by hand, and it still showed `Loader[AC]` two releases after it was removed.) Where a guide page teaches a symbol, its entry links there.

A declaration marked `build: js && wasm` exists only in a build for the browser (`GOOS=js GOARCH=wasm`), and one marked `build: !(js && wasm)` only in a native one. [pkg.go.dev](https://pkg.go.dev/github.com/panyam/goapplib) has pretty much the same API for each tagged release, though it lags `main` until the next tag.

The packages: [goapplib](#goapplib), [page](#page), [wasmhost](#wasmhost), and from the [storage module](../storage-module/), [mounts](#mounts), [connections](#connections), [blobs](#blobs), the [folder](#folder), [github](#github), [bucket](#bucket), [s3](#s3) and [gcs](#gcs) drivers, and the [gormstore](#gormstore) and [gaestore](#gaestore) backends. The s3 and gcs drivers and the two backends are modules of their own.

{{ apiref "" }}

{{ apiref "page" }}

{{ apiref "wasmhost" }}

{{ apiref "storage/mounts" }}

{{ apiref "storage/connections" }}

{{ apiref "storage/blobs" }}

{{ apiref "storage/mounts/folder" }}

{{ apiref "storage/mounts/github" }}

{{ apiref "storage/mounts/bucket" }}

{{ apiref "storage/drivers/s3" }}

{{ apiref "storage/drivers/gcs" }}

{{ apiref "storage/backends/gorm" }}

{{ apiref "storage/backends/gae" }}
