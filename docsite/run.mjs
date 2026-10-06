// Mission #91's exercise: serves the built site (dist/) the way Pages does, under /goapplib/, loads
// every page in headless Chromium, and waits for each live demo to say it's ready (data-demo on the
// demo's <html>: "ready", or "error: <why>"). It also checks for what the mission still needs, so the
// run shows how far the site has got.
//
// Checks waiting on a ticket sit in `pending`: their failure is reported but doesn't fail the run,
// and their passing does (XPASS), so the PR that makes one pass has to take it off the list.
// `make exercise-docsite` builds and tests the site first.
import { createServer } from "node:http";
import { mkdirSync, readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const pending = {
  "demo:islands": "#93",
  "page:guide/getting-started": "#94",
  "page:guide/views": "#95",
  "page:guide/templates": "#96",
  "guide-off-github": "#97",
};

const here = (p) => fileURLToPath(new URL(p, import.meta.url));
const dist = here("./dist/");
const prefix = "/goapplib";

// Viewport shots only, of each demo scrolled into view.
const shots = here("./dist-screenshots/");
mkdirSync(shots, { recursive: true });

const types = {
  ".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css",
  ".svg": "image/svg+xml", ".png": "image/png", ".json": "application/json", ".wasm": "application/wasm",
};
const server = createServer((req, res) => {
  const path = decodeURIComponent(new URL(req.url, "http://x").pathname);
  if (!path.startsWith(prefix + "/")) return void res.writeHead(404).end();
  let file = join(dist, path.slice(prefix.length));
  if (!file.startsWith(dist)) return void res.writeHead(403).end();
  try {
    if (statSync(file).isDirectory()) file = join(file, "index.html");
    res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream" }).end(readFileSync(file));
  } catch {
    res.writeHead(404).end();
  }
});
await new Promise((ok) => server.listen(0, "127.0.0.1", ok));
const origin = `http://127.0.0.1:${server.address().port}`;

// Every built page, as a URL path: dist/guide/index.html -> /goapplib/guide/.
const pages = [];
(function walk(dir) {
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, e.name);
    if (e.isDirectory() && !p.startsWith(join(dist, "static"))) walk(p);
    else if (e.name === "index.html") pages.push(`${prefix}/${relative(dist, dir)}${dir === dist.slice(0, -1) ? "" : "/"}`.replace(/\/+$/, "/"));
  }
})(dist.slice(0, -1));
pages.sort();

const results = [];
const check = (name, ok, detail) => results.push({ name, ok, detail });

const browser = await chromium.launch();
const demos = new Set();
let linksGitHubGuides = [];
try {
  const page = await browser.newPage({ viewport: { width: 1200, height: 800 } });
  for (const path of pages) {
    const errors = [];
    const onError = (e) => errors.push(String(e));
    page.on("pageerror", onError);
    const res = await page.goto(origin + path);
    if (await page.locator('a[href*="/USAGE_GUIDE.md"], a[href*="/INTEGRATION_GUIDE.md"]').count()) linksGitHubGuides.push(path);
    for (const fig of await page.locator("figure.demo").all()) {
      const name = await fig.getAttribute("data-demo");
      demos.add(name);
      const frame = await (await fig.locator("iframe").elementHandle()).contentFrame();
      let state;
      try {
        state = await frame.waitForFunction(() => document.documentElement.dataset.demo, null, { timeout: 30_000 }).then((h) => h.jsonValue());
      } catch {
        state = "never said it was ready (30 s)";
      }
      await fig.scrollIntoViewIfNeeded();
      await page.screenshot({ path: `${shots}${path.slice(prefix.length + 1).replaceAll("/", "_") || "index_"}${name}.png` });
      check(`demo:${name}`, state === "ready", `on ${path}: ${state}`);
    }
    page.off("pageerror", onError);
    check(`page:${path.slice(prefix.length + 1, -1) || "index"}`, res.ok() && errors.length === 0, `HTTP ${res.status()}${errors.length ? `, errors: ${errors.join("; ")}` : ""}`);
  }
} finally {
  await browser.close();
  server.close();
}

// What the mission still needs, which a later ticket adds.
for (const name of Object.keys(pending)) {
  if (name.startsWith("demo:") && !results.some((r) => r.name.startsWith(name))) check(name, false, "no page embeds it yet");
  if (name.startsWith("page:") && !results.some((r) => r.name === name)) check(name, false, "no such page yet");
}
check("guide-off-github", linksGitHubGuides.length === 0, linksGitHubGuides.length ? `still linking USAGE_GUIDE.md or INTEGRATION_GUIDE.md: ${linksGitHubGuides.join(", ")}` : "no page sends a reader to the old guides");
check("demos", demos.size > 0, `${demos.size} demo(s): ${[...demos].join(", ")}`);

// A demo check named for a pending prefix ("demo:wasmhost-cache") counts as that pending check.
const pendingFor = (name) => pending[name] ?? Object.entries(pending).find(([k]) => k.startsWith("demo:") && name.startsWith(k + "-"))?.[1];
let failed = 0;
for (const r of results) {
  const ticket = pendingFor(r.name);
  let mark;
  if (ticket && r.ok) { mark = "XPASS"; failed++; }
  else if (ticket) mark = `pending ${ticket}`;
  else if (r.ok) mark = "ok";
  else { mark = "FAIL"; failed++; }
  console.log(`${mark.padEnd(12)} ${r.name}: ${r.detail}`);
}
console.log(`\n${results.filter((r) => r.ok).length}/${results.length} checks pass; screenshots in ${relative(process.cwd(), shots)}/`);
if (failed) {
  console.log(`${failed} check(s) failed or passed while still pending (XPASS: take it off \`pending\` in run.mjs)`);
  process.exit(1);
}
