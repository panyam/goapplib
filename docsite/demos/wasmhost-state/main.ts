// The wasmhost-state demo: exercise/workerstate's service, which builds a result through a much
// larger scratch allocation and keeps it in wasmhost.BrowserCache. Two lanes, each its own worker:
// "serve" for quick queries, warmed from the cache, and "jobs" for long jobs. On load it opens the
// state (ingesting in a throwaway worker the first time, restoring from the cache after that),
// then checks a query beside a long job, a streamed job, an abort, and a memory limit.
import { oneShotFetch, startLane, workerMemory } from "../../../tsappkit/src/wasmhost";
import { assets, failed, ready } from "../_lib/frame";

const $ = (id: string) => document.getElementById(id)!;
const opts = { ...assets, ns: "state" };
// Small, so a phone can run it: a 64 MB peak on the way to a 4 MB result.
const INGEST = { peakMB: 64, resultMB: 4 };
const LIMIT = 48 << 20;
const sleep = (ms: number) => new Promise((ok) => setTimeout(ok, ms));
const mb = (b: number) => `${(b / 1024 / 1024).toFixed(0)} MB`;

function step(text: string, ok = true) {
  const li = document.createElement("li");
  li.textContent = (ok ? "" : "Failed: ") + text;
  $("steps").append(li);
  if (!ok) throw new Error(text);
}

async function run() {
  const restarts: number[] = [];
  const serve = startLane(opts, {
    // A new serving worker loads the state from the cache, and never ingests.
    warm: async (f) => (await f(location.origin + "/restore", { method: "POST", body: JSON.stringify(INGEST) })).arrayBuffer(),
    maxMemoryBytes: LIMIT,
    onRestart: ({ memoryBytes }) => restarts.push(memoryBytes),
  });
  const post = (path: string, body?: unknown, init: RequestInit = {}) =>
    serve.fetch(location.origin + path, { method: "POST", body: body === undefined ? undefined : JSON.stringify(body), ...init });

  let state = await (await serve.fetch(location.origin + "/state")).json();
  if (state.resultBytes > 0) {
    step(`The serving worker's warm found the ${mb(state.resultBytes)} result in the cache, from an earlier visit, without ingesting.`);
  } else {
    const t = performance.now();
    const job = await (await oneShotFetch(opts)(location.origin + "/open", { method: "POST", body: JSON.stringify(INGEST) })).json();
    const tookMs = Math.round(performance.now() - t);
    state = await (await post("/open", INGEST)).json();
    const mem = await workerMemory(await serve.worker());
    step(
      `Ingested through a ${INGEST.peakMB} MB peak in a throwaway worker (${tookMs} ms), which put the result in the cache and ended. ` +
        `The serving worker restored it from there and holds ${mb(mem)}.`,
      job.ingestCount === 1 && state.restored === true && state.ingestCount === 0 && mem < INGEST.peakMB << 20,
    );
  }

  const jobs = startLane(opts);
  const postJob = (ms: number, init: RequestInit = {}) =>
    jobs.fetch(location.origin + "/longjob", { method: "POST", body: JSON.stringify({ ms }), ...init });

  const long = postJob(1500).then((r) => r.text());
  await sleep(100);
  const tq = performance.now();
  const q = await (await post("/query")).json();
  const queryMs = Math.round(performance.now() - tq);
  await long;
  step(`A query to the serving lane answered in ${queryMs} ms while the jobs lane ran a 1.5 s job.`, q.ok === true && queryMs < 500);

  const ts = performance.now();
  const body = (await postJob(1500)).body!.getReader();
  const arrivals: number[] = [];
  for (let r = await body.read(); !r.done; r = await body.read()) arrivals.push(Math.round(performance.now() - ts));
  step(
    `A 1.5 s job that flushes every 250 ms streamed ${arrivals.length} chunks, the first at ${arrivals[0]} ms.`,
    arrivals.length >= 3 && arrivals[0] < 1000,
  );

  const ac = new AbortController();
  const aborted = postJob(5000, { signal: ac.signal })
    .then((r) => r.text())
    .then(
      () => false,
      () => ac.signal.aborted,
    );
  await sleep(300);
  const ta = performance.now();
  ac.abort();
  const settled = await aborted;
  const settleMs = Math.round(performance.now() - ta);
  const after = await (await jobs.fetch(location.origin + "/state")).json();
  step(`Aborted a 5 s job after 300 ms; it settled ${settleMs} ms later, and the jobs lane's new worker answered.`, settled && typeof after.ingestCount === "number");

  await (await post("/churn", { mb: 64 })).json();
  for (let i = 0; i < 40 && restarts.length === 0; i++) await sleep(50);
  const st = await (await serve.fetch(location.origin + "/state")).json();
  step(
    `A request that churned 64 MB left the serving worker at ${restarts.length ? mb(restarts[0]) : "?"}, past its ${mb(LIMIT)} limit, so the lane replaced it once idle; the new one restored the state from the cache.`,
    restarts.length === 1 && st.restored === true && st.resultBytes === INGEST.resultMB << 20,
  );
  ready();
  wire(serve.fetch, postJob);
}

function wire(serveFetch: typeof fetch, postJob: (ms: number, init?: RequestInit) => Promise<Response>) {
  const out = (s: string) => ($("out").textContent = s);
  const [job, abort, query, reload, clear] = ["job", "abort", "query", "reload", "clear"].map((id) => $(id) as HTMLButtonElement);
  const progress = $("progress") as HTMLProgressElement;
  let ac: AbortController | null = null;
  job.disabled = query.disabled = reload.disabled = clear.disabled = false;

  job.onclick = async () => {
    ac = new AbortController();
    job.disabled = true;
    abort.disabled = false;
    progress.value = 0;
    try {
      const reader = (await postJob(3000, { signal: ac.signal })).body!.pipeThrough(new TextDecoderStream()).getReader();
      let buf = "";
      for (let r = await reader.read(); !r.done; r = await reader.read()) {
        buf += r.value;
        for (let i; (i = buf.indexOf("\n")) >= 0; buf = buf.slice(i + 1)) {
          const line = JSON.parse(buf.slice(0, i));
          if (line.progress !== undefined) progress.value = line.progress;
          if (line.done) out(`The job ${line.done} after ${line.ms} ms.`);
        }
      }
      progress.value = 1;
    } catch {
      out(ac.signal.aborted ? "Aborted: the lane ended that worker and started a fresh one." : "The job failed.");
    } finally {
      job.disabled = false;
      abort.disabled = true;
    }
  };
  abort.onclick = () => ac?.abort();
  query.onclick = async () => {
    const t = performance.now();
    const q = await (await serveFetch(location.origin + "/query", { method: "POST" })).json();
    out(`The serving lane answered in ${Math.round(performance.now() - t)} ms: ${JSON.stringify(q)}`);
  };
  reload.onclick = () => location.reload();
  clear.onclick = async () => {
    const root = await navigator.storage.getDirectory();
    await root.removeEntry("wasmhost", { recursive: true }).catch(() => {});
    location.reload();
  };
}

run().catch(failed);
