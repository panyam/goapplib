---
title: "Go API"
description: "Every exported type, function and method of goapplib, page and wasmhost, read from the source on main when the site is built."
---

This page is generated. When the site builds, it reads goapplib's Go source on `main` with `go/doc` and writes out each package's exported API: every type, function, method, constant and variable, with its declaration and doc comment. A test fails the build if an exported name is missing, so the page can't drift from the code the way the hand-kept list it replaces did. (We kept that list by hand, and it still showed `Loader[AC]` two releases after it was removed.) Where a guide page teaches a symbol, its entry links there.

A declaration marked `build: js && wasm` exists only in a build for the browser (`GOOS=js GOARCH=wasm`), and one marked `build: !(js && wasm)` only in a native one. [pkg.go.dev](https://pkg.go.dev/github.com/panyam/goapplib) has pretty much the same API for each tagged release, though it lags `main` until the next tag.

The packages: [goapplib](#goapplib), [page](#page), [wasmhost](#wasmhost).

{{ apiref "" }}

{{ apiref "page" }}

{{ apiref "wasmhost" }}
