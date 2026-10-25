// Drives the tour's browser spec (issue 128) in headless Chromium, over the fixture app in web/.
// jsdom has no layout, so what needs one lives here: a click in a hole reaches its target and one
// outside reaches nothing, wheel input is blocked outside the holes, a hole follows a target that
// moves, a step over several rows cuts a hole for each, and the panel stays clear of the holes at
// 1200 and 400 px.
//
// Checks still waiting on a ticket are listed in `pending`: their failure is reported but doesn't
// fail the run, and their passing does (XPASS), as in the other exercises. `make exercise-tour`
// bundles dist/ first.
import { mkdirSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { chromium } from "playwright-core";

const pending = {};

const here = (p) => fileURLToPath(new URL(p, import.meta.url));
const url = pathToFileURL(here("./dist/index.html")).href;
const shots = here("./dist/screenshots/");
mkdirSync(shots, { recursive: true });

const results = [];
const check = (name, ok, detail) => results.push({ name, ok, detail });

// What's under a point, from the page's own hit testing: "dim" for the overlay's layer, else the
// id of the nearest element that has one.
const hit = (page, x, y) =>
  page.evaluate(([x, y]) => {
    const el = document.elementFromPoint(x, y);
    if (!el) return null;
    if (el.closest(".tsappkit-tour-dim")) return "dim";
    if (el.closest(".tsappkit-tour-panel")) return "panel";
    const row = el.closest("#parts tr");
    if (row) return `row${[...row.parentElement.children].indexOf(row)}`;
    return el.closest("[id]")?.id ?? el.tagName;
  }, [x, y]);
const center = (page, sel) =>
  page.locator(sel).first().evaluate((el) => {
    const r = el.getBoundingClientRect();
    return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
  });
const fixture = (page, expr) => page.evaluate(`window.fixture.${expr}`);
const frames = (page, n = 3) =>
  page.evaluate((n) => new Promise((ok) => {
    const step = () => (n-- > 0 ? requestAnimationFrame(step) : ok());
    step();
  }), n);

// The panel against each hole's ring: none may overlap, and the panel stays in the viewport.
const panelClear = (page) =>
  page.evaluate(() => {
    const p = document.querySelector(".tsappkit-tour-panel").getBoundingClientRect();
    const vw = document.documentElement.clientWidth, vh = document.documentElement.clientHeight;
    const rings = [...document.querySelectorAll(".tsappkit-tour-ring")].map((r) => r.getBoundingClientRect());
    const hits = rings.filter((r) => p.x < r.right && r.x < p.right && p.y < r.bottom && r.y < p.bottom).length;
    const inside = p.x >= 0 && p.y >= 0 && p.right <= vw && p.bottom <= vh;
    return { ok: hits === 0 && inside && rings.length > 0, detail: `${rings.length} hole(s), ${hits} under the panel, in view: ${inside}` };
  });

async function wide(browser) {
  const page = await browser.newPage({ viewport: { width: 1200, height: 800 } });
  await page.goto(url);
  await fixture(page, "start()");
  await frames(page);
  await page.screenshot({ path: shots + "wide-1-run.png" });

  const other = await center(page, "#other");
  await page.mouse.click(other.x, other.y);
  check("outside-reaches-nothing", (await hit(page, other.x, other.y)) === "dim" && (await fixture(page, "app.otherClicks")) === 0,
    `under Settings: ${await hit(page, other.x, other.y)}, its clicks: ${await fixture(page, "app.otherClicks")}`);

  const log = await center(page, "#log");
  await page.mouse.move(log.x, log.y);
  await page.mouse.wheel(0, 400);
  await frames(page);
  const scrolled = await page.evaluate(() => ({ log: document.querySelector("#log").scrollTop, page: window.scrollY }));
  check("wheel-outside-blocked", scrolled.log === 0 && scrolled.page === 0, `findings scrollTop ${scrolled.log}, page scrollY ${scrolled.page}`);
  const c1 = await panelClear(page);
  check("panel-clear-wide-run", c1.ok, c1.detail);

  const run = await center(page, "#run");
  await page.mouse.click(run.x, run.y);
  check("hole-click-reaches-target", (await fixture(page, "app.checksRun")) === 1 && (await fixture(page, "step()")) === "rows",
    `checks run ${await fixture(page, "app.checksRun")}x, now on ${await fixture(page, "step()")}`);
  await frames(page);
  await page.screenshot({ path: shots + "wide-2-rows.png" });

  const rowHits = [];
  for (let i = 0; i < 4; i++) {
    const c = await center(page, `#parts tr:nth-child(${i + 1})`);
    rowHits.push(await hit(page, c.x, c.y));
  }
  const holes = await page.evaluate(() => document.querySelector(".tsappkit-tour").dataset.tourHoles);
  check("several-regions", rowHits.join() === "row0,row1,row2,row3" && (await hit(page, other.x, other.y)) === "dim",
    `under each row: ${rowHits.join(", ")}; rows padded into ${holes} hole(s)`);
  const c2 = await panelClear(page);
  check("panel-clear-wide-rows", c2.ok, c2.detail);

  const first = await center(page, "#parts tr:nth-child(1)");
  await page.mouse.click(first.x, first.y);
  const tip = await center(page, "#tip");
  await page.mouse.move(tip.x, tip.y);
  check("dom-step-advances-on-hover", (await fixture(page, "step()")) === "mover", `after the hover, on ${await fixture(page, "step()")}`);

  await frames(page);
  const before = await center(page, "#mover");
  await fixture(page, "app.move()");
  await frames(page);
  const after = await center(page, "#mover");
  const [atOld, atNew] = [await hit(page, before.x, before.y), await hit(page, after.x, after.y)];
  check("hole-follows-target", atOld === "dim" && atNew === "mover", `old spot: ${atOld}, new spot: ${atNew}`);
  await page.screenshot({ path: shots + "wide-3-mover.png" });

  await page.locator(".tsappkit-tour-next").click({ timeout: 5000 });
  await page.mouse.move(log.x, log.y);
  await page.mouse.wheel(0, 200);
  await page.waitForFunction(() => document.querySelector("#log").scrollTop > 0, null, { timeout: 2000 }).catch(() => {});
  const inHole = await page.evaluate(() => document.querySelector("#log").scrollTop);
  check("wheel-in-hole-scrolls", inHole > 0, `findings scrollTop ${inHole}`);
  await page.screenshot({ path: shots + "wide-4-log.png" });

  await page.keyboard.press("Escape");
  const ended = await fixture(page, "log.at(-1)");
  check("esc-cancels", ended.end === "cancelled" && (await page.locator(".tsappkit-tour").count()) === 0, JSON.stringify(ended));

  // Off path: the step wants the first row and the reader clicks the third. The tour goes and the
  // app does what it does with that click.
  await fixture(page, "start(1)");
  await frames(page);
  const third = await center(page, "#parts tr:nth-child(3)");
  await page.mouse.click(third.x, third.y);
  const off = await fixture(page, "log.at(-1)");
  const selected = await fixture(page, "app.selected");
  check("off-path-ends", off.end === "offPath" && off.at === "rows" && selected === 2 && (await page.locator(".tsappkit-tour").count()) === 0,
    `${JSON.stringify(off)}, app selected row ${selected}`);

  await fixture(page, "start()");
  await page.locator(".tsappkit-tour-showme").click({ timeout: 5000 });
  check("show-me-acts-through-app", (await fixture(page, "app.checksRun")) === 2 && (await fixture(page, "step()")) === "rows",
    `checks run ${await fixture(page, "app.checksRun")}x, now on ${await fixture(page, "step()")}`);
  await page.close();
}

async function narrow(browser) {
  const page = await browser.newPage({ viewport: { width: 400, height: 800 } });
  await page.goto(url);
  await fixture(page, "start()");
  await frames(page);
  const c1 = await panelClear(page);
  check("panel-clear-narrow-run", c1.ok, c1.detail);
  await page.screenshot({ path: shots + "narrow-1-run.png" });
  await fixture(page, "start(1)");
  await frames(page);
  const c2 = await panelClear(page);
  check("panel-clear-narrow-rows", c2.ok, c2.detail);
  await page.screenshot({ path: shots + "narrow-2-rows.png" });
  await page.close();
}

const browser = await chromium.launch();
try {
  // A phase that throws (a step that never came, so its button never showed) is a failed check
  // rather than a crash, so the checks before it still print.
  for (const [name, phase] of [["wide", wide], ["narrow", narrow]]) {
    await phase(browser).catch((e) => check(`${name}-ran`, false, String(e.message).split("\n")[0]));
  }
} finally {
  await browser.close();
}

let failed = 0;
for (const r of results) {
  const ticket = pending[r.name];
  let mark = r.ok ? "ok  " : "FAIL";
  if (ticket && r.ok) mark = "XPASS";
  else if (ticket) mark = `pending ${ticket}`;
  if (mark === "FAIL" || mark === "XPASS") failed++;
  console.log(`${mark}  ${r.name}: ${r.detail}`);
}
console.log(`screenshots in ${shots}`);
if (failed) {
  console.log(`${failed} check(s) failed or passed while still pending (XPASS: take it off \`pending\` in run.mjs)`);
  process.exit(1);
}
