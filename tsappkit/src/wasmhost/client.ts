// The page's side of a wasm host worker (goapplib issue 32). startWorker loads the app's Go wasm in
// a Web Worker; workerFetch is a fetch to hand a Connect transport, so the app's generated clients
// talk to the worker exactly as they would to a server; mountFiles pushes files in before the
// requests that read them.
import type { Files, HostChunk, HostReply, HostRequest, HostStatus } from "./protocol";

export type { Files } from "./protocol";

/** Where the worker's pieces are served, and the global name the Go side exports under. */
export interface StartWorkerOptions {
  /** The built worker script, @panyam/tsappkit/wasmhost/worker.js, as your bundler serves it. */
  worker: string | URL;
  /** The Go wasm build. */
  wasm: string | URL;
  /** Go's wasm_exec.js, from `$(go env GOROOT)/lib/wasm`, matching the Go that built the wasm. */
  exec: string | URL;
  /** The ns the Go program passed to wasmhost.Serve. */
  ns: string;
}

type Unsent = HostRequest extends infer R ? (R extends HostRequest ? Omit<R, "id"> : never) : never;

class Channel {
  private next = 1;
  private pending = new Map<
    number,
    { resolve: (r: HostReply) => void; reject: (e: Error) => void; onChunk?: (c: HostChunk) => void }
  >();
  private dead: Error | null = null;

  constructor(private worker: Worker) {
    worker.addEventListener("message", (ev: MessageEvent<HostReply | HostStatus | HostChunk>) => {
      const m = ev.data;
      if ("exited" in m) {
        this.fail(new Error(`wasm host: ${m.exited}`));
      } else if ("chunk" in m) {
        this.pending.get(m.id)?.onChunk?.(m);
      } else if ("id" in m) {
        this.pending.get(m.id)?.resolve(m);
        this.pending.delete(m.id);
      }
    });
    worker.addEventListener("error", (ev: ErrorEvent) => this.fail(new Error(`wasm host worker: ${ev.message}`)));
  }

  /** Sends req and returns its id with the final reply; chunks before it go to onChunk. */
  start(req: Unsent, transfer: Transferable[], onChunk?: (c: HostChunk) => void): { id: number; done: Promise<HostReply> } {
    const id = this.next++;
    if (this.dead) return { id, done: Promise.reject(this.dead) };
    const done = new Promise<HostReply>((resolve, reject) => {
      this.pending.set(id, { resolve, reject, onChunk });
      this.worker.postMessage({ ...req, id }, transfer);
    });
    return { id, done };
  }

  call(req: Unsent, transfer: Transferable[]): Promise<HostReply> {
    return this.start(req, transfer).done;
  }

  fail(err: Error) {
    this.dead = err;
    for (const p of this.pending.values()) p.reject(err);
    this.pending.clear();
  }
}

const channels = new WeakMap<Worker, Channel>();

/**
 * Terminates worker and rejects every request still waiting on it with `err`, which terminate
 * alone wouldn't do: a terminated worker never replies, so its pending requests would wait forever.
 * For lane.ts; not exported from the package.
 */
export function endWorker(worker: Worker, err: Error): void {
  channels.get(worker)?.fail(err);
  worker.terminate();
}

function channel(worker: Worker): Channel {
  let c = channels.get(worker);
  if (!c) {
    c = new Channel(worker);
    channels.set(worker, c);
  }
  return c;
}

/**
 * Starts the worker and resolves with it once the Go program has installed its exports, or rejects
 * with why it couldn't load (and terminates it, since it can't answer anything). Relative URLs
 * resolve against the page, not the worker script.
 */
export function startWorker(opts: StartWorkerOptions): Promise<Worker> {
  const abs = (u: string | URL) => new URL(u, location.href).href;
  const url = new URL(abs(opts.worker));
  url.searchParams.set("wasm", abs(opts.wasm));
  url.searchParams.set("exec", abs(opts.exec));
  url.searchParams.set("ns", opts.ns);
  const worker = new Worker(url);
  channel(worker);
  return new Promise((resolve, reject) => {
    const onMessage = (ev: MessageEvent<HostStatus | HostReply>) => {
      const m = ev.data;
      if (!("ready" in m)) return;
      worker.removeEventListener("message", onMessage);
      if (m.ready) {
        resolve(worker);
        return;
      }
      worker.terminate();
      reject(new Error(`wasm host failed to load: ${m.error}`));
    };
    worker.addEventListener("message", onMessage);
    worker.addEventListener(
      "error",
      (ev: ErrorEvent) => {
        worker.terminate();
        reject(new Error(`wasm host worker: ${ev.message}`));
      },
      { once: true },
    );
  });
}

// Statuses whose Response must have no body.
const NULL_BODY = new Set([101, 103, 204, 205, 304]);

/**
 * A fetch that answers in the worker, for `createConnectTransport({ baseUrl, fetch: workerFetch(worker) })`.
 * Only the path and query reach the worker, so baseUrl can be anything absolute, such as
 * location.origin. It rejects if the worker reports an error (no handler, a failed build, a handler
 * panic) and resolves with any HTTP status the handler chose.
 *
 * A handler that flushes (http.Flusher) streams: the Response resolves at its first flush, and its
 * body delivers each flush as it happens, even from a Go loop that never yields, then the rest
 * when the handler returns. Aborting the request (init.signal) rejects it, or errors a body that's
 * already streaming, and cancels the handler's context; a handler sees that the next time it checks
 * ctx, which a loop that never yields to JS doesn't get to do (a lane ends such a worker instead).
 * Once the body is streaming, Chrome's text() and json() report the abort as "TypeError: Failed to
 * fetch" rather than an AbortError, so tell an abort from a failure by signal.aborted.
 */
export function workerFetch(worker: Worker): (input: RequestInfo | URL, init?: RequestInit) => Promise<Response> {
  const c = channel(worker);
  return async (input, init) => {
    const req = new Request(input, init);
    const signal = req.signal;
    if (signal.aborted) throw abortReason(signal);
    const buf = new Uint8Array(await req.arrayBuffer());
    const body = buf.byteLength ? buf : null;
    const headers: Record<string, string> = {};
    req.headers.forEach((v, k) => (headers[k] = v));
    const u = new URL(req.url);

    return new Promise<Response>((resolve, reject) => {
      let stream: ReadableStreamDefaultController<Uint8Array> | undefined;
      let settled = false;
      const fail = (err: unknown) => {
        if (settled) return;
        settled = true;
        if (stream) stream.error(err);
        else reject(err);
      };
      const { id, done } = c.start(
        { kind: "http", method: req.method, url: u.pathname + u.search, headers, body },
        body ? [body.buffer] : [],
        (chunk) => {
          if (settled) return;
          if (!stream) {
            const status = chunk.status ?? 200;
            const rs = new ReadableStream<Uint8Array>({ start: (ctl) => void (stream = ctl) });
            resolve(new Response(NULL_BODY.has(status) ? null : rs, { status, headers: chunk.headers }));
          }
          if (chunk.body.byteLength) stream!.enqueue(chunk.body);
        },
      );
      const onAbort = () => {
        void c.call({ kind: "cancel", target: id }, []).catch(() => {});
        fail(abortReason(signal));
      };
      signal.addEventListener("abort", onAbort, { once: true });
      done.then(
        (reply) => {
          signal.removeEventListener("abort", onAbort);
          if (!reply.ok) return fail(new Error(reply.error));
          if (settled) return;
          settled = true;
          if (stream) {
            if (reply.body?.byteLength) stream.enqueue(reply.body);
            stream.close();
            return;
          }
          const status = reply.status ?? 200;
          const resBody = NULL_BODY.has(status) ? null : ((reply.body ?? null) as BodyInit | null);
          resolve(new Response(resBody, { status, headers: reply.headers }));
        },
        (err) => {
          signal.removeEventListener("abort", onAbort);
          fail(err);
        },
      );
    });
  };
}

function abortReason(signal: AbortSignal): unknown {
  return signal.reason ?? new DOMException("This request was aborted.", "AbortError");
}

/**
 * A fetch like workerFetch's, except that each call starts a fresh worker from `opts`, sends it that
 * one request, and terminates it once the response is in, whether the request succeeded or not.
 * Ending the worker is the only way to give its wasm memory back to the browser, since wasm memory
 * never shrinks, so route a request with a big transient peak (an ingest that parses a large input
 * on the way to a small result) through this, and have it leave its result in the shared Cache
 * (Go's wasmhost.BrowserCache) for the long-lived worker to load. Every worker for the same `ns`
 * sees the same cache.
 *
 * Each call pays for a worker start and a wasm instantiate, a few hundred ms, so it's for jobs
 * that take seconds, not for ordinary requests. The worker ends when the response's body has been
 * read to the end, fails, or is cancelled, so a streamed body isn't cut short; read or cancel the
 * body, or the worker stays up.
 */
export function oneShotFetch(opts: StartWorkerOptions): (input: RequestInfo | URL, init?: RequestInit) => Promise<Response> {
  return async (input, init) => {
    const worker = await startWorker(opts);
    let res: Response;
    try {
      res = await workerFetch(worker)(input, init);
    } catch (err) {
      worker.terminate();
      throw err;
    }
    return untilBodyEnds(res, () => worker.terminate());
  };
}

/**
 * res with `end` called once its body has been read to the end, has failed, or has been
 * cancelled (or at once, for a response without a body). For oneShotFetch and lanes, which must
 * not let go of a worker while its streamed body is still arriving. Not exported from the package.
 */
export function untilBodyEnds(res: Response, end: () => void): Response {
  if (!res.body) {
    end();
    return res;
  }
  const reader = res.body.getReader();
  const body = new ReadableStream<Uint8Array>({
    async pull(ctl) {
      try {
        const { done, value } = await reader.read();
        if (done) {
          ctl.close();
          end();
        } else {
          ctl.enqueue(value);
        }
      } catch (err) {
        end();
        ctl.error(err);
      }
    },
    cancel(reason) {
      end();
      return reader.cancel(reason);
    },
  });
  return new Response(body, { status: res.status, statusText: res.statusText, headers: res.headers });
}

/**
 * Replaces the mount `name` with `files`. The buffers are transferred to the worker, not copied, so
 * each Uint8Array is empty here afterwards; copy first anything the page still needs. Resolves once
 * the worker has mounted them (and, for a rebuild-mode host, rebuilt its handler).
 */
export function mountFiles(worker: Worker, name: string, files: Files): Promise<void> {
  return sendFiles(worker, "mount", name, files);
}

/**
 * Adds `files` to the mount `name`, keeping the files it already holds; a file at a path the mount
 * already has is replaced, and a missing mount is created. This suits a page that brings files in a
 * piece at a time, since it only sends what's new. The worker checks the whole batch first, so a
 * rejection (a path that is already a directory, say) leaves the mount as it was. Buffers are
 * transferred as with mountFiles.
 */
export function addFiles(worker: Worker, name: string, files: Files): Promise<void> {
  return sendFiles(worker, "add", name, files);
}

async function sendFiles(worker: Worker, kind: "mount" | "add", name: string, files: Files): Promise<void> {
  const transfer = new Set<ArrayBuffer>();
  for (const b of Object.values(files)) {
    // A tag check rather than instanceof, which fails for a buffer from another realm (an iframe),
    // and a SharedArrayBuffer can't be transferred.
    if (Object.prototype.toString.call(b.buffer) === "[object ArrayBuffer]") transfer.add(b.buffer as ArrayBuffer);
  }
  const reply = await channel(worker).call({ kind, name, files }, [...transfer]);
  if (!reply.ok) throw new Error(reply.error);
}

/**
 * The bytes of linear memory the worker's wasm holds. Wasm memory grows and never shrinks, so this
 * is the peak the engine has needed so far, which is what an app weighs before giving a browser a
 * bigger job.
 */
export async function workerMemory(worker: Worker): Promise<number> {
  const reply = await channel(worker).call({ kind: "stats" }, []);
  if (!reply.ok) throw new Error(reply.error);
  return reply.memoryBytes ?? 0;
}

/** Removes the mount `name`. Removing a name that isn't mounted succeeds. */
export async function unmountFiles(worker: Worker, name: string): Promise<void> {
  const reply = await channel(worker).call({ kind: "unmount", name }, []);
  if (!reply.ok) throw new Error(reply.error);
}
