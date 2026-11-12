// #140: a phone page built as a dock. The canvas is the only panel in the grid, with its header
// hidden, and every other panel is a tab in a bottom drawer with no tab strip. The bar's buttons
// open a panel with dock.show and close it on a second tap, and drawerChanged lights the button for
// the panel on show. This is the setup that replaces tsappkit's MobileBottomDrawer. Under automation
// it taps and drags through the sheet, and fails if any answer is wrong.
import "dockview/dist/styles/dockview.css";
import "../_dock/style.css";
import { createDock, type PanelDef } from "../../../tsappkit-dock/src";
import { failed, ready } from "../_lib/frame";
import { frame, host, hostFor, release, results, sleep } from "../_dock/fixtures";

const container = document.getElementById("dock")!;
const bar = document.getElementById("bar")!;
const answer = results(document.getElementById("results") as HTMLTableElement);
const sheet = { kind: "drawer", edge: "bottom" } as const;

const panels: PanelDef[] = [
  { id: "canvas", title: "Map" },
  { id: "checks", title: "Checks", defaultPlacement: sheet },
  { id: "notes", title: "Notes", defaultPlacement: sheet },
  { id: "solid", title: "Solid", defaultPlacement: sheet },
];
const dock = createDock({
  container,
  panels,
  defaultLayout: (api) => {
    api.addPanel({ id: "canvas", component: "canvas", title: "Map" });
    api.groups[0].header.hidden = true;
  },
  hostFor,
  release,
  theme: () => "dark",
  moveMenu: false,
  drawers: { bottom: { size: 0.65, min: 160, tabs: false } },
  drawerChanged: (_edge, open, active) => {
    for (const b of bar.querySelectorAll<HTMLButtonElement>("button")) b.setAttribute("aria-pressed", String(open && b.dataset.panel === active));
  },
});

const drawerEl = () => container.querySelector<HTMLElement>(".dock-drawer-bottom")!;
const scrimEl = () => container.querySelector<HTMLElement>(".dock-drawer-scrim-bottom")!;
const button = (id: string) => bar.querySelector<HTMLButtonElement>(`[data-panel="${id}"]`)!;
const isOpen = () => drawerEl().style.display === "flex";
const visible = () => [...drawerEl().querySelectorAll<HTMLElement>(".dock-drawer-body > *")].find((el) => el.style.display !== "none");
const pressed = () => [...bar.querySelectorAll<HTMLButtonElement>('[aria-pressed="true"]')].map((b) => b.dataset.panel).join(",");

bar.addEventListener("click", (e) => {
  const id = (e.target as HTMLElement).closest<HTMLButtonElement>("button")?.dataset.panel;
  if (!id) return;
  const where = dock.where(id);
  const showing = isOpen() && where?.kind === "drawer" && visible() === host(id);
  if (showing) dock.toggleDrawer("bottom", false);
  else dock.show(id);
});

// A finger on the sheet's grab handle, moved to each y in turn, `gap` ms apart (0 is a frame), then
// lifted. A flick is a gap of a frame; a gap over 80 ms keeps 40 px steps under DRAWER_FLICK.
async function swipe(ys: number[], gap = 0): Promise<void> {
  const handle = drawerEl().querySelector<HTMLElement>(".dock-drawer-handle")!;
  const r = handle.getBoundingClientRect();
  const x = r.left + r.width / 2;
  const at = (type: string, y: number, target: EventTarget) =>
    target.dispatchEvent(new PointerEvent(type, { bubbles: true, cancelable: true, pointerType: "touch", isPrimary: true, clientX: x, clientY: y }));
  at("pointerdown", r.top + r.height / 2, handle);
  for (const y of ys) {
    await (gap ? sleep(gap) : frame());
    at("pointermove", y, document);
  }
  at("pointerup", ys[ys.length - 1], document);
  await frame();
}

async function run(): Promise<string[]> {
  const got: string[] = [];
  const check = (key: string, q: string, ok: boolean, detail?: string) => {
    if (!ok) got.push(detail ? `${key} (${detail})` : key);
    answer(key, q, detail ? `${ok ? "yes" : "no"} (${detail})` : ok);
  };
  const dockRect = () => container.getBoundingClientRect();
  const height = () => Math.round(drawerEl().getBoundingClientRect().height);

  // dockview places always-rendered content a frame after a layout.
  await frame();
  await frame();
  const map = host("canvas")!.getBoundingClientRect();
  check("map", "The map fills the dock, with no tab header", Math.abs(map.height - dockRect().height) <= 1, `${Math.round(map.height)} of ${Math.round(dockRect().height)} px`);

  button("checks").click();
  await sleep(50);
  const want = Math.round(dockRect().height * 0.65);
  check("open", "Checks opens the sheet at 65% of the dock, on Checks", isOpen() && Math.abs(height() - want) <= 1 && visible() === host("checks"), `${height()} px, want ${want}`);
  check("lit", "Its button lights up", pressed() === "checks", pressed() || "none");

  button("notes").click();
  await sleep(50);
  check("switch", "Notes switches the open sheet to Notes", isOpen() && visible() === host("notes") && pressed() === "notes");

  button("notes").click();
  await sleep(50);
  check("again", "Notes again closes it, and no button stays lit", !isOpen() && pressed() === "", pressed() || "none");

  button("solid").click();
  await sleep(50);
  scrimEl().click();
  await sleep(50);
  check("scrim", "A tap on the dimmed map closes it", !isOpen());

  button("checks").click();
  await sleep(50);
  const top = drawerEl().getBoundingClientRect().top;
  const bottom = dockRect().bottom;
  await swipe([top + 40, top + 80, top + 120], 150);
  check("resize", "A slow drag down a little resizes it and leaves it open", isOpen() && Math.abs(height() - (want - 120)) <= 2, `${height()} px`);

  const t2 = drawerEl().getBoundingClientRect().top;
  const steps = Array.from({ length: 12 }, (_, i) => t2 + ((bottom - 30 - t2) * (i + 1)) / 12);
  await swipe(steps, 150);
  check("drag-close", "A drag down to near the bottom closes it", !isOpen());

  button("checks").click();
  await sleep(50);
  check("kept", "It reopens at the size it had before that drag", Math.abs(height() - (want - 120)) <= 2, `${height()} px`);

  const t3 = drawerEl().getBoundingClientRect().top;
  await swipe([t3 + 5, t3 + 10, t3 + 120]);
  check("flick", "A quick flick down closes it from high up", !isOpen());

  // Leave the demo as a fresh load would, for the screenshot and for whoever opens it next.
  dock.reset();

  return got;
}

if (navigator.webdriver) {
  run().then(
    (wrong) => (wrong.length ? failed(`phone sheet checks failed: ${wrong.join(", ")}`) : ready()),
    (err: Error) => failed(err.message),
  );
} else {
  ready();
}
