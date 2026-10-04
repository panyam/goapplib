// The page's side of a wasm host worker (goapplib issue 32). startWorker loads the app's Go wasm in
// a Web Worker; workerFetch is a fetch to hand a Connect transport, so the app's generated clients
// talk to the worker exactly as they would to a server; mountFiles pushes files in before the
// requests that read them.
import type { Files, HostReply, HostRequest, HostStatus } from "./protocol";

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
  private pending = new Map<number, { resolve: (r: HostReply) => void; reject: (e: Error) => void }>();
  private dead: Error | null = null;

  constructor(private worker: Worker) {
    worker.addEventListener("message", (ev: MessageEvent<HostReply | HostStatus>) => {
      const m = ev.data;
      if ("exited" in m) {
        this.fail(new Error(`wasm host: ${m.exited}`));
      } else if ("id" in m) {
        this.pending.get(m.id)?.resolve(m);
        this.pending.delete(m.id);
      }
    });
    worker.addEventListener("error", (ev: ErrorEvent) => this.fail(new Error(`wasm host worker: ${ev.message}`)));
  }

  call(req: Unsent, transfer: Transferable[]): Promise<HostReply> {
    if (this.dead) return Promise.reject(this.dead);
    return new Promise((resolve, reject) => {
      const id = this.next++;
      this.pending.set(id, { resolve, reject });
      this.worker.postMessage({ ...req, id }, transfer);
    });
  }

  private fail(err: Error) {
    this.dead = err;
    for (const p of this.pending.values()) p.reject(err);
    this.pending.clear();
  }
}

const channels = new WeakMap<Worker, Channel>();

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
 * with why it couldn't load. Relative URLs resolve against the page, not the worker script.
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
      if (m.ready) resolve(worker);
      else reject(new Error(`wasm host failed to load: ${m.error}`));
    };
    worker.addEventListener("message", onMessage);
    worker.addEventListener("error", (ev: ErrorEvent) => reject(new Error(`wasm host worker: ${ev.message}`)), {
      once: true,
    });
  });
}

// Statuses whose Response must have no body.
const NULL_BODY = new Set([101, 103, 204, 205, 304]);

/**
 * A fetch that answers in the worker, for `createConnectTransport({ baseUrl, fetch: workerFetch(worker) })`.
 * Only the path and query reach the worker, so baseUrl can be anything absolute, such as
 * location.origin. It rejects if the worker reports an error (no handler, a failed build, a handler
 * panic) and resolves with any HTTP status the handler chose.
 */
export function workerFetch(worker: Worker): (input: RequestInfo | URL, init?: RequestInit) => Promise<Response> {
  const c = channel(worker);
  return async (input, init) => {
    const req = new Request(input, init);
    const buf = new Uint8Array(await req.arrayBuffer());
    const body = buf.byteLength ? buf : null;
    const headers: Record<string, string> = {};
    req.headers.forEach((v, k) => (headers[k] = v));
    const u = new URL(req.url);
    const reply = await c.call(
      { kind: "http", method: req.method, url: u.pathname + u.search, headers, body },
      body ? [body.buffer] : [],
    );
    if (!reply.ok) throw new Error(reply.error);
    const status = reply.status ?? 200;
    const resBody = NULL_BODY.has(status) ? null : ((reply.body ?? null) as BodyInit | null);
    return new Response(resBody, { status, headers: reply.headers });
  };
}

/**
 * Replaces the mount `name` with `files`. The buffers are transferred to the worker, not copied, so
 * each Uint8Array is empty here afterwards; copy first anything the page still needs. Resolves once
 * the worker has mounted them (and, for a rebuild-mode host, rebuilt its handler).
 */
export async function mountFiles(worker: Worker, name: string, files: Files): Promise<void> {
  const transfer = new Set<ArrayBuffer>();
  for (const b of Object.values(files)) {
    // A tag check rather than instanceof, which fails for a buffer from another realm (an iframe),
    // and a SharedArrayBuffer can't be transferred.
    if (Object.prototype.toString.call(b.buffer) === "[object ArrayBuffer]") transfer.add(b.buffer as ArrayBuffer);
  }
  const reply = await channel(worker).call({ kind: "mount", name, files }, [...transfer]);
  if (!reply.ok) throw new Error(reply.error);
}

/** Removes the mount `name`. Removing a name that isn't mounted succeeds. */
export async function unmountFiles(worker: Worker, name: string): Promise<void> {
  const reply = await channel(worker).call({ kind: "unmount", name }, []);
  if (!reply.ok) throw new Error(reply.error);
}
