---
title: "Overview"
description: "What goapplib is, the packages it ships, and how this site's demos run."
---

goapplib is for web apps whose pages are rendered on the server, in Go, and that need client code only in places. It ships as a Go module, a storage module beside it, and three npm packages, all sharing one version (the [Reference]({{.Site.PathPrefix}}/reference/) lists them). We built it for our own apps, and thambura, agni and lilbattle all run on it.

## goapplib

It's the Go module, `github.com/panyam/goapplib`. A page is a view, a Go type that loads its own data and renders through [templar](https://github.com/panyam/templar) templates. Mixins add the common parts (a base page, pagination, auth, filtering, htmx awareness), and pages register on a mux individually or in groups. The same templates render whole pages and htmx fragments.

## tsappkit

`@panyam/tsappkit` is the browser half. `BasePage` gives a page a component lifecycle, and `IslandPage` mounts the islands a Go page declares in its page spec, each when its load strategy says to: at once, when the browser is idle, when its slot scrolls into view, or when a media query matches. `@panyam/tsappkit-solid` adds `SolidIsland`, for islands written in Solid. Islands that show the same state share one presenter, which drives each through a `Port`. The [guide]({{.Site.PathPrefix}}/guide/islands/) shows both with live demos.

## tsappkit-dock

`@panyam/tsappkit-dock` builds an IDE-like page on [dockview-core](https://dockview.dev). You list the panels and a default arrangement. Each panel then sits in the grid, floats over it, or slides out of a drawer on any edge, and the reader moves it from its ⋯ menu. A bottom drawer works as a phone sheet. The layout is saved, and a saved layout picks up panels added since. The dock moves DOM elements, so it needs no UI framework. The [guide]({{.Site.PathPrefix}}/guide/dock/) has a live dock whose panels are this site's pages, and [Dock experiments]({{.Site.PathPrefix}}/guide/dock-lab/) tries pinned drawers and popout windows.

## wasmhost

`wasmhost` (Go) and `@panyam/tsappkit/wasmhost` (TS) run an app's own HTTP or Connect handlers as wasm inside a Web Worker. The page pushes the files the handlers read into the worker, and its generated Connect clients talk to the worker through `workerFetch`, so they don't know whether a server or the worker answered. A cache keeps state across a reload, lanes give long jobs their own worker, and `workerStats` reports a worker's live Go heap beside its wasm memory, so a lane replaces a worker only when a fresh one would come back smaller. The [guide]({{.Site.PathPrefix}}/guide/wasmhost/) walks through it with live demos.

## storage

`github.com/panyam/goapplib/storage` is a Go module of its own, so an app that doesn't use it gets nothing new in its `go.sum`. An app keeps its records, such as a note's title and author, in its own store. The files those records point at live in mounts. A mount is a named place, a folder or a GitHub repo for now, and a file in one is `mount://<name>/<path>`. A GitHub mount is pinned to a commit or follows a branch, and it reads the repo at any other commit by downloading only the files that changed. The app reaches a private repo through a connection. A connection is a sign-in the server holds, and its record names where the token is, never the token itself. A blob store keeps each downloaded file once, by its hash. GORM and Datastore backends store the mount and connection records. The [guide]({{.Site.PathPrefix}}/guide/storage/) builds a small notes app on it, and the [Reference]({{.Site.PathPrefix}}/reference/storage-module/) has the design.

## Demos on this site

Pages here embed live demos that run in your browser. Each runs in its own frame, built from the source in [`docsite/demos/`](https://github.com/panyam/goapplib/tree/main/docsite/demos) against the goapplib on `main`, and `make exercise-docsite` drives every one in headless Chromium before the site deploys. The [Demos]({{.Site.PathPrefix}}/demos/) page lists every one. This one only checks that the frame works:

{{ demo "hello" 120 }}
