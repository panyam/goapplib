// #138: should a drawer have a pinned mode that pushes the grid aside instead of covering it? This
// prototypes pinning from outside tsappkit-dock, with no change to createDock. The dock's container
// sits in a stage, the drawers mount on the stage (drawerHost), and a pinned drawer hides its scrim
// and insets the container by its own size. The table answers the ticket's questions in whatever
// browser runs it. Under automation it runs on load and calls ready() once every row has an answer.
import "dockview/dist/styles/dockview.css";
import "../_dock/style.css";
import { createDock, type DrawerEdge, type PanelDef } from "../../../tsappkit-dock/src";
import { failed, ready } from "../_lib/frame";
import { counts, frame, host, hostFor, release, results, sleep } from "../_dock/fixtures";

const EDGES: DrawerEdge[] = ["left", "right", "top", "bottom"];
const stage = document.getElementById("stage")!;
const container = document.getElementById("dock")!;
const corner = document.getElementById("corner") as HTMLSelectElement;
const answer = results(document.getElementById("results") as HTMLTableElement);

const panels: PanelDef[] = [
  { id: "canvas", title: "Canvas" },
  { id: "checks", title: "Checks", position: { direction: "right", referencePanel: "canvas" }, defaultPlacement: { kind: "drawer", edge: "right" } },
  { id: "notes", title: "Notes", defaultPlacement: { kind: "drawer", edge: "bottom" } },
  { id: "solid", title: "Solid", defaultPlacement: { kind: "drawer", edge: "left" } },
];
const dock = createDock({
  container,
  drawerHost: stage,
  panels,
  defaultLayout: [{ id: "canvas" }],
  hostFor,
  release,
  theme: () => "dark",
  drawers: { right: { size: 320, min: 200 }, left: { size: 260, min: 200 }, bottom: { size: 160, min: 100 } },
});

// ---- the pinning prototype ----

const pinned = new Set<DrawerEdge>();
const drawerEl = (edge: DrawerEdge) => stage.querySelector<HTMLElement>(`.dock-drawer-${edge}`);
const scrimEl = (edge: DrawerEdge) => stage.querySelector<HTMLElement>(`.dock-drawer-scrim-${edge}`);
const isOpen = (edge: DrawerEdge) => drawerEl(edge)?.style.display === "flex";
const set = (el: HTMLElement, prop: "left" | "right" | "top" | "bottom", v: string) => {
  if (el.style[prop] !== v) el.style[prop] = v;
};

function apply(): void {
  const inset = { left: 0, right: 0, top: 0, bottom: 0 };
  for (const edge of EDGES) {
    const root = drawerEl(edge);
    const scrim = scrimEl(edge);
    if (!root || !scrim) continue;
    const pin = pinned.has(edge) && isOpen(edge);
    const want = isOpen(edge) && !pin ? "block" : "none";
    if (scrim.style.display !== want) scrim.style.display = want;
    if (pin) inset[edge] = edge === "left" || edge === "right" ? root.offsetWidth : root.offsetHeight;
  }
  // Two pinned drawers on adjacent edges: one of them runs the full length and the other stops at it.
  const sides = corner.value === "sides";
  for (const edge of EDGES) {
    const root = drawerEl(edge);
    if (!root) continue;
    if (edge === "top" || edge === "bottom") {
      set(root, "left", sides ? `${inset.left}px` : "0px");
      set(root, "right", sides ? `${inset.right}px` : "0px");
    } else {
      set(root, "top", sides ? "0px" : `${inset.top}px`);
      set(root, "bottom", sides ? "0px" : `${inset.bottom}px`);
    }
  }
  for (const edge of EDGES) set(container, edge, `${inset[edge]}px`);
}

function addPinButtons(): void {
  for (const edge of EDGES) {
    const tabs = drawerEl(edge)?.querySelector(".dock-drawer-tabs");
    if (!tabs || tabs.querySelector(".pin-btn")) continue;
    const b = document.createElement("button");
    b.type = "button";
    b.className = "pin-btn";
    b.textContent = "Pin";
    b.setAttribute("aria-pressed", "false");
    b.addEventListener("click", () => pin(edge, !pinned.has(edge)));
    tabs.appendChild(b);
  }
}

function pin(edge: DrawerEdge, on: boolean): void {
  if (on) pinned.add(edge);
  else pinned.delete(edge);
  const b = drawerEl(edge)?.querySelector(".pin-btn");
  if (b) {
    b.textContent = on ? "Unpin" : "Pin";
    b.setAttribute("aria-pressed", String(on));
  }
  apply();
}

new MutationObserver((records) => {
  if (records.some((r) => r.target !== container)) {
    addPinButtons();
    apply();
  }
}).observe(stage, { subtree: true, childList: true, attributes: true, attributeFilter: ["style"] });
corner.addEventListener("change", apply);

// Escape and a click outside close an overlay drawer, not a pinned one.
document.addEventListener(
  "keydown",
  (e) => {
    if (e.key !== "Escape" || ![...pinned].some(isOpen)) return;
    e.stopPropagation();
    for (const edge of EDGES) if (!pinned.has(edge) && isOpen(edge)) dock.toggleDrawer(edge, false);
  },
  true,
);

for (const b of document.querySelectorAll<HTMLButtonElement>(".bar button[data-edge]")) {
  b.addEventListener("click", () => dock.toggleDrawer(b.dataset.edge as DrawerEdge));
}
addPinButtons();
apply();

// ---- the experiments ----

const rect = (el: Element) => el.getBoundingClientRect();
const fmt = (r: DOMRect) => `${Math.round(r.width)}×${Math.round(r.height)} at ${Math.round(r.left - rect(stage).left)},${Math.round(r.top - rect(stage).top)}`;
const stats = (xs: number[]) => {
  const s = [...xs].sort((a, b) => a - b);
  const at = (q: number) => s[Math.min(s.length - 1, Math.floor(q * s.length))];
  return `median ${at(0.5).toFixed(1)} ms, p95 ${at(0.95).toFixed(1)} ms, max ${s[s.length - 1].toFixed(1)} ms`;
};

async function drag(edge: "right", from: number, to: number, steps: number): Promise<{ frames: number[]; layouts: number; resizes: number }> {
  const handle = drawerEl(edge)!.querySelector<HTMLElement>(".dock-drawer-handle")!;
  let layouts = 0;
  const sub = dock.api.onDidLayoutChange(() => layouts++);
  const r0 = counts.get("canvas")!.resize;
  handle.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
  const frames: number[] = [];
  let last = await frame();
  for (let i = 1; i <= steps; i++) {
    const size = from + ((to - from) * i) / steps;
    document.dispatchEvent(new PointerEvent("pointermove", { clientX: rect(stage).right - size, bubbles: true }));
    const now = await frame();
    frames.push(now - last);
    last = now;
  }
  document.dispatchEvent(new PointerEvent("pointerup", { bubbles: true }));
  await frame();
  sub.dispose();
  return { frames, layouts, resizes: counts.get("canvas")!.resize - r0 };
}

async function run(): Promise<void> {
  const canvas = () => rect(host("canvas")!);

  // E1: dragging a pinned drawer's edge, against the same drag on an overlay drawer.
  dock.show("checks");
  await sleep(100);
  const overlay = await drag("right", 320, 520, 60);
  answer("drag-overlay", "Drag 60 steps, overlay: frame time", stats(overlay.frames));
  answer("drag-overlay-layout", "Drag 60 steps, overlay: dockview layout events / canvas resizes", `${overlay.layouts} / ${overlay.resizes}`);
  dock.toggleDrawer("right", false);
  dock.show("checks");
  pin("right", true);
  await sleep(100);
  const pinnedDrag = await drag("right", 320, 520, 60);
  answer("drag-pinned", "Drag 60 steps, pinned: frame time", stats(pinnedDrag.frames));
  answer("drag-pinned-layout", "Drag 60 steps, pinned: dockview layout events / canvas resizes", `${pinnedDrag.layouts} / ${pinnedDrag.resizes}`);
  answer("drag-pinned-fit", "Drag, pinned: grid right edge meets the drawer's left edge", Math.abs(rect(container).right - rect(drawerEl("right")!).left) <= 1);

  // E2: a pinned drawer against the same panel docked in the grid at the same width.
  const w = rect(drawerEl("right")!).width;
  const pinnedCanvas = canvas();
  const c = rect(host("canvas")!);
  const hit = document.elementFromPoint(c.left + c.width / 2, c.top + c.height / 2);
  answer("pinned-click", "Pinned: the middle of the grid is the canvas, not a scrim", !!hit && host("canvas")!.contains(hit));
  host("canvas")!.querySelector("canvas")!.click();
  answer("pinned-stays", "Pinned: a click in the grid leaves the drawer open", isOpen("right"));
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  answer("pinned-escape", "Pinned: Escape leaves the drawer open", isOpen("right"));
  pin("right", false);
  dock.place("checks", { kind: "grid" });
  await sleep(100);
  dock.api.getPanel("checks")!.api.setSize({ width: Math.round(w) });
  await sleep(200);
  const gridCanvas = canvas();
  answer("vs-grid", "Pinned vs docked at the same width: canvas size", `${Math.round(pinnedCanvas.width)}×${Math.round(pinnedCanvas.height)} vs ${Math.round(gridCanvas.width)}×${Math.round(gridCanvas.height)}`);
  const header = dock.api.getPanel("checks")!.group.element.querySelector(".dv-tabs-and-actions-container");
  answer("vs-grid-chrome", "Docked: the panel gains a tab header (px)", header ? Math.round(rect(header).height) : 0);
  dock.place("checks", { kind: "drawer", edge: "right" });
  dock.toggleDrawer("right", false);

  // E3: two pinned drawers on adjacent edges, under each corner rule.
  dock.show("solid");
  dock.show("notes");
  pin("left", true);
  pin("bottom", true);
  for (const rule of ["sides", "ends"]) {
    corner.value = rule;
    apply();
    await sleep(100);
    const l = rect(drawerEl("left")!);
    const b = rect(drawerEl("bottom")!);
    const overlap = Math.max(0, Math.min(l.right, b.right) - Math.max(l.left, b.left)) * Math.max(0, Math.min(l.bottom, b.bottom) - Math.max(l.top, b.top));
    answer(`corner-${rule}`, `Corner, ${rule === "sides" ? "left takes it" : "bottom takes it"}: left / bottom / grid, overlap px²`, `${fmt(l)} / ${fmt(b)} / ${fmt(rect(container))}, ${Math.round(overlap)}`);
  }
  corner.value = "sides";
  pin("left", false);
  pin("bottom", false);
  dock.toggleDrawer("left", false);
  dock.toggleDrawer("bottom", false);
  apply();

  // E4: where the drawers live. Pinning shrinks the dock's container, so a drawer inside it would
  // shrink with it.
  answer("host", "Drawers mount outside the dock's container (drawerHost)", !container.contains(drawerEl("right")!));
}

let running = false;
document.getElementById("run")!.addEventListener("click", () => {
  if (running) return;
  running = true;
  run()
    .catch((err: Error) => answer("error", "The run stopped", err.message))
    .finally(() => (running = false));
});
if (navigator.webdriver) run().then(ready, (err: Error) => failed(err.message));
else ready();
