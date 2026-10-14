---
title: "Overview"
description: "What goapplib is, the three packages it ships, and how this site's demos run."
---

goapplib is for web apps whose pages are rendered on the server, in Go, and that need client code only in places. It ships as three packages that share one version (the [Reference]({{.Site.PathPrefix}}/reference/) lists them). We built it for our own apps, and thambura, agni and lilbattle all run on it.

## goapplib

It's the Go module, `github.com/panyam/goapplib`. A page is a view, a Go type that loads its own data and renders through [templar](https://github.com/panyam/templar) templates. Mixins add the common parts (a base page, pagination, auth, filtering, htmx awareness), and pages register on a mux individually or in groups. The same templates render whole pages and htmx fragments.

## tsappkit

`@panyam/tsappkit` is the browser half. `BasePage` gives a page a component lifecycle, and `IslandPage` mounts the islands a Go page declares in its page spec, each when its load strategy says to: at once, when the browser is idle, when its slot scrolls into view, or when a media query matches. `@panyam/tsappkit-solid` adds `SolidIsland`, for islands written in Solid. Islands that show the same state share one presenter, which drives each through a `Port`. The [guide]({{.Site.PathPrefix}}/guide/islands/) shows both with live demos.

## wasmhost

`wasmhost` (Go) and `@panyam/tsappkit/wasmhost` (TS) run an app's own HTTP or Connect handlers as wasm inside a Web Worker. The page pushes the files the handlers read into the worker, and its generated Connect clients talk to the worker through `workerFetch`, so they don't know whether a server or the worker answered. A cache keeps state across a reload, lanes give long jobs their own worker, and `workerStats` reports a worker's live Go heap beside its wasm memory, so a lane replaces a worker only when a fresh one would come back smaller. The [guide]({{.Site.PathPrefix}}/guide/wasmhost/) walks through it with live demos.

## Demos on this site

Pages here embed live demos that run in your browser. Each runs in its own frame, built from the source in [`docsite/demos/`](https://github.com/panyam/goapplib/tree/main/docsite/demos) against the goapplib on `main`, and `make exercise-docsite` drives every one in headless Chromium before the site deploys. This one only checks that the frame works:

{{ demo "hello" 120 }}
