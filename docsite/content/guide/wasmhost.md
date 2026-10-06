---
title: "Go handlers in a Web Worker"
description: "wasmhost runs an app's own HTTP or Connect handlers as wasm in a Web Worker, so the page's clients get their answers without a round trip."
---

Some apps have a service that's mostly a function of files the user already has in the browser: a design they dropped in, a project folder, a document. Sending those files to a server so it can answer questions about them costs a round trip on every question, and sometimes the files aren't allowed to leave the machine at all. wasmhost takes the service's Go handlers, the ones the server already runs, and runs them as wasm inside a Web Worker. The page's generated clients talk to the worker through a fetch that never touches the network, and they can't tell it from the server.

This is how agni's in-browser engine works (agni#863), and it's what the demo below is running.

{{ demo "wasmhost-files" 400 }}

The demo is `exercise/wasmhost`'s `FilesService`, a two-method Connect service. `ReadFile` reads a file the page mounted, and `Spin` keeps the worker busy for as long as you ask. On load it runs each step once and checks it; the buttons are there to poke at it afterwards.

## When it fits

It fits when the work is a pure function of inputs the page can hand over, the user would otherwise wait on a server for it, and the Go code already exists. It doesn't fit when the answer needs the server's data, or when the download matters more than the round trips. A Go wasm build is big. The demo's is 15 MB, about 3.5 MB gzipped, nearly all of it protobuf, connect-go and `net/http`, which is what any Go Connect service compiles to. A service that speaks plain JSON comes out at about half that (the state demo on the [next page]({{.Site.PathPrefix}}/guide/wasmhost-state/) is 1.9 MB gzipped). Pages download it once and the browser caches it after that, but it's still a few seconds on a slow phone.

## The pieces

There are two programs, one Go and one TypeScript, plus three files the page has to serve.

The Go side is a `main` built with `GOOS=js GOARCH=wasm`. It hands its handler to wasmhost and never returns. The namespace (`"files"` here) is the global name its exports go under, and the page has to use the same one:

```go
//go:build js && wasm

package main

import (
	"github.com/panyam/goapplib/exercise/wasmhost/service"
	"github.com/panyam/goapplib/wasmhost"
)

func main() {
	h := wasmhost.New("files")
	h.Serve(service.Handler(h.Root()))
}
```

A handler that doesn't read the mounted files can skip the `Host` and call `wasmhost.Serve("files", handler)`.

The page serves three files next to its own script:

- **The wasm**, from `GOOS=js GOARCH=wasm go build -o app.wasm ./wasm`. For a release build, `-trimpath -ldflags="-s -w"` drops the build paths and the symbol table, though only about half a MB of the demo's 15.
- **`wasm_exec.js`**, Go's loader, from `$(go env GOROOT)/lib/wasm/`. It has to come from the same Go toolchain that built the wasm, since the two talk through a private ABI that changes between Go versions.
- **`worker.js`**, from the npm package as `@panyam/tsappkit/wasmhost/worker.js`, prebuilt as a classic script (it loads `wasm_exec.js` with `importScripts`, which a module worker can't do). Copy it beside your bundle; agni's build does `copyFileSync(require.resolve("@panyam/tsappkit/wasmhost/worker.js"), ...)`. The worker and the client code have to come from the same tsappkit version, so ship them together. We learned that one the hard way, since the v0.2.0 worker treated a request kind it didn't know as an unmount, so a newer page's `addFiles` sent to a cached old worker quietly unmounted the files.

Put a version on each URL (a content hash works well) if anything caches them. This site does, because GitHub Pages caches for ten minutes, and a stale `wasm_exec.js` next to a new wasm fails with errors that point nowhere useful.

## Talking to it

The page side is `@panyam/tsappkit/wasmhost`. `startWorker` starts the worker and resolves once the Go program is up:

```ts
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { mountFiles, startWorker, workerFetch } from "@panyam/tsappkit/wasmhost";

const worker = await startWorker({ worker: "worker.js", wasm: "app.wasm", exec: "wasm_exec.js", ns: "files" });
await mountFiles(worker, "docs", { "hello.txt": new TextEncoder().encode("hello") });

const client = createClient(FilesService, createConnectTransport({ baseUrl: location.origin, fetch: workerFetch(worker) }));
const res = await client.readFile({ path: "docs/hello.txt" });
```

`workerFetch(worker)` is a fetch, so anything that takes one works, and a plain HTTP handler is just as happy with `fetch(location.origin + "/path")` calls through it. Only the path and query reach the worker, so `baseUrl` can be any absolute URL.

**Files go in before the request asks for them.** `mountFiles(worker, name, files)` replaces the mount `name`, and `addFiles` adds to it, keeping what's there. The handler sees every mount as a top-level directory of one `fs.FS`, `Host.Root()`, so mount `"docs"` holding `"hello.txt"` reads as `"docs/hello.txt"`. The rule exists because `fs.FS` is synchronous and every way a browser has of getting bytes is asynchronous, so a handler can't go and fetch a file halfway through a request. The buffers are transferred to the worker rather than copied, so each `Uint8Array` is empty on the page afterwards; copy anything the page still needs first.

For files the user drops on the page, `filesFromDrop(event.dataTransfer)` reads dropped files and folders into the shape `mountFiles` takes, and `filesFromFileList` does the same for an `<input type="file">`. Try dropping a folder on the demo above.

`workerMemory(worker)` reports how much wasm memory the worker holds. Wasm memory grows and never shrinks, so that's the peak so far, which is worth checking before handing a phone a bigger job.

## Fixed handlers and rebuilt ones

`Host.Serve(handler)` (or `Host.Handle` and `Host.Export`, for a custom main) gives the host one handler for good, and that handler reads the live `Root`, so it sees each mount change on its next read. That suits a handler that opens files per request.

Some handlers read everything when they're made: a parser that loads a whole project, a template set. For those, `wasmhost.ServeRebuild(ns, build)` calls `build(root)` once at the start and again after every mount change, and serves whatever the latest build returned. A build that fails makes requests fail with its error until a later one succeeds.

## Testing it natively

`Host` doesn't need a browser. `wasmhost.New`, `Mount`, `Handle` and `Do` all run in a normal `go test`, so a service's tests can mount files and send requests in-process:

```go
h := wasmhost.New("files")
h.Handle(service.Handler(h.Root()))
_ = h.Mount("docs", map[string][]byte{"hello.txt": []byte("hello")})
res, err := h.Do(ctx, wasmhost.Request{Method: "POST", URL: "/files.v1.FilesService/ReadFile", Header: hdr, Body: body})
```

`Do` buffers the response. `DoStream` hands each `Flush` to a callback as it happens, which is how the browser side streams (see the [next page]({{.Site.PathPrefix}}/guide/wasmhost-state/)). `exercise/wasmhost/service/service_test.go` is a full example, and goapplib's own `make wasm-test` runs the wasmhost tests as wasm under Node.

## Gotchas

- **Exports never block.** Each function the Go side exports copies its arguments and does its work on a goroutine, returning a Promise. Work done inside a `js.FuncOf` callback hangs the moment it waits on JS (a fetch, a timer, the cache), and it hangs rather than failing, so it's a pretty confusing bug to chase. If you write your own exports, do the same.
- **Under Node, await each Promise as soon as you create it.** Node ends the process on a rejection that has no handler yet, so a wasm test that builds several rejected Promises and awaits them later crashes the test binary.
- **A Go loop that never yields keeps the worker to itself.** The page stays responsive (that's the point of the worker), but a second request to the same worker waits until the first one returns. The [next page]({{.Site.PathPrefix}}/guide/wasmhost-state/) covers lanes, which give long jobs their own worker.
