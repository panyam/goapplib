// #139: do tsappkit-dock panels survive a dockview popout window? Each panel kind goes out into its
// own window, gets used there, and comes back. The table answers the ticket's questions in whatever
// browser runs it. Under automation it runs on load and calls ready() once every row has an answer.
import "dockview/dist/styles/dockview.css";
import "../_dock/style.css";
import { themeDark, themeLight } from "dockview-core";
import { delegateEvents } from "solid-js/web";
import { createDock, saveLayout, type Dock, type DockStorage, type PanelDef } from "../../../tsappkit-dock/src";
import { failed, ready } from "../_lib/frame";
import { counts, host, hostFor, release, results, setSolidCount, sleep, solidCount } from "../_dock/fixtures";

const popoutUrl = new URL("popout.html", location.href).pathname;
const panels: PanelDef[] = [
  { id: "canvas", title: "Canvas" },
  { id: "solid", title: "Solid", position: { direction: "right", referencePanel: "canvas" } },
  { id: "notes", title: "Notes", position: { direction: "right", referencePanel: "solid" } },
];
const layout = panels.map(({ id, position }) => ({ id, position }));
const answer = results(document.getElementById("results") as HTMLTableElement);
const container = document.getElementById("dock")!;

// Every window the page opens, so the reload check can count what dockview reopened.
const opened: (Window | null)[] = [];
const realOpen = window.open.bind(window);
window.open = ((...args: Parameters<typeof window.open>) => {
  const w = realOpen(...args);
  opened.push(w);
  return w;
}) as typeof window.open;

let shortcutHits = 0;
document.addEventListener("keydown", (e) => {
  if (e.key === "k") shortcutHits++;
});

const make = (storage: DockStorage | null = null): Dock =>
  createDock({ container, panels, defaultLayout: layout, hostFor, release, storage, key: "dock-popout-lab", theme: () => "dark" });
let dock = make();
(window as unknown as { lab: unknown }).lab = { get dock() { return dock; }, host, windowOf: (id: string) => windowOf(id) };

const windowOf = (id: string): Window | null => {
  const loc = dock.api.getPanel(id)?.group.api.location;
  return loc?.type === "popout" ? loc.getWindow() : null;
};

async function popOut(id: string): Promise<Window | null> {
  const ok = await dock.api.addPopoutGroup(dock.api.getPanel(id)!, { popoutUrl });
  answer(`${id}-opened`, `${id}: addPopoutGroup resolved true`, ok);
  for (let i = 0; i < 40 && !windowOf(id); i++) await sleep(50);
  const win = windowOf(id);
  for (let i = 0; i < 40 && win && host(id)!.ownerDocument !== win.document; i++) await sleep(50);
  return win;
}

async function run(): Promise<void> {
  const before = (id: string) => ({ ...counts.get(id)! });

  // ---- Solid ----
  const solid0 = before("solid");
  const solidWin = await popOut("solid");
  answer("solid-moved", "Solid: its element moved into the popout's document", !!solidWin && host("solid")!.ownerDocument === solidWin.document);
  answer("solid-hooks", "Solid: hostFor / release calls for the pop out", `${counts.get("solid")!.hostFor - solid0.hostFor} / ${counts.get("solid")!.release - solid0.release}`);
  const n0 = solidCount();
  host("solid")!.querySelector<HTMLButtonElement>(".count")!.click();
  answer("solid-click", "Solid: a click in the popout reaches its handler", solidCount() > n0);
  if (solidWin) delegateEvents(["click"], solidWin.document);
  const n1 = solidCount();
  host("solid")!.querySelector<HTMLButtonElement>(".count")!.click();
  answer("solid-click-delegated", "Solid: … after delegateEvents(['click'], popout.document)", solidCount() > n1);

  // ---- Canvas: pointer, resize ----
  const canvasWin = await popOut("canvas");
  const c0 = before("canvas");
  const canvas = host("canvas")!.querySelector("canvas")!;
  canvas.dispatchEvent(new (canvasWin as unknown as typeof globalThis).PointerEvent("pointerdown", { bubbles: true }));
  answer("canvas-pointer", "Canvas: its own pointerdown listener fires in the popout", counts.get("canvas")!.pointer > c0.pointer);
  const cv = host("canvas")!.querySelector("canvas")!;
  answer("canvas-ctx-out", "Canvas: it can still draw in the popout", drawable(cv));
  canvasWin?.resizeTo(520, 380);
  await sleep(400);
  answer("canvas-resize", "Canvas: a ResizeObserver from the page sees the popout resize", counts.get("canvas")!.resize > c0.resize);

  // ---- Keyboard ----
  const notesWin = await popOut("notes");
  const k0 = shortcutHits;
  notesWin?.document.body.dispatchEvent(new (notesWin as unknown as typeof globalThis).KeyboardEvent("keydown", { key: "k", bubbles: true }));
  answer("keys", "Keys: a shortcut listening on the page's document hears a key in the popout", shortcutHits > k0);
  host("notes")!.querySelector<HTMLTextAreaElement>(".notes")!.value = "typed in the popout";

  // ---- Styles ----
  const dv = notesWin?.document.querySelector(".dv-groupview, .dv-tabs-and-actions-container");
  answer("styles", "Styles: dockview's stylesheet reached the popout", !!dv && notesWin!.getComputedStyle(dv).display !== "" && notesWin!.document.styleSheets.length > 0);
  const themeIn = (d?: Document) => d?.querySelector("[class*='dockview-theme']")?.className.match(/dockview-theme-\w+/)?.[0] ?? "none";
  const t0 = `${themeIn(document)} / ${themeIn(notesWin?.document)}`;
  dock.api.updateOptions({ theme: themeLight });
  await sleep(200);
  answer("theme", "Styles: a theme switch, page / popout (before → after)", `${t0} → ${themeIn(document)} / ${themeIn(notesWin?.document)}`);
  dock.api.updateOptions({ theme: themeDark });

  // ---- tsappkit-dock's view of it ----
  answer("where", "tsappkit-dock: where('notes') while it's popped out", JSON.stringify(dock.where("notes")));

  // ---- Reload: save, tear down, restore ----
  const saved = dock.api.toJSON() as { popoutGroups?: unknown[] };
  answer("saved", "Reload: popout groups in the saved layout", saved.popoutGroups?.length ?? 0);
  const storage = memory();
  saveLayout(storage, "dock-popout-lab", panels, saved, {});
  const opens0 = opened.length;
  dock.dispose();
  for (const w of opened) w?.close();
  await sleep(200);
  dock = make(storage);
  await sleep(1500);
  const reopened = opened.slice(opens0).filter((w) => w && !w.closed).length;
  answer("restore-windows", "Reload: windows the restore opened without a click", reopened);
  answer("restore-where", "Reload: where each panel came back", panels.map((p) => `${p.id}:${windowOf(p.id) ? "popout" : dock.where(p.id)?.kind ?? "closed"}`).join(" "));
  answer("restore-content", "Reload: Solid count and notes text survived", `${solidCount()} / ${JSON.stringify(host("notes")!.querySelector<HTMLTextAreaElement>(".notes")!.value)}`);

  // ---- Back into the grid ----
  // tsappkit-dock's where() reports a popout as grid. So toggle closes the panel, and show re-adds it
  // at its position, or with none to dockview's active group, either of which can be a popout.
  dock.toggle("notes");
  dock.show("notes");
  await sleep(300);
  answer("back-dock", "Back: tsappkit-dock toggle + show put notes in", windowOf("notes") ? "a popout" : dock.where("notes")?.kind ?? "nowhere");

  // Removing the panel and adding it with a position that names no panel puts it at the grid's edge,
  // whether or not anything is still in the grid. moveTo into a new grid group lost a panel after
  // a restore, when every group was a popout.
  for (const p of panels) {
    const panel = dock.api.getPanel(p.id);
    if (!panel || !windowOf(p.id)) continue;
    dock.api.removePanel(panel);
    dock.api.addPanel({ id: p.id, component: p.id, title: p.title, position: { direction: "right" } });
  }
  await sleep(300);
  answer(
    "back",
    "Back: after removePanel + addPanel at the grid's edge, each panel's place / document",
    panels.map((p) => `${p.id}:${windowOf(p.id) ? "popout" : dock.where(p.id)?.kind ?? "closed"}/${host(p.id)!.ownerDocument === document ? "page" : "popout"}`).join(" "),
  );
  // The canvas went through a popout that has since closed.
  const back = host("canvas")!.querySelector("canvas")!;
  let restored = false;
  back.addEventListener("contextrestored", () => (restored = true), { once: true });
  answer("canvas-ctx-back", "Canvas: back in the page, it can still draw", drawable(back));
  await sleep(2000);
  answer("canvas-ctx-restored", "Canvas: 2 s later, contextrestored fired / it can draw", `${restored} / ${drawable(back) ? "yes" : "no"}`);
  const fresh = document.createElement("canvas");
  back.replaceWith(fresh);
  fresh.width = fresh.height = 4;
  answer("canvas-new", "Canvas: a new <canvas> element in its place can draw", drawable(fresh));
  fresh.replaceWith(back);
  const n2 = solidCount();
  host("solid")!.querySelector<HTMLButtonElement>(".count")!.click();
  answer("back-click", "Back in the grid: Solid's click works again", solidCount() > n2);
  for (const w of opened) w?.close();
  setSolidCount(0);
}

// Whether a canvas's 2D context still draws: paint one pixel and read it back. isContextLost() would
// say the same, but WebKit's 2D context doesn't have it.
function drawable(cv: HTMLCanvasElement): boolean {
  const g = cv.getContext("2d");
  if (!g) return false;
  g.fillStyle = "#ff0000";
  g.fillRect(0, 0, 1, 1);
  return g.getImageData(0, 0, 1, 1).data[0] === 255;
}

function memory(): DockStorage {
  const m = new Map<string, string>();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => void m.set(k, v), removeItem: (k) => void m.delete(k) };
}

let running = false;
const start = () => {
  if (running) return Promise.resolve();
  running = true;
  return run().finally(() => (running = false));
};
document.getElementById("run")!.addEventListener("click", () => {
  start().catch((err: Error) => answer("error", "The run stopped", err.message));
});
if (navigator.webdriver) start().then(ready, (err: Error) => failed(err.message));
else ready();
