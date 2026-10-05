// The worker-state exercise page (goapplib issue 75, mission 74). It measures and records; run.mjs
// decides what passes. Both loads open the state with POST /open, which restores it from the
// worker's store (wasmhost.BrowserStore, issue 76) or ingests it. The first load finds nothing and
// ingests, then runs three long jobs (one with a quick query beside it, one read as a stream, one
// aborted). After a reload, /open should restore, which run.mjs tells from a rebuild by the ingest
// count.
import { startWorker, workerFetch, workerMemory } from "../../../tsappkit/src/wasmhost";

const INGEST = { peakMB: 256, resultMB: 32 };
const JOB_MS = 2000;
const FLAG = "workerstate-ingested";

type Data = Record<string, unknown> & { phase?: string; done?: boolean; error?: string };
const data: Data = {};
(window as unknown as { exercise: Data }).exercise = data;

function log(line: string) {
  document.getElementById("log")!.textContent += line + "\n";
}

async function run() {
  const t0 = performance.now();
  const worker = await startWorker({ worker: "worker.js", wasm: "state.wasm", exec: "wasm_exec.js", ns: "state" });
  const f = workerFetch(worker);
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

  data.memBefore = await workerMemory(worker);
  const ti = performance.now();
  data.ingest = await (await post("/open", INGEST)).json();
  data.ingestMs = Math.round(performance.now() - ti);
  data.memAfter = await workerMemory(worker);
  sessionStorage.setItem(FLAG, "1");
  log(`ingest ${JSON.stringify(INGEST)} in ${data.ingestMs} ms: ${JSON.stringify(data.ingest)}`);
  log(`worker memory ${mb(data.memBefore as number)} before, ${mb(data.memAfter as number)} after`);

  // A quick query 100 ms into a long job.
  const job = post("/longjob", { ms: JOB_MS }).then((r) => r.text());
  await sleep(100);
  const tq = performance.now();
  data.query = await (await post("/query")).json();
  data.queryMs = Math.round(performance.now() - tq);
  await job;
  log(`query during a ${JOB_MS} ms job answered in ${data.queryMs} ms`);

  // A long job read as a stream: when does each progress line arrive?
  const tp = performance.now();
  const res = await post("/longjob", { ms: JOB_MS });
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

  // A long job aborted 300 ms in: how long until the fetch settles, and how did the job end?
  const ac = new AbortController();
  const ta = performance.now();
  const settled = post("/longjob", { ms: JOB_MS }, { signal: ac.signal }).then(
    (r) => r.text().then(() => "resolved"),
    (e: unknown) => (e instanceof DOMException && e.name === "AbortError" ? "aborted" : `rejected: ${String(e)}`),
  );
  await sleep(300);
  const tAbort = performance.now();
  ac.abort();
  data.abortOutcome = await settled;
  data.abortSettleMs = Math.round(performance.now() - tAbort);
  data.abortTotalMs = Math.round(performance.now() - ta);
  data.stateAfterAbort = await getState();
  log(`aborted job: fetch ${data.abortOutcome} ${data.abortSettleMs} ms after the abort; state ${JSON.stringify(data.stateAfterAbort)}`);
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
