// #139: do tsappkit-dock panels survive a dockview popout window? Each panel kind goes out into its
// own window, gets used there, and comes back. The table answers the ticket's questions in whatever
// browser runs it. Under automation it runs on load and calls ready() once every row has an answer.
import "dockview/dist/styles/dockview.css";
import "../_dock/style.css";
import { themeDark, themeLight } from "dockview-core";
import { delegateEvents } from "solid-js/web";
import { createDock, saveLayout, type Dock, type DockStorage, type PanelDef } from "../../../tsappkit-dock/src";
import { failed, ready } from "../_lib/frame";
import { counts, host, hostFor, rebuildCanvas, release, results, setSolidCount, sleep, solidCount } from "../_dock/fixtures";

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

// What the dock's moved hook reported, per panel. A canvas whose document changed gets a new <canvas>,
// since one that has been in a closed popout can't draw.
const moves = new Map<string, string[]>();
const make = (storage: DockStorage | null = null): Dock =>
  createDock({
    container,
    panels,
    defaultLayout: layout,
    hostFor,
    release,
    storage,
    key: "dock-popout-lab",
    theme: () => "dark",
    moved: (id, _el, to, from, { documentChanged }) => {
      moves.set(id, [...(moves.get(id) ?? []), `${from?.kind ?? "closed"}→${to?.kind ?? "closed"}${documentChanged ? " (new document)" : ""}`]);
      if (documentChanged && id === "canvas") rebuildCanvas(id);
    },
  });
const movesOf = (id: string) => {
  const m = moves.get(id) ?? [];
  moves.delete(id);
  return m.join(", ") || "nothing";
};
let dock = make();

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

// A throwaway popout of the notes panel for the canvas-death check. Closing its window sends the
// panel home, and the run pops notes out again afterwards for the rows that need it.
async function popOutProbe(): Promise<Window | null> {
  if (!windowOf("notes")) await dock.api.addPopoutGroup(dock.api.getPanel("notes")!, { popoutUrl });
  for (let i = 0; i < 40 && !windowOf("notes"); i++) await sleep(50);
  const w = windowOf("notes");
  for (let i = 0; i < 40 && w && host("notes")!.ownerDocument !== w.document; i++) await sleep(50);
  return w;
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
  moves.clear();
  const canvasWin = await popOut("canvas");
  await sleep(100);
  answer("moved-out", "moved: what the dock reported for the canvas popping out", movesOf("canvas"));
  const c0 = before("canvas");
  const canvas = host("canvas")!.querySelector("canvas")!;
  canvas.dispatchEvent(new (canvasWin as unknown as typeof globalThis).PointerEvent("pointerdown", { bubbles: true }));
  answer("canvas-pointer", "Canvas: its own pointerdown listener fires in the popout", counts.get("canvas")!.pointer > c0.pointer);
  const cv = host("canvas")!.querySelector("canvas")!;
  answer("canvas-ctx-out", "Canvas: it can still draw in the popout", drawable(cv));
  const size = () => {
    const r = host("canvas")!.getBoundingClientRect();
    return `${Math.round(r.width)}×${Math.round(r.height)}`;
  };
  const s0 = size();
  const r0 = counts.get("canvas")!.resize;
  canvasWin?.resizeTo(520, 380);
  await sleep(400);
  answer("canvas-resize", "Canvas: resizeTo on the popout, panel size before → after / ResizeObserver calls", `${s0} → ${size()} / ${counts.get("canvas")!.resize - r0}`);

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

  // ---- When does a canvas die? ----
  // Two probe canvases made in the notes popout, each drawn there. One is taken out before the window
  // closes, and one is still inside when it does.
  const probeWin = await popOutProbe();
  if (probeWin) {
    const taken = probeWin.document.createElement("canvas");
    const left = probeWin.document.createElement("canvas");
    host("notes")!.append(taken, left);
    const outFirst = drawable(taken) && drawable(left);
    document.body.appendChild(taken);
    probeWin.close();
    for (let i = 0; i < 40 && !probeWin.closed; i++) await sleep(50);
    await sleep(300);
    document.body.appendChild(left);
    answer("die-out", "Canvas made in a popout and taken out before it closes: draws afterwards", outFirst && drawable(taken));
    answer("die-in", "Canvas still inside the popout when it closes: draws afterwards", drawable(left));
    taken.remove();
    left.remove();
  }

  if (!windowOf("notes")) await popOut("notes");

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
  // The canvas came back from the restore in a popout, so the <canvas> in it now was made there.
  const old = host("canvas")!.querySelector("canvas")!;
  let restored = false;
  old.addEventListener("contextrestored", () => (restored = true), { once: true });
  moves.clear();
  for (const p of panels) if (dock.where(p.id)?.kind === "popout") dock.place(p.id, { kind: "grid" });
  await sleep(300);
  answer(
    "back",
    "Back: after dock.place(id, grid), each panel's place / document",
    panels.map((p) => `${p.id}:${dock.where(p.id)?.kind ?? "closed"}/${host(p.id)!.ownerDocument === document ? "page" : "popout"}`).join(" "),
  );
  answer(
    "back-size",
    "Back: each panel's group size in the grid",
    panels.map((p) => {
      const r = dock.api.getPanel(p.id)?.group.element.getBoundingClientRect();
      return `${p.id}:${r ? `${Math.round(r.width)}×${Math.round(r.height)}` : "none"}`;
    }).join(" "),
  );
  answer("moved-back", "moved: what the dock reported for the canvas coming home", movesOf("canvas"));
  answer("canvas-old", "Canvas: the <canvas> made in the popout can draw at home", drawable(old));
  const now = host("canvas")!.querySelector("canvas")!;
  answer("canvas-rebuilt", "Canvas: the <canvas> moved rebuilt can draw at home", now !== old && drawable(now));
  await sleep(2000);
  answer("canvas-ctx-restored", "Canvas: 2 s later, contextrestored fired on the old one", restored);
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
// What tsappkit-dock promises about popouts (#142). The other rows describe the browser and dockview,
// which the dock doesn't control, so they're reported, not checked.
const promised: [string, (answer: string) => boolean][] = [
  ["moved-out", (a) => a.includes("grid→popout (new document)")],
  ["moved-back", (a) => a.includes("popout→grid (new document)")],
  ["canvas-rebuilt", (a) => a === "yes"],
  ["where", (a) => a === '{"kind":"popout"}'],
  ["back", (a) => a === "canvas:grid/page solid:grid/page notes:grid/page"],
  ["back-size", (a) => !/:0×|:\d+×0|none/.test(a)],
];
function check(): void {
  for (const [key, ok] of promised) {
    const a = document.querySelector<HTMLTableRowElement>(`#results tr[data-key="${key}"]`)?.dataset.answer ?? "(no answer)";
    if (!ok(a)) throw new Error(`${key}: ${a}`);
  }
}
if (navigator.webdriver) start().then(check).then(ready, (err: Error) => failed(err.message));
else ready();
