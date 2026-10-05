// The worker-state exercise page (goapplib issue 75, mission 74). It measures and records; run.mjs
// decides what passes. Both loads open the state with POST /open, which restores it from the
// worker's cache (wasmhost.BrowserCache, issue 76) or ingests it. The first load sends its /open
// through oneShotFetch first (issue 77), so a throwaway worker does the ingest and the serving
// worker restores; then it runs three long jobs (one with a quick query beside it, one read as a stream, one
// aborted). After a reload, /open should restore, which run.mjs tells from a rebuild by the ingest
// count.
import { oneShotFetch, startLane, workerMemory } from "../../../tsappkit/src/wasmhost";

const INGEST = { peakMB: 256, resultMB: 32 };
const JOB_MS = 2000;
const FLAG = "workerstate-ingested";
const WATERMARK = 96 << 20;
const CHURN_MB = 100;

type Data = Record<string, unknown> & { phase?: string; done?: boolean; error?: string };
const data: Data = {};
(window as unknown as { exercise: Data }).exercise = data;

function log(line: string) {
  document.getElementById("log")!.textContent += line + "\n";
}

async function run() {
  const t0 = performance.now();
  const opts = { worker: "worker.js", wasm: "state.wasm", exec: "wasm_exec.js", ns: "state" };
  // The serving worker is a lane too (issue 79): its warm restores the state from the cache (and
  // never ingests, so a replacement can't pull in the ingest's peak), and past 96 MB of wasm memory
  // the lane swaps it for a fresh, warmed worker once it's idle.
  const restarts: number[] = [];
  data.restarts = restarts;
  const serve = startLane(opts, {
    warm: async (wf) => {
      const r = await wf(location.origin + "/restore", { method: "POST", body: JSON.stringify(INGEST) });
      if (r.status !== 200 && r.status !== 404) throw new Error(`restore: ${r.status}`);
      await r.arrayBuffer();
    },
    maxMemoryBytes: WATERMARK,
    onRestart: ({ memoryBytes }) => restarts.push(memoryBytes),
  });
  await serve.worker();
  const f = serve.fetch;
  const post = (path: string, body?: unknown, init: RequestInit = {}) =>
    f(location.origin + path, { method: "POST", body: body === undefined ? undefined : JSON.stringify(body), ...init });
  const getState = async () => (await f(location.origin + "/state")).json();
  log(`worker ready in ${Math.round(performance.now() - t0)} ms`);

  if (sessionStorage.getItem(FLAG)) {
    data.phase = "reload";
    const to = performance.now();
    data.open = await (await post("/open", INGEST)).json();
    data.openMs = Math.round(performance.now() - to);
    data.state = await getState();
    log(`after reload, state: ${JSON.stringify(data.state)}`);
    return;
  }
  data.phase = "first";

  // The ingest runs in a throwaway worker (issue 77), which misses the cache, builds the result
  // through its 256 MB peak, puts it in the cache and is terminated. The serving worker then opens
  // the same inputs, finds them in the cache, and only ever holds the 32 MB result.
  data.memBefore = await workerMemory(await serve.worker());
  const ti = performance.now();
  const oneShot = oneShotFetch(opts);
  data.job = await (await oneShot(location.origin + "/open", { method: "POST", body: JSON.stringify(INGEST) })).json();
  data.jobMs = Math.round(performance.now() - ti);
  const tr = performance.now();
  data.ingest = await (await post("/open", INGEST)).json();
  data.restoreMs = Math.round(performance.now() - tr);
  data.ingestMs = Math.round(performance.now() - ti);
  data.memAfter = await workerMemory(await serve.worker());
  sessionStorage.setItem(FLAG, "1");
  log(`ingest ${JSON.stringify(INGEST)} in a throwaway worker in ${data.jobMs} ms: ${JSON.stringify(data.job)}`);
  log(`serving worker opened it from the cache in ${data.restoreMs} ms: ${JSON.stringify(data.ingest)}`);
  log(`worker memory ${mb(data.memBefore as number)} before, ${mb(data.memAfter as number)} after`);

  // Long jobs go to their own lane (issue 80), a second worker, so the serving worker stays free
  // for queries, and aborting a job ends that lane's worker and starts a fresh one. The toy's long
  // job needs no state, so the lane has no warm; agni's would restore from the cache.
  const jobs = startLane(opts);
  await jobs.worker();
  const postJob = (body: unknown, init: RequestInit = {}) =>
    jobs.fetch(location.origin + "/longjob", { method: "POST", body: JSON.stringify(body), ...init });

  // A quick query 100 ms into a long job.
  const job = postJob({ ms: JOB_MS }).then((r) => r.text());
  await sleep(100);
  const tq = performance.now();
  data.query = await (await post("/query")).json();
  data.queryMs = Math.round(performance.now() - tq);
  await job;
  log(`query during a ${JOB_MS} ms job answered in ${data.queryMs} ms`);

  // A long job read as a stream: when does each progress line arrive?
  const tp = performance.now();
  const res = await postJob({ ms: JOB_MS });
  const reader = res.body!.getReader();
  const arrivals: { at: number; line: string }[] = [];
  const dec = new TextDecoder();
  let buf = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    let i;
    while ((i = buf.indexOf("\n")) >= 0) {
      arrivals.push({ at: Math.round(performance.now() - tp), line: buf.slice(0, i) });
      buf = buf.slice(i + 1);
    }
  }
  data.progressArrivals = arrivals;
  log(`streamed job: ${arrivals.length} lines, arriving at ${arrivals.map((a) => a.at).join(", ")} ms`);

  // A long job aborted 300 ms in: how long until the fetch settles, and does the lane answer again?
  const ac = new AbortController();
  const ta = performance.now();
  // Since issue 78 the job's response arrives at its first progress line, before the abort, so the
  // abort usually lands while its body is streaming: reading the body is what fails then.
  const settled = postJob({ ms: JOB_MS }, { signal: ac.signal })
    .then((r) => r.text())
    .then(
      () => "resolved",
      (e: unknown) => {
        data.abortError = e instanceof Error ? `${e.name}: ${e.message}` : String(e);
        return ac.signal.aborted ? "aborted" : `rejected: ${String(e)}`;
      },
    );
  await sleep(300);
  const tAbort = performance.now();
  ac.abort();
  data.abortOutcome = await settled;
  data.abortSettleMs = Math.round(performance.now() - tAbort);
  data.abortTotalMs = Math.round(performance.now() - ta);
  const tl = performance.now();
  data.laneAfterAbort = await (await jobs.fetch(location.origin + "/state")).json();
  data.laneAfterAbortMs = Math.round(performance.now() - tl);
  log(`aborted job: fetch ${data.abortOutcome} ${data.abortSettleMs} ms after the abort; the jobs lane answered again in ${data.laneAfterAbortMs} ms`);

  // A request that allocates and drops 100 MB leaves the serving worker past its 96 MB limit; once
  // it's idle the lane replaces it, and the next request is answered by a warmed replacement.
  data.memBeforeChurn = await workerMemory(await serve.worker());
  await (await post("/churn", { mb: CHURN_MB })).json();
  // The lane measures once the churn's response is read and it's idle; wait for that, not a guess.
  for (let i = 0; i < 40 && restarts.length === 0; i++) await sleep(50);
  data.stateAfterChurn = await getState();
  data.memAfterChurn = await workerMemory(await serve.worker());
  log(`after a ${CHURN_MB} MB churn: lane restarts ${JSON.stringify(restarts.map(mb))}; state ${JSON.stringify(data.stateAfterChurn)}; worker memory ${mb(data.memAfterChurn as number)}`);
}

const sleep = (ms: number) => new Promise((ok) => setTimeout(ok, ms));
const mb = (b: number) => `${(b / 1024 / 1024).toFixed(1)} MB`;

run()
  .catch((err: unknown) => {
    data.error = String(err);
    log(`error: ${data.error}`);
  })
  .finally(() => {
    data.done = true;
  });
