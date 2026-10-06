---
title: "State, heavy jobs and long requests"
description: "Keeping a wasmhost service's state across a reload, running a job with a big memory peak, and giving long jobs their own worker."
---

The [first page]({{.Site.PathPrefix}}/guide/wasmhost/) covered a service that answers from the files it's given. Real ones tend to build something first (a parsed design, a laid-out board, an index), and that's where three problems turn up. Building it can take seconds, and a reload throws it away. Building it can need far more memory than the result, and wasm memory never shrinks, so the worker keeps the peak forever. And a request that runs for seconds holds the worker's one thread, so a quick question behind it waits.

wasmhost has one tool for each: a cache for the state, a throwaway worker for the peak, and lanes for the long jobs. The demo runs all three, using `exercise/workerstate`'s service, which builds a 4 MB result through a 64 MB scratch buffer:

{{ demo "wasmhost-state" 520 }}

Reload the demo (the button, or the page) and the first step changes. The second time, nothing ingests, because the serving worker's warm finds the result in the cache. "Clear the cache and reload" puts it back to a first visit.

## A cache for state the service can rebuild

`wasmhost.Cache` is a key-to-blob store with two methods, `Get` and `Put`. Keys name the inputs, not the result. A service works out the key from a request before it has anything, and the same inputs always find the same blob. `wasmhost.CacheKey(parts...)` hashes the parts into a key. Put a version string first, so a build that changes what it stores stops finding what an older build wrote.

In the browser it's `wasmhost.BrowserCache()`, which keeps blobs in the page origin's private file system (OPFS, under `wasmhost/<ns>/`), so they outlive a reload. Natively there's `DirCache(dir)` and `MemCache`, so the same service code runs in tests. The demo's service opens its state like this (trimmed; the real one also reports cache errors and copes with no cache):

```go
func (s *Service) open(w http.ResponseWriter, r *http.Request) {
	req, _ := readIngest(w, r)
	key := wasmhost.CacheKey([]byte("workerstate-v1"), []byte(strconv.Itoa(req.PeakMB)), []byte(strconv.Itoa(req.ResultMB)))
	if b, err := s.Cache.Get(r.Context(), key); err == nil {
		writeJSON(w, s.keep(b, true)) // restored
		return
	}
	st := s.build(req) // the expensive part
	_ = s.Cache.Put(r.Context(), key, s.current())
	writeJSON(w, st)
}
```

It's a cache, not a datastore. Anything in it has to be rebuildable from its inputs. A miss (`ErrMiss`), a browser without OPFS (`ErrNoCache`) and the user clearing site data all just mean the service builds it again, which is mostly a matter of waiting. Data a user would lose belongs on a server.

`Get` and `Put` wait on the browser, so call them from a handler (which runs on its own goroutine and is allowed to wait), never from inside a `js.FuncOf` callback.

## A throwaway worker for a big peak

A worker that has needed 64 MB once holds 64 MB until it ends, even if all it kept is 4 MB, because wasm linear memory only grows. The way to give memory back is to end the worker. `oneShotFetch(opts)` is a fetch that starts a fresh worker for each request and terminates it once the response is in:

```ts
import { oneShotFetch, startLane } from "@panyam/tsappkit/wasmhost";

const opts = { worker: "worker.js", wasm: "app.wasm", exec: "wasm_exec.js", ns: "state" };
// body is the JSON inputs, and serve is the long-lived worker's lane (see the next section).
// The ingest runs in a worker of its own, which Puts the result in the cache and ends.
await (await oneShotFetch(opts)(location.origin + "/open", { method: "POST", body })).json();
// The long-lived worker opens the same inputs and finds them in the cache.
await (await serve.fetch(location.origin + "/open", { method: "POST", body })).json();
```

Every worker for the same `ns` sees the same `BrowserCache`, and that shared cache makes the handoff work. The first step of the demo does exactly this, and it checks that the serving worker ends up holding well under the 64 MB peak.

Each call pays for a worker start and a wasm instantiate, a few hundred ms, so it's for jobs that take seconds, not for every request. The worker ends when the response body has been read to the end, failed or been cancelled, so read or cancel the body, or the worker stays up.

## Lanes for long jobs

A Go handler that loops for three seconds without yielding has the worker to itself for three seconds. The page doesn't freeze, since it's a worker, but every other request to that worker queues behind it. `startLane(opts, options)` gives a kind of request its own replaceable worker, and an app usually wants two: a "serve" lane for quick queries and a "jobs" lane for long jobs.

```ts
const serve = startLane(opts, {
  // A new serving worker loads the state from the cache, and never ingests.
  warm: (f) => f(location.origin + "/restore", { method: "POST", body }),
  maxMemoryBytes: 48 << 20,
});
const jobs = startLane(opts);

const client = createClient(Service, createConnectTransport({ baseUrl: location.origin, fetch: serve.fetch }));
```

Each lane's `fetch` goes to that lane's current worker. `warm` runs on every new worker before the lane hands it a request, which is where a service loads its state from the cache, so a replacement answers like the worker it replaced.

Aborting a request on a lane (`init.signal`) terminates the lane's worker and starts a fresh, warmed one. That sounds drastic, but it's the only way to stop a Go loop that never yields, because the worker can't read the "cancel" message until the loop lets go of the thread. Other requests still waiting on the old worker are rejected too, so keep a lane to one kind of request.

`maxMemoryBytes` covers the slower problem, a worker that creeps up through many requests that each allocate and drop some memory. The lane checks its worker's memory whenever it goes idle, and past the limit replaces it with a warmed one. It never interrupts a request. Size it above what the warmed state needs, or the lane restarts after every request. The demo's last step pushes the serving worker past its 48 MB limit on purpose and watches it come back.

## Streaming and cancelling

A handler that writes and calls `Flush` (`http.Flusher`) streams to the page as it runs. `workerFetch`'s Response resolves at the first flush, and its body delivers each flush as it happens. That works even from a loop that never yields, because sending a chunk is a synchronous call from Go into JS. The demo's "Run a 3 s job" moves its progress bar that way, from NDJSON lines the job flushes every 250 ms.

Aborting cancels the handler's `r.Context()`, which a handler that waits on JS or yields will see. A loop that never yields won't, because the cancel can't arrive until it does, which is why a lane ends the worker instead. Data can stream out of a busy worker, then, but nothing gets in until the handler yields or returns.

We tripped over one more thing while building the demo. Once a body is streaming, Chrome's `text()` and `json()` report an abort as `TypeError: Failed to fetch` rather than an `AbortError`, so tell an abort from a failure by `signal.aborted`, not by the error.
