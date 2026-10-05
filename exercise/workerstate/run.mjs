// Drives the worker-state exercise (mission #74, goapplib issue 75) in headless Chromium: the page
// loads, ingests and runs its long jobs, then reloads and reads its state back.
//
// Checks still waiting on a ticket are listed in `pending`: their failure is reported but doesn't
// fail the run, and their passing does, so the PR that makes one pass has to take it off the list.
// `make exercise-worker-state` builds dist/ first. Pass --serve to leave the server up for a browser.
import { createReadStream, existsSync, mkdirSync } from "node:fs";
import { createServer } from "node:http";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const pending = {
};

const dist = fileURLToPath(new URL("./dist/", import.meta.url));
const shots = join(dist, "screenshots/");
const types = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm", ".map": "application/json" };

const server = createServer((req, res) => {
  const path = normalize(new URL(req.url, "http://x").pathname).replace(/^\/+/, "") || "index.html";
  const file = join(dist, path);
  if (!file.startsWith(dist) || !existsSync(file)) {
    res.writeHead(404).end();
    return;
  }
  res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream" });
  createReadStream(file).pipe(res);
});
await new Promise((ok) => server.listen(0, "127.0.0.1", ok));
const url = `http://127.0.0.1:${server.address().port}/`;

if (process.argv.includes("--serve")) {
  console.log(`serving ${dist} at ${url}`);
} else {
  try {
    process.exitCode = await drive(url);
  } finally {
    server.close();
  }
}

async function drive(url) {
  mkdirSync(shots, { recursive: true });
  const browser = await chromium.launch();
  const results = [];
  const check = (name, ok, detail) => results.push({ name, ok, detail });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
    page.on("pageerror", (e) => errors.push(e.message));
    const workers = [];
    page.on("worker", (w) => {
      const seen = { closed: false };
      workers.push(seen);
      w.on("close", () => (seen.closed = true));
      w.on("console", (m) => m.type() === "error" && errors.push(`worker: ${m.text()}`));
    });

    const first = await load(page, () => page.goto(url));
    // A terminated worker's close event can land just after the page says it's done.
    for (let i = 0; i < 20 && workers.filter((w) => w.closed).length < 2; i++) await page.waitForTimeout(100);
    const firstWorkers = workers.map((w) => ({ ...w }));
    await page.screenshot({ path: shots + "1-first-load.png" });
    const again = await load(page, () => page.reload());
    await page.screenshot({ path: shots + "2-after-reload.png" });

    const mb = (b) => `${(b / 1024 / 1024).toFixed(1)} MB`;
    const result = first.ingest?.resultBytes ?? 0;
    console.log(
      `  ingest in a throwaway worker ${first.jobMs} ms, restored by the serving worker in ${first.restoreMs} ms; serving worker memory ${mb(first.memBefore)} before, ${mb(first.memAfter)} after (result ${mb(result)})`,
    );


    check(
      "ingest",
      !first.error && result === 32 << 20 && /^[0-9a-f]{64}$/.test(first.ingest?.checksum ?? ""),
      first.error ? `page error: ${first.error}` : `ingest built ${mb(result)} with checksum ${first.ingest?.checksum?.slice(0, 12)}…`,
    );

    console.log(`  after reload, /open took ${again.openMs} ms (restored: ${again.state?.restored})`);

    check(
      "ingest-in-throwaway-worker",
      first.job?.ingestCount === 1 &&
        first.job?.restored === false &&
        first.ingest?.restored === true &&
        first.ingest?.ingestCount === 0 &&
        first.ingest?.checksum === first.job?.checksum &&
        firstWorkers.length === 4 &&
        firstWorkers.filter((w) => w.closed).length === 2,
      `throwaway worker: ingestCount ${first.job?.ingestCount}, restored ${first.job?.restored}; serving worker: restored ${first.ingest?.restored}, ingestCount ${first.ingest?.ingestCount}; ${firstWorkers.length} workers started on the first load and ${firstWorkers.filter((w) => w.closed).length} ended (want 4 and 2: serving, throwaway (ended), jobs lane (ended by the abort), its replacement)`,
    );

    const growth = first.memAfter - first.memBefore;
    check(
      "memory-after-ingest",
      result > 0 && growth < 2 * result,
      `the serving worker grew by ${mb(growth)} for a ${mb(result)} result; want under ${mb(2 * result)}`,
    );

    check(
      "restore-on-reload",
      !again.error && again.state?.restored === true && again.state?.checksum === first.ingest?.checksum && again.state?.ingestCount === 0,
      `after reload: ${JSON.stringify(again.state ?? again.error)}; want restored, checksum ${first.ingest?.checksum?.slice(0, 12)}…, ingestCount 0`,
    );

    check(
      "quick-query-during-long-job",
      first.query?.ok === true && first.queryMs < 200,
      `a query 100 ms into a 2000 ms job answered in ${first.queryMs} ms; want under 200`,
    );

    check(
      "abort-stops-long-job",
      first.abortOutcome === "aborted" && first.abortSettleMs < 300 && typeof first.laneAfterAbort?.ingestCount === "number",
      `fetch ${first.abortOutcome} ${first.abortSettleMs} ms after the abort (${first.abortError}); the jobs lane answered again in ${first.laneAfterAbortMs} ms (${first.laneAfterAbort ? "yes" : "no"}); want aborted within 300 ms, and a lane that answers afterwards`,
    );

    const arrivals = first.progressArrivals ?? [];
    const end = arrivals.length ? arrivals[arrivals.length - 1].at : 0;
    const early = arrivals.filter((a) => a.line.includes('"progress"') && a.at < end - 200);
    check(
      "progress",
      early.length >= 3,
      `${early.length} progress lines arrived while the job ran (lines at ${arrivals.map((a) => a.at).join(", ")} ms); want at least 3`,
    );

    check("console", errors.length === 0, errors.length ? errors.join(" | ") : "no console errors");
  } finally {
    await browser.close();
  }

  let failed = false;
  for (const r of results) {
    const ticket = pending[r.name];
    let status;
    if (ticket && r.ok) {
      status = `XPASS (passes but is still pending on ${ticket}; take it off the pending list in run.mjs)`;
      failed = true;
    } else if (ticket) {
      status = `FAIL (pending ${ticket})`;
    } else {
      status = r.ok ? "PASS" : "FAIL";
      failed ||= !r.ok;
    }
    console.log(`${status} ${r.name}: ${r.detail}`);
  }
  console.log(`screenshots: ${shots}`);
  console.log(
    failed ? "exercise-worker-state: FAIL" : `exercise-worker-state: PASS${Object.keys(pending).length ? " (with pending checks)" : ""}`,
  );
  return failed ? 1 : 0;
}

async function load(page, navigate) {
  await navigate();
  await page.waitForFunction(() => window.exercise?.done, null, { timeout: 120_000 });
  return page.evaluate(() => window.exercise);
}
