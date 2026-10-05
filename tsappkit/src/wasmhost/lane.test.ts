import { afterEach, describe, expect, it, vi } from "vitest";
import { startLane } from "./lane";
import type { HostReply, HostRequest, HostStatus } from "./protocol";

// Each fake worker reports ready, then answers an http request by its path: "/slow" never
// answers (a job that never yields), anything else answers at once with the worker's number.
class LaneWorker extends EventTarget {
  static all: LaneWorker[] = [];
  n = LaneWorker.all.length + 1;
  terminated = false;
  paths: string[] = [];

  constructor(_url: string | URL) {
    super();
    LaneWorker.all.push(this);
    queueMicrotask(() => this.send({ ready: true }));
  }

  postMessage(req: HostRequest) {
    if (this.terminated || req.kind !== "http") return;
    this.paths.push(req.url);
    if (req.url === "/slow") return;
    const body = new TextEncoder().encode(`worker ${this.n}: ${req.url}`);
    queueMicrotask(() => this.send({ id: req.id, ok: true, status: 200, body }));
  }

  terminate() {
    this.terminated = true;
  }

  send(m: HostReply | HostStatus) {
    if (!this.terminated) this.dispatchEvent(new MessageEvent("message", { data: m }));
  }
}

const opts = { worker: "w.js", wasm: "a.wasm", exec: "e.js", ns: "state" };
const text = async (r: Promise<Response>) => (await r).text();

afterEach(() => {
  vi.unstubAllGlobals();
  LaneWorker.all = [];
});

describe("startLane", () => {
  it("warms each worker before it answers, and answers on the warmed worker", async () => {
    vi.stubGlobal("Worker", LaneWorker);
    const warmed: number[] = [];
    const lane = startLane(opts, { warm: async (f) => warmed.push(Number((await (await f("http://x/open")).text()).split(":")[0].split(" ")[1])) });
    expect(await text(lane.fetch("http://x/state"))).toBe("worker 1: /state");
    expect(warmed).toEqual([1]);
    expect(LaneWorker.all[0].paths).toEqual(["/open", "/state"]);
  });

  it("on an abort, rejects at once, ends the busy worker, and answers the next request on a new, warmed one", async () => {
    vi.stubGlobal("Worker", LaneWorker);
    let warms = 0;
    const lane = startLane(opts, { warm: async () => void warms++ });
    await lane.worker();
    const ac = new AbortController();
    const job = lane.fetch("http://x/slow", { signal: ac.signal });
    const other = lane.fetch("http://x/slow");
    await new Promise((r) => setTimeout(r, 5));
    ac.abort();
    await expect(job).rejects.toMatchObject({ name: "AbortError" });
    await expect(other).rejects.toThrow("restarted after an abort");
    expect(LaneWorker.all[0].terminated).toBe(true);
    expect(await text(lane.fetch("http://x/state"))).toBe("worker 2: /state");
    expect(warms).toBe(2);
  });

  it("keeps two lanes apart: a job holding one doesn't hold up the other", async () => {
    vi.stubGlobal("Worker", LaneWorker);
    const serve = startLane(opts);
    const jobs = startLane(opts);
    void jobs.fetch("http://x/slow");
    await new Promise((r) => setTimeout(r, 5));
    expect(await text(serve.fetch("http://x/query"))).toBe("worker 1: /query");
  });

  it("gives a failed warm's error to the next request, and starts another worker for the one after", async () => {
    vi.stubGlobal("Worker", LaneWorker);
    let tries = 0;
    const lane = startLane(opts, {
      warm: async () => {
        if (++tries === 1) throw new Error("cache unreadable");
      },
    });
    await expect(lane.fetch("http://x/state")).rejects.toThrow("cache unreadable");
    expect(LaneWorker.all[0].terminated).toBe(true);
    expect(await text(lane.fetch("http://x/state"))).toBe("worker 2: /state");
  });

  it("rejects an already-aborted request without touching the worker, and everything after close", async () => {
    vi.stubGlobal("Worker", LaneWorker);
    const lane = startLane(opts);
    await lane.worker();
    const ac = new AbortController();
    ac.abort();
    await expect(lane.fetch("http://x/slow", { signal: ac.signal })).rejects.toMatchObject({ name: "AbortError" });
    expect(LaneWorker.all).toHaveLength(1);
    expect(LaneWorker.all[0].terminated).toBe(false);
    lane.close();
    await expect(lane.fetch("http://x/state")).rejects.toThrow("closed");
    await new Promise((r) => setTimeout(r, 0));
    expect(LaneWorker.all[0].terminated).toBe(true);
  });
});

describe("startLane with a streamed response", () => {
  it("ends the worker when a request is aborted after its body has started streaming", async () => {
    // The worker answers /stream with one chunk and then stays busy, as a long job that reports
    // progress does.
    class StreamWorker extends LaneWorker {
      postMessage(req: HostRequest) {
        if (this.terminated || req.kind !== "http") return;
        if (req.url !== "/stream") return super.postMessage(req);
        const body = new TextEncoder().encode("p1\n");
        queueMicrotask(() =>
          this.dispatchEvent(new MessageEvent("message", { data: { id: req.id, chunk: true, status: 200, headers: {}, body } })),
        );
      }
    }
    vi.stubGlobal("Worker", StreamWorker);
    const lane = startLane(opts);
    await lane.worker();
    const ac = new AbortController();
    const res = await lane.fetch("http://x/stream", { signal: ac.signal });
    const reader = res.body!.getReader();
    expect(new TextDecoder().decode((await reader.read()).value)).toBe("p1\n");
    ac.abort();
    await expect(reader.read()).rejects.toMatchObject({ name: "AbortError" });
    expect(LaneWorker.all[0].terminated).toBe(true);
    expect(await text(lane.fetch("http://x/state"))).toBe("worker 2: /state");
  });
});
