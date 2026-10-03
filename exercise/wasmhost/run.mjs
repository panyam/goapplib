// Drives the exercise page in headless Chromium and exits non-zero unless every step passed, no
// FilesService request reached the network, and the console never mentioned a deadlock.
// `make exercise-wasmhost` builds dist/ first. Pass --serve to leave the server up for a browser.
import { createReadStream, existsSync } from "node:fs";
import { createServer } from "node:http";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const dist = fileURLToPath(new URL("./dist/", import.meta.url));
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
  process.exitCode = await drive(url);
  server.close();
}

async function drive(url) {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage();
    const consoleLines = [];
    const networkRPCs = [];
    page.on("console", (m) => consoleLines.push(`${m.type()}: ${m.text()}`));
    page.on("pageerror", (e) => consoleLines.push(`pageerror: ${e.message}`));
    page.on("worker", (w) => w.on("console", (m) => consoleLines.push(`worker ${m.type()}: ${m.text()}`)));
    page.context().on("request", (r) => {
      if (r.url().includes("/files.v1.FilesService/")) networkRPCs.push(r.url());
    });

    await page.goto(url);
    await page.waitForFunction(() => window.exercise?.done, null, { timeout: 60_000 });
    const { steps } = await page.evaluate(() => window.exercise);

    for (const s of steps) console.log(`${s.ok ? "PASS" : "FAIL"} ${s.name}: ${s.detail}`);
    const deadlock = consoleLines.filter((l) => /deadlock/i.test(l));
    const checks = [
      ["network", networkRPCs.length === 0, `${networkRPCs.length} FilesService requests reached the network`],
      ["console", deadlock.length === 0, deadlock.length ? deadlock.join(" | ") : "no deadlock in the console"],
    ];
    for (const [name, ok, detail] of checks) console.log(`${ok ? "PASS" : "FAIL"} ${name}: ${detail}`);

    const want = ["start", "mount", "read", "responsive"];
    const passed = want.every((n) => steps.some((s) => s.name === n && s.ok)) && checks.every(([, ok]) => ok);
    if (!passed && consoleLines.length) console.log(`console:\n  ${consoleLines.join("\n  ")}`);
    console.log(passed ? "exercise-wasmhost: PASS" : "exercise-wasmhost: FAIL");
    return passed ? 0 : 1;
  } finally {
    await browser.close();
  }
}
