---
title: "Guide"
description: "How to build with goapplib, one topic per page."
---

Start with [Getting started](getting-started/), which builds a one-page app whose code CI compiles and renders, then [Concepts](concepts/), which covers how a request flows through goapplib. [Views and mixins](views/) and [Routing](routing/) build a small app with a list, detail pages, a login check and a page group, all compiled and tested.

[Island pages](islands/) covers a server-rendered page whose islands mount from a spec Go writes, each when its load strategy says, with live demos.

Two pages cover running an app's Go handlers in the browser, each with a live demo.

1. [Go handlers in a Web Worker](wasmhost/) covers wasmhost's pieces, mounting files, and the Connect transport.
2. [State, heavy jobs and long requests](wasmhost-state/) covers the cache, throwaway workers, lanes, streaming and aborts.

The rest of the guide is moving here from the repo's [USAGE_GUIDE.md](https://github.com/panyam/goapplib/blob/main/USAGE_GUIDE.md). Until each topic has its page, that file is where to read it:

- Templates, layout and htmx: [USAGE_GUIDE.md, Templates](https://github.com/panyam/goapplib/blob/main/USAGE_GUIDE.md#templates) ([#96](https://github.com/panyam/goapplib/issues/96)).
