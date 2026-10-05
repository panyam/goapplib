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
  | { id: number; kind: "stats" }
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

export type HostReply =
  | {
      id: number;
      ok: true;
      status?: number;
      headers?: Record<string, string>;
      body?: Uint8Array;
      /** For stats: the wasm linear memory's size, which only grows. */
      memoryBytes?: number;
    }
  | { id: number; ok: false; error: string };

/** Sent once when the wasm has loaded (or failed to), and again if the Go program exits. */
export type HostStatus = { ready: true } | { ready: false; error: string } | { exited: string };
