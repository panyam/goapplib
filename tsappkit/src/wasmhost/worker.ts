// The wasm host worker. It loads a Go wasm build that calls goapplib's wasmhost.Serve (or
// ServeRebuild) and forwards the page's messages to the functions that build exports on
// globalThis[ns]. Running the Go program here, off the page's thread, keeps a request that takes
// seconds from freezing the page.
//
// It is a classic worker, built as an IIFE, because Go's wasm_exec.js is a plain script defining
// globalThis.Go, which importScripts can load and a module worker cannot. The asset URLs and the
// namespace come in the worker's query string (?wasm=&exec=&ns=), which startWorker writes.
import type { Files, HostReply, HostRequest, HostStatus } from "./protocol";
import { workerStore } from "./store";

interface Exports {
  ready?: () => void;
  http(
    method: string,
    url: string,
    headers: Record<string, string>,
    body: Uint8Array | null,
  ): Promise<{ status: number; headers: Record<string, string>; body: Uint8Array }>;
  mount(name: string, files: Files): Promise<void>;
  add(name: string, files: Files): Promise<void>;
  unmount(name: string): Promise<void>;
}

// The project compiles against the DOM lib, which describes a window, and the WebWorker lib can't
// be loaded beside it, so the few worker globals used here are declared.
declare function importScripts(...urls: string[]): void;
declare const self: {
  location: Location;
  postMessage(message: unknown, transfer?: Transferable[]): void;
  onmessage: ((ev: MessageEvent<HostRequest>) => void) | null;
  Go: new () => { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> };
} & Record<string, unknown>;

const params = new URLSearchParams(self.location.search);
const wasmUrl = params.get("wasm") ?? "app.wasm";
const execUrl = params.get("exec") ?? "wasm_exec.js";
const ns = params.get("ns") ?? "wasmhost";

// The wasm's linear memory, exported by Go as `mem`. It only grows, so its size is the peak so far.
let memory: WebAssembly.Memory | undefined;

const post = (m: HostStatus | HostReply, transfer: Transferable[] = []) => self.postMessage(m, transfer);

// Go's wasmhost.BrowserStore finds the store here (store.ts).
const store = workerStore(ns, (self as unknown as { navigator?: { storage?: { getDirectory?: () => Promise<unknown> } } }).navigator?.storage);
if (store) self.wasmhostStore = store;

async function boot(): Promise<Exports> {
  importScripts(execUrl);
  const go = new self.Go();
  const ready = new Promise<void>((resolve) => {
    self[ns] = { ready: resolve };
  });
  const res = await fetch(wasmUrl);
  if (!res.ok) throw new Error(`fetch ${wasmUrl}: ${res.status}`);
  // instantiateStreaming insists on the application/wasm content type, which not every static
  // server sends.
  const { instance } = res.headers.get("content-type")?.startsWith("application/wasm")
    ? await WebAssembly.instantiateStreaming(res, go.importObject)
    : await WebAssembly.instantiate(await res.arrayBuffer(), go.importObject);
  memory = instance.exports.mem as WebAssembly.Memory | undefined;
  void go.run(instance).then(() => post({ exited: "the Go program exited" }));
  await ready;
  return self[ns] as Exports;
}

const booted = boot();
booted.then(
  () => post({ ready: true }),
  (err: unknown) => post({ ready: false, error: String(err) }),
);

self.onmessage = async (ev) => {
  const req = ev.data;
  try {
    const host = await booted;
    if (req.kind === "http") {
      const res = await host.http(req.method, req.url, req.headers, req.body);
      post({ id: req.id, ok: true, status: res.status, headers: res.headers, body: res.body }, [res.body.buffer]);
    } else if (req.kind === "mount") {
      await host.mount(req.name, req.files);
      post({ id: req.id, ok: true });
    } else if (req.kind === "add") {
      await host.add(req.name, req.files);
      post({ id: req.id, ok: true });
    } else if (req.kind === "stats") {
      post({ id: req.id, ok: true, memoryBytes: memory?.buffer.byteLength ?? 0 });
    } else if (req.kind === "unmount") {
      await host.unmount(req.name);
      post({ id: req.id, ok: true });
    } else {
      // A page newer than this worker can send a kind it doesn't know; say so rather than guess.
      throw new Error(`unknown request kind ${JSON.stringify((req as { kind: unknown }).kind)}`);
    }
  } catch (err) {
    post({ id: req.id, ok: false, error: err instanceof Error ? err.message : String(err) });
  }
};
