// Drives the islands exercise (mission #60) in headless Chromium. The page's spec names three
// islands: hero (eager), below (visible, 3000 px down) and narrow (media:(max-width: 600px)).
//
// Checks still waiting on a ticket are listed in `pending`: their failure is reported but doesn't
// fail the run, and their passing does, so the PR that makes one pass has to take it off the list.
// `make exercise-islands` builds dist/ (the bundle, its metafile and the server) first.
import { spawn } from "node:child_process";
import { mkdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const pending = {};

const here = (p) => fileURLToPath(new URL(p, import.meta.url));
const meta = JSON.parse(readFileSync(here("./dist/meta.json"), "utf8"));

// Proof for a person: what the page looked like at each step. Viewport shots only, since a
// full-page shot resizes the viewport and would put every slot in view.
const shots = here("./dist/screenshots/");
mkdirSync(shots, { recursive: true });
const shoot = (page, name) => page.screenshot({ path: shots + name });

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
// Slots whose island has mounted but whose Go-rendered fallback is still there (issue 39's
// one-owner-per-region rule: the island replaces the placeholder).
const fallbacks = [];
const check = (name, ok, detail) => results.push({ name, ok, detail });

const browser = await chromium.launch();
try {
  await serverRendered(browser);
  await wide(browser);
  await narrowViewport(browser);
  await overlay(browser);
} finally {
  await browser.close();
  server.kill();
}

const order = [
  "mount-eager-only",
  "load-eager-only",
  "visible-mounts-on-scroll",
  "visible-loads-on-scroll",
  "media-query",
  "fallback-replaced-on-mount",
  "modulepreload",
  "island-overlay",
  "console",
];
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
console.log(`screenshots: ${shots}`);
console.log(failed ? "exercise-islands: FAIL" : `exercise-islands: PASS${Object.keys(pending).length ? " (with pending checks)" : ""}`);
process.exitCode = failed ? 1 : 0;

// The page with JavaScript off: what Go rendered, a fallback in every slot. A picture only, since
// the Go test (server_test.go) already checks the fallbacks are there.
async function serverRendered(browser) {
  const ctx = await browser.newContext({ viewport: { width: 1200, height: 800 }, javaScriptEnabled: false });
  const page = await ctx.newPage();
  await page.goto(url);
  await shoot(page, "0-server-rendered-no-js.png");
  await ctx.close();
}

// The page at desktop width: what loads and mounts at first, then after scrolling to the bottom.
async function wide(browser) {
  const { page, scripts, errors } = await open(browser, { width: 1200, height: 800 });
  const at = await snapshot(page);
  await shoot(page, "1-wide-at-load.png");
  fallbacks.push(...(await fallbacksLeft(page)).map((s) => `${s} at load, 1200 px`));
  console.log(`  1200 px, at load: requested ${scripts.join(", ")}; loaded [${at.loaded}]; mounted [${at.mounted}]`);

  check("mount-eager-only", same(at.mounted, ["hero"]), `at load, islands mounted [${at.mounted}]; want [hero]`);
  check("load-eager-only", same(at.loaded, ["hero"]), `at load, island modules evaluated [${at.loaded}]; want [hero]`);

  const before = { fallback: at.bottomFallback, loaded: at.loaded.includes("below"), mounted: at.mounted.includes("below") };
  await page.locator('[data-slot="bottom"]').scrollIntoViewIfNeeded();
  await shoot(page, "2-wide-bottom-as-it-scrolls-in.png");
  const mountedAfter = await page
    .waitForFunction(() => window.exercise?.mounted.includes("below"), null, { timeout: 3000 })
    .then(() => true, () => false);
  const loadedAfter = await page.evaluate(() => window.exercise.loaded.includes("below"));
  fallbacks.push(...(await fallbacksLeft(page)).map((s) => `${s} after scrolling`));
  await shoot(page, "3-wide-bottom-after-scrolling.png");
  check(
    "visible-mounts-on-scroll",
    before.fallback && !before.mounted && mountedAfter,
    `before scrolling: fallback shown ${before.fallback}, below mounted ${before.mounted}; after scrolling: mounted ${mountedAfter}`,
  );
  check(
    "visible-loads-on-scroll",
    !before.loaded && loadedAfter,
    `below's module evaluated before scrolling ${before.loaded}, after ${loadedAfter}; want false then true`,
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
  await shoot(page, "4-narrow-at-load.png");
  check(
    "media-query",
    !wide.narrowAtWide && at.mounted.includes("narrow"),
    `narrow mounted at 1200 px: ${wide.narrowAtWide}, at 400 px: ${at.mounted.includes("narrow")}; want false then true`,
  );
  fallbacks.push(...(await fallbacksLeft(page)).map((s) => `${s} at 400 px`));
  check(
    "fallback-replaced-on-mount",
    fallbacks.length === 0,
    fallbacks.length ? `mounted slots still showing their fallback: ${fallbacks.join(", ")}` : "every mounted slot's fallback is gone (top at load, bottom after scrolling, side at 400 px)",
  );
  const all = [...wide.errors, ...errors];
  check("console", all.length === 0, all.length ? all.join(" | ") : "no console errors on either page");
}

// The page with ?islands at desktop width: IslandPage's debug overlay labels each slot with its
// island's state (issue 42), and below's label turns from waiting to mounted as it scrolls in.
async function overlay(browser) {
  const { page, errors } = await open(browser, { width: 1200, height: 800 }, "?islands");
  const labels = () => page.$$eval("[data-island-debug]", (els) => Object.fromEntries(els.map((el) => [el.dataset.slot, el.dataset.islandDebug])));
  const at = await labels();
  await shoot(page, "5-wide-overlay-at-load.png");
  await page.locator('[data-slot="bottom"]').scrollIntoViewIfNeeded();
  await page.waitForFunction(() => window.exercise?.mounted.includes("below"), null, { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(100);
  const after = await labels();
  await shoot(page, "6-wide-overlay-after-scrolling.png");
  check(
    "island-overlay",
    /^hero · top · eager · mounted \d+ ms$/.test(at.top ?? "") &&
      at.bottom === "below · bottom · visible · waiting" &&
      at.side === "narrow · side · media:(max-width: 600px) · waiting" &&
      /^below · bottom · visible · mounted \d+ ms$/.test(after.bottom ?? "") &&
      errors.length === 0,
    `at load: ${JSON.stringify(at)}; after scrolling, bottom: ${JSON.stringify(after.bottom)}`,
  );
}

async function open(browser, viewport, query = "") {
  const page = await browser.newPage({ viewport });
  const scripts = [];
  const errors = [];
  page.on("request", (r) => {
    if (r.resourceType() === "script") scripts.push(new URL(r.url()).pathname);
  });
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(url + query);
  await page.waitForFunction(() => window.exercise?.mounted.includes("hero"), null, { timeout: 10_000 });
  await page.waitForTimeout(500);
  return { page, scripts, errors };
}

function fallbacksLeft(page) {
  return page.$$eval("[data-slot][data-mounted]", (els) => els.filter((el) => el.querySelector("[data-fallback]")).map((el) => el.dataset.slot));
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
