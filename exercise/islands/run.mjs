// Drives the islands exercise (mission #60) in headless Chromium. The page's spec names three
// islands: hero (eager), below (visible, 3000 px down) and narrow (media:(max-width: 600px)).
//
// Checks still waiting on a ticket are listed in `pending`: their failure is reported but doesn't
// fail the run, and their passing does, so the PR that makes one pass has to take it off the list.
// `make exercise-islands` builds dist/ (the bundle, its metafile and the server) first.
import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const pending = {
  "load-eager-only": "#35 and #36",
  "visible-on-scroll": "#35 and #36",
  "media-query": "#36",
  modulepreload: "#35",
};

const here = (p) => fileURLToPath(new URL(p, import.meta.url));
const meta = JSON.parse(readFileSync(here("./dist/meta.json"), "utf8"));

const server = spawn(here("./dist/server"), ["-dist", here("./dist"), "-partial", here("../../templates/page/Islands.html")], {
  stdio: ["ignore", "pipe", "inherit"],
});
const url = await new Promise((ok, fail) => {
  server.stdout.on("data", (b) => {
    const m = /listening on (\S+)/.exec(String(b));
    if (m) ok(m[1]);
  });
  server.on("exit", (code) => fail(new Error(`server exited with ${code}`)));
});

const results = [];
const check = (name, ok, detail) => results.push({ name, ok, detail });

const browser = await chromium.launch();
try {
  await wide(browser);
  await narrowViewport(browser);
} finally {
  await browser.close();
  server.kill();
}

const order = ["load-eager-only", "visible-on-scroll", "media-query", "modulepreload", "console"];
results.sort((a, b) => order.indexOf(a.name) - order.indexOf(b.name));
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
console.log(failed ? "exercise-islands: FAIL" : "exercise-islands: PASS (with pending checks)");
process.exitCode = failed ? 1 : 0;

// The page at desktop width: what loads and mounts at first, then after scrolling to the bottom.
async function wide(browser) {
  const { page, scripts, errors } = await open(browser, { width: 1200, height: 800 });
  const at = await snapshot(page);
  console.log(`  1200 px, at load: requested ${scripts.join(", ")}; loaded [${at.loaded}]; mounted [${at.mounted}]`);

  check(
    "load-eager-only",
    same(at.loaded, ["hero"]) && same(at.mounted, ["hero"]),
    `at load, island modules evaluated [${at.loaded}] and islands mounted [${at.mounted}]; want [hero] for both`,
  );

  const before = { fallback: at.bottomFallback, loaded: at.loaded.includes("below"), mounted: at.mounted.includes("below") };
  await page.locator('[data-slot="bottom"]').scrollIntoViewIfNeeded();
  const mountedAfter = await page
    .waitForFunction(() => window.exercise?.mounted.includes("below"), null, { timeout: 3000 })
    .then(() => true, () => false);
  check(
    "visible-on-scroll",
    before.fallback && !before.loaded && !before.mounted && mountedAfter,
    `before scrolling: fallback shown ${before.fallback}, below loaded ${before.loaded}, mounted ${before.mounted}; after scrolling: mounted ${mountedAfter}`,
  );

  const narrowAtWide = at.mounted.includes("narrow");

  const links = await page.$$eval('link[rel="modulepreload"]', (ls) => ls.map((l) => new URL(l.href).pathname));
  const want = eagerChunks();
  check(
    "modulepreload",
    want !== null && same(links, want),
    want === null
      ? `hero has no chunk of its own (the registry imports it with the page), so there is nothing to preload; page links: [${links}]`
      : `page links [${links}], want the eager chunks [${want}]`,
  );

  wide.narrowAtWide = narrowAtWide;
  wide.errors = errors;
}

// The page at phone width, where narrow's media query matches.
async function narrowViewport(browser) {
  const { page, errors } = await open(browser, { width: 400, height: 800 });
  const at = await snapshot(page);
  check(
    "media-query",
    !wide.narrowAtWide && at.mounted.includes("narrow"),
    `narrow mounted at 1200 px: ${wide.narrowAtWide}, at 400 px: ${at.mounted.includes("narrow")}; want false then true`,
  );
  const all = [...wide.errors, ...errors];
  check("console", all.length === 0, all.length ? all.join(" | ") : "no console errors on either page");
}

async function open(browser, viewport) {
  const page = await browser.newPage({ viewport });
  const scripts = [];
  const errors = [];
  page.on("request", (r) => {
    if (r.resourceType() === "script") scripts.push(new URL(r.url()).pathname);
  });
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(url);
  await page.waitForFunction(() => window.exercise?.mounted.includes("hero"), null, { timeout: 10_000 });
  await page.waitForTimeout(500);
  return { page, scripts, errors };
}

function snapshot(page) {
  return page.evaluate(() => ({
    loaded: [...window.exercise.loaded],
    mounted: [...window.exercise.mounted],
    bottomFallback: document.querySelector('[data-slot="bottom"] [data-fallback]') !== null,
  }));
}

// The chunks the eager island (hero) needs besides the entry, from esbuild's metafile: the output
// files holding hero's module, plus what they import. null when hero lives in the entry itself.
function eagerChunks() {
  const outputs = meta.outputs;
  const entry = Object.keys(outputs).find((o) => outputs[o].entryPoint?.endsWith("web/main.ts"));
  const heroOut = Object.keys(outputs).filter(
    (o) => o !== entry && o.endsWith(".js") && Object.keys(outputs[o].inputs).some((i) => i.endsWith("web/islands/hero.ts")),
  );
  if (heroOut.length === 0) return null;
  const seen = new Set();
  const visit = (o) => {
    if (seen.has(o) || o === entry) return;
    seen.add(o);
    for (const imp of outputs[o].imports ?? []) if (imp.kind === "import-statement") visit(imp.path);
  };
  heroOut.forEach(visit);
  return [...seen].map((o) => "/static/" + o.replace(/^.*?dist\//, "")).sort();
}

function same(a, b) {
  return JSON.stringify([...a].sort()) === JSON.stringify([...b].sort());
}
