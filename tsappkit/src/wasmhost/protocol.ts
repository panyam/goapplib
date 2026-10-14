// Messages between the page (client.ts) and the wasm host worker (worker.ts). One request message
// and one reply per call, matched by id. Byte buffers travel as transferables, not copies.

/** A mount's files, keyed by slash-separated path within the mount. */
export type Files = Record<string, Uint8Array>;

export type HostRequest =
  | {
      id: number;
      kind: "http";
      method: string;
      url: string;
      headers: Record<string, string>;
      body: Uint8Array | null;
    }
  | { id: number; kind: "mount"; name: string; files: Files }
  | { id: number; kind: "add"; name: string; files: Files }
  /** gc: collect garbage first, so heap.heapAlloc is the live heap (a pause in the worker). */
  | { id: number; kind: "stats"; gc?: boolean }
  | { id: number; kind: "unmount"; name: string }
  /** Cancels the Go context of http request `target`, if it's still running. */
  | { id: number; kind: "cancel"; target: number };

/**
 * Part of an http response, sent when the Go handler flushes (wasmhost's Host.DoStream), before the
 * request's final HostReply. The first chunk of a response carries its status and headers.
 */
export type HostChunk = {
  id: number;
  chunk: true;
  status?: number;
  headers?: Record<string, string>;
  body: Uint8Array;
};

/** The Go heap inside a worker, in bytes, from runtime.MemStats. */
export interface HeapStats {
  /** Bytes of allocated heap objects. Right after a collection (stats with gc), what the worker holds live. */
  heapAlloc: number;
  /** Bytes in heap spans in use, which is heapAlloc plus fragmentation. */
  heapInuse: number;
  /** Bytes allocated over the worker's life, freed or not. */
  totalAlloc: number;
  /** Bytes Go has obtained from the wasm memory, which tracks its size. */
  sys: number;
  /** Collections completed so far. */
  numGC: number;
}

export type HostReply =
  | {
      id: number;
      ok: true;
      status?: number;
      headers?: Record<string, string>;
      body?: Uint8Array;
      /** For stats: the wasm linear memory's size, which only grows. */
      memoryBytes?: number;
      /** For stats: the Go heap, absent from a wasm built against goapplib before 0.7.4. */
      heap?: HeapStats;
    }
  | { id: number; ok: false; error: string };

/** Sent once when the wasm has loaded (or failed to), and again if the Go program exits. */
export type HostStatus = { ready: true } | { ready: false; error: string } | { exited: string };
