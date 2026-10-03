import { afterEach, describe, expect, it, vi } from "vitest";
import { mountFiles, startWorker, unmountFiles, workerFetch } from "./client";
import type { HostReply, HostRequest, HostStatus } from "./protocol";

// FakeWorker passes each message through structuredClone with its transfer list, as postMessage
// does, so a transferred buffer really is detached and a duplicate transfer really throws.
class FakeWorker extends EventTarget {
  static last: FakeWorker;
  url: string;
  received: HostRequest[] = [];
  answer: (req: HostRequest) => { reply: HostReply; transfer?: Transferable[] } | null = (req) => ({
    reply: { id: req.id, ok: true },
  });

  constructor(url: string | URL) {
    super();
    this.url = String(url);
    FakeWorker.last = this;
  }

  postMessage(msg: HostRequest, transfer: Transferable[] = []) {
    const req = structuredClone(msg, { transfer });
    this.received.push(req);
    const out = this.answer(req);
    if (out) queueMicrotask(() => this.send(out.reply, out.transfer));
  }

  send(m: HostReply | HostStatus, transfer: Transferable[] = []) {
    this.dispatchEvent(new MessageEvent("message", { data: structuredClone(m, { transfer }) }));
  }
}

const worker = () => new FakeWorker("w.js") as unknown as Worker & FakeWorker;
const bytes = (s: string) => new TextEncoder().encode(s);
const str = (b?: Uint8Array) => new TextDecoder().decode(b);

afterEach(() => vi.unstubAllGlobals());

describe("workerFetch", () => {
  it("sends the request as one message and builds the Response from the reply", async () => {
    const w = worker();
    w.answer = (req) => ({
      reply: { id: req.id, ok: true, status: 201, headers: { "content-type": "application/json" }, body: bytes('{"ok":1}') },
    });
    const body = bytes('{"q":1}');
    const res = await workerFetch(w)("http://anything.example/pkg.Svc/Get?x=1", {
      method: "POST",
      headers: { "Content-Type": "application/json", "Connect-Protocol-Version": "1" },
      body,
    });
    const req = w.received[0];
    expect(req).toMatchObject({ kind: "http", method: "POST", url: "/pkg.Svc/Get?x=1" });
    expect(req.kind === "http" && req.headers).toMatchObject({
      "content-type": "application/json",
      "connect-protocol-version": "1",
    });
    expect(req.kind === "http" && str(req.body ?? undefined)).toBe('{"q":1}');
    expect(res.status).toBe(201);
    expect(res.headers.get("content-type")).toBe("application/json");
    expect(await res.text()).toBe('{"ok":1}');
  });

  it("sends a GET with no body as null", async () => {
    const w = worker();
    await workerFetch(w)("http://x/a");
    expect(w.received[0]).toMatchObject({ kind: "http", method: "GET", body: null });
  });

  it("gives a 204 a null body", async () => {
    const w = worker();
    w.answer = (req) => ({ reply: { id: req.id, ok: true, status: 204, body: new Uint8Array() } });
    const res = await workerFetch(w)("http://x/a");
    expect(res.status).toBe(204);
    expect(res.body).toBeNull();
  });

  it("rejects with the worker's error", async () => {
    const w = worker();
    w.answer = (req) => ({ reply: { id: req.id, ok: false, error: "wasmhost: no handler" } });
    await expect(workerFetch(w)("http://x/a")).rejects.toThrow("wasmhost: no handler");
  });

  it("matches replies to requests by id when they come back out of order", async () => {
    const w = worker();
    const held: HostRequest[] = [];
    w.answer = (req) => (held.push(req), null);
    const f = workerFetch(w);
    const a = f("http://x/a");
    const b = f("http://x/b");
    await vi.waitFor(() => expect(held).toHaveLength(2));
    for (const req of [...held].reverse()) {
      if (req.kind === "http") w.send({ id: req.id, ok: true, status: 200, body: bytes(req.url) });
    }
    expect(await (await a).text()).toBe("/a");
    expect(await (await b).text()).toBe("/b");
  });

  it("rejects pending and later calls once the Go program exits", async () => {
    const w = worker();
    w.answer = () => null;
    const pending = workerFetch(w)("http://x/a");
    await vi.waitFor(() => expect(w.received).toHaveLength(1));
    w.send({ exited: "the Go program exited" });
    await expect(pending).rejects.toThrow("exited");
    await expect(workerFetch(w)("http://x/b")).rejects.toThrow("exited");
  });
});

describe("mountFiles", () => {
  it("transfers each buffer once, even when files share one", async () => {
    const w = worker();
    const shared = bytes("abcdef");
    const files = { "a.txt": shared.subarray(0, 3), "b.txt": shared.subarray(3), "c.txt": bytes("c") };
    await mountFiles(w, "docs", files);
    const req = w.received[0];
    expect(req.kind).toBe("mount");
    if (req.kind !== "mount") return;
    expect(req.name).toBe("docs");
    expect(Object.fromEntries(Object.entries(req.files).map(([k, v]) => [k, str(v)]))).toEqual({
      "a.txt": "abc",
      "b.txt": "def",
      "c.txt": "c",
    });
    expect(shared.buffer.byteLength).toBe(0);
    expect(files["c.txt"].byteLength).toBe(0);
  });

  it("rejects with the worker's error", async () => {
    const w = worker();
    w.answer = (req) => ({ reply: { id: req.id, ok: false, error: "mount a/b: invalid argument" } });
    await expect(mountFiles(w, "a/b", {})).rejects.toThrow("invalid argument");
  });

  it("unmounts by name", async () => {
    const w = worker();
    await unmountFiles(w, "docs");
    expect(w.received[0]).toMatchObject({ kind: "unmount", name: "docs" });
  });
});

describe("startWorker", () => {
  it("passes absolute asset URLs and the namespace, and resolves on ready", async () => {
    vi.stubGlobal("Worker", FakeWorker);
    const started = startWorker({ worker: "/assets/worker.js", wasm: "app.wasm", exec: "/go/wasm_exec.js", ns: "files" });
    const w = FakeWorker.last;
    const url = new URL(w.url);
    expect(url.pathname).toBe("/assets/worker.js");
    expect(url.searchParams.get("wasm")).toBe(new URL("app.wasm", location.href).href);
    expect(url.searchParams.get("exec")).toBe(new URL("/go/wasm_exec.js", location.href).href);
    expect(url.searchParams.get("ns")).toBe("files");
    w.send({ ready: true });
    expect(await started).toBe(w);
  });

  it("rejects with the load error", async () => {
    vi.stubGlobal("Worker", FakeWorker);
    const started = startWorker({ worker: "w.js", wasm: "a.wasm", exec: "e.js", ns: "x" });
    FakeWorker.last.send({ ready: false, error: "fetch a.wasm: 404" });
    await expect(started).rejects.toThrow("fetch a.wasm: 404");
  });
});
