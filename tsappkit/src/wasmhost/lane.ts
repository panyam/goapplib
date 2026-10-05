// Lanes (goapplib issue 80): more than one worker for one wasm service, with the app routing
// requests by kind, so a quick request never waits behind a long one. Go's wasm target runs one
// thread per worker, and a job that never yields keeps that thread busy, so neither a second
// request nor an abort can reach it. A lane holds one worker it can replace: aborting a request
// on it terminates the worker, which is the only way to stop a job that never yields, and starts
// a fresh one, warmed from the shared Cache by the app's warm function.
import { endWorker, startWorker, workerFetch, type StartWorkerOptions } from "./client";

type Fetch = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export interface LaneOptions {
  /**
   * Runs on each new worker before the lane hands it a request: typically a request that loads
   * the service's state from the cache (wasmhost.BrowserCache), so a replacement worker answers
   * like the one it replaced. A warm that throws ends that worker, and the error is what the next
   * request gets; the request after it starts another worker.
   */
  warm?: (fetch: Fetch) => Promise<unknown>;
}

/** One replaceable worker. See startLane. */
export interface Lane {
  /**
   * A fetch for a Connect transport, like workerFetch, answered by the lane's current worker.
   * Aborting a request (init.signal) rejects it with the signal's reason at once, terminates the
   * worker, and starts and warms a new one; other requests still waiting on the old worker are
   * rejected too. A request made meanwhile waits for the new worker.
   */
  fetch: Fetch;
  /** The lane's current worker, once it's started and warmed. */
  worker(): Promise<Worker>;
  /** Replaces the worker, as an abort does, and resolves once the new one is warm. */
  restart(): Promise<void>;
  /** Terminates the worker. Requests after this reject. */
  close(): void;
}

/**
 * Starts a lane: a worker from `opts`, warmed with `lane.warm`, that the lane replaces whenever a
 * request on it is aborted. Give each kind of request its own lane (a "serve" lane for quick
 * queries, a "jobs" lane for long jobs) by building each client's transport on that lane's fetch.
 * Every lane of one ns shares the same Cache, which is how a replacement or a second lane gets the
 * state without rebuilding it.
 */
export function startLane(opts: StartWorkerOptions, lane: LaneOptions = {}): Lane {
  let closed = false;
  let current!: Promise<Worker>;
  let failed = false;

  const boot = () => {
    failed = false;
    const p: Promise<Worker> = startWorker(opts).then(async (w) => {
      if (lane.warm) {
        try {
          await lane.warm(workerFetch(w));
        } catch (err) {
          endWorker(w, err instanceof Error ? err : new Error(String(err)));
          throw err;
        }
      }
      return w;
    });
    p.catch(() => {
      if (current === p) failed = true;
    });
    current = p;
    return p;
  };

  const replace = (old: Worker, why: Error) => {
    endWorker(old, why);
    return boot();
  };

  boot();

  return {
    async fetch(input, init) {
      if (closed) throw new Error("wasm host lane: closed");
      const signal = init?.signal ?? (input instanceof Request ? input.signal : undefined);
      if (signal?.aborted) throw abortReason(signal);
      if (failed) boot();
      const booting = current;
      const w = await booting;
      if (signal?.aborted) throw abortReason(signal);
      const run = workerFetch(w)(input, init);
      if (!signal) return run;
      return new Promise<Response>((resolve, reject) => {
        const onAbort = () => {
          reject(abortReason(signal));
          if (current === booting) void replace(w, new Error("wasm host lane: restarted after an abort")).catch(() => {});
        };
        signal.addEventListener("abort", onAbort, { once: true });
        run.then(
          (res) => {
            signal.removeEventListener("abort", onAbort);
            resolve(res);
          },
          (err) => {
            signal.removeEventListener("abort", onAbort);
            reject(err);
          },
        );
      });
    },
    worker: () => current,
    async restart() {
      const old = await current.catch(() => undefined);
      await (old ? replace(old, new Error("wasm host lane: restarted")) : boot());
    },
    close() {
      closed = true;
      void current.then((w) => endWorker(w, new Error("wasm host lane: closed")), () => {});
    },
  };
}

function abortReason(signal: AbortSignal): unknown {
  return signal.reason ?? new DOMException("This request was aborted.", "AbortError");
}
