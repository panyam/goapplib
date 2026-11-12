import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createDock, type Dock, type DockOptions } from "./dock.js";
import type { DockStorage, PanelDef, PanelSpec } from "./layout.js";

const KEY = "drawer-dock";
const REGISTRY: PanelDef[] = [
  { id: "files", title: "Files" },
  { id: "diff", title: "Diff" },
  { id: "log", title: "Log", defaultPlacement: { kind: "drawer", edge: "bottom" } },
];
const LAYOUT: PanelSpec[] = [{ id: "diff" }, { id: "files", position: { direction: "left", referencePanel: "diff" } }];

function fakeStorage(): DockStorage & { data: Record<string, string> } {
  const data: Record<string, string> = {};
  return {
    data,
    getItem: (k) => (k in data ? data[k] : null),
    setItem: (k, v) => {
      data[k] = v;
    },
    removeItem: (k) => {
      delete data[k];
    },
  };
}

let container: HTMLElement;
let hosts: Map<string, HTMLElement>;
let calls: string[];
const docks: Dock[] = [];

function make(over: Partial<DockOptions> = {}): Dock {
  const dock = createDock({
    container,
    panels: REGISTRY,
    defaultLayout: LAYOUT,
    resolve: (id) => (id.startsWith("page:") ? { id, title: id.slice(5) } : undefined),
    hostFor: (id) => {
      calls.push(`host ${id}`);
      let el = hosts.get(id);
      if (!el) {
        el = document.createElement("div");
        el.dataset.host = id;
        hosts.set(id, el);
      }
      return el;
    },
    release: (id, el) => {
      calls.push(`release ${id}`);
      expect(el).toBe(hosts.get(id));
    },
    theme: () => "light",
    ...over,
  });
  docks.push(dock);
  return dock;
}

const drawerEl = (edge: string) => container.querySelector<HTMLElement>(`.dock-drawer-${edge}`);
const scrimEl = (edge: string) => container.querySelector<HTMLElement>(`.dock-drawer-scrim-${edge}`);
const tabs = (edge: string) => [...(drawerEl(edge)?.querySelectorAll<HTMLButtonElement>(".dock-drawer-tab") ?? [])].map((b) => b.dataset.panel);
const settle = () => new Promise((r) => setTimeout(r, 0));

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  hosts = new Map();
  calls = [];
});

afterEach(() => {
  for (const d of docks.splice(0)) d.dispose();
  container.remove();
  document.querySelectorAll(".dock-menu").forEach((m) => m.remove());
});

describe("defaultPlacement", () => {
  it("starts a panel in its drawer, closed, and leaves the grid to the specs", () => {
    const dock = make();
    expect(dock.where("log")).toEqual({ kind: "drawer", edge: "bottom" });
    expect(dock.where("diff")).toEqual({ kind: "grid" });
    expect(dock.api.getPanel("log")).toBeUndefined();
    expect(tabs("bottom")).toEqual(["log"]);
    expect(drawerEl("bottom")!.style.display).toBe("none");
  });

  it("starts a float panel floating", () => {
    const dock = make({ panels: [...REGISTRY, { id: "ask", title: "Ask", defaultPlacement: { kind: "float" } }] });
    expect(dock.where("ask")).toEqual({ kind: "float" });
  });
});

describe("place", () => {
  it("moves a grid panel into a drawer through release and hostFor, keeping its element", () => {
    const dock = make();
    const el = hosts.get("files")!;
    calls = [];
    expect(dock.place("files", { kind: "drawer", edge: "left" })).toBe(true);
    expect(calls).toEqual(["release files", "host files"]);
    expect(dock.where("files")).toEqual({ kind: "drawer", edge: "left" });
    expect(dock.api.getPanel("files")).toBeUndefined();
    expect(drawerEl("left")!.contains(el)).toBe(true);
    expect(drawerEl("left")!.style.display).toBe("flex");
  });

  it("moves a drawer panel back to the grid at its position", () => {
    const dock = make();
    calls = [];
    dock.place("log", { kind: "grid" });
    expect(calls).toEqual(["release log", "host log"]);
    expect(dock.where("log")).toEqual({ kind: "grid" });
    expect(dock.api.getPanel("log")!.view.content.element).toBe(hosts.get("log"));
    expect(tabs("bottom")).toEqual([]);
  });

  it("floats a grid panel without releasing it, and docks it back", () => {
    const dock = make();
    calls = [];
    dock.place("files", { kind: "float" });
    expect(dock.where("files")).toEqual({ kind: "float" });
    expect(calls).toEqual([]);
    dock.place("files", { kind: "grid" });
    expect(dock.where("files")).toEqual({ kind: "grid" });
    expect(dock.api.getPanel("files")!.view.content.element).toBe(hosts.get("files"));
  });

  it("moves a panel from one drawer to another", () => {
    const dock = make();
    dock.place("log", { kind: "drawer", edge: "top" });
    expect(tabs("bottom")).toEqual([]);
    expect(tabs("top")).toEqual(["log"]);
    expect(drawerEl("top")!.contains(hosts.get("log")!)).toBe(true);
  });

  it("opens a closed panel where it's told, and refuses an unknown id", () => {
    const dock = make();
    dock.toggle("files");
    expect(dock.where("files")).toBeNull();
    dock.place("files", { kind: "drawer", edge: "right" });
    expect(dock.where("files")).toEqual({ kind: "drawer", edge: "right" });
    expect(dock.place("nope", { kind: "grid" })).toBe(false);
  });
});

describe("drawers", () => {
  it("shows tabs for several panels and switches between them", () => {
    const dock = make();
    dock.place("files", { kind: "drawer", edge: "bottom" });
    expect(tabs("bottom")).toEqual(["log", "files"]);
    expect(hosts.get("files")!.style.display).toBe("");
    expect(hosts.get("log")!.style.display).toBe("none");
    drawerEl("bottom")!.querySelector<HTMLButtonElement>('[data-panel="log"]')!.click();
    expect(hosts.get("log")!.style.display).toBe("");
    expect(hosts.get("files")!.style.display).toBe("none");
  });

  it("show opens a panel's drawer and brings its tab to the front", () => {
    const dock = make();
    dock.place("files", { kind: "drawer", edge: "bottom" });
    dock.toggleDrawer("bottom", false);
    dock.show("log");
    expect(drawerEl("bottom")!.style.display).toBe("flex");
    expect(hosts.get("log")!.style.display).toBe("");
  });

  it("closes on Escape and on a click on the scrim", () => {
    const dock = make();
    dock.show("log");
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(drawerEl("bottom")!.style.display).toBe("none");
    dock.toggleDrawer("bottom");
    expect(scrimEl("bottom")!.style.display).toBe("block");
    scrimEl("bottom")!.click();
    expect(drawerEl("bottom")!.style.display).toBe("none");
  });

  it("toggleDrawer flips a drawer and won't open an empty one", () => {
    const dock = make();
    expect(dock.toggleDrawer("bottom")).toBe(true);
    expect(dock.toggleDrawer("bottom")).toBe(false);
    expect(dock.toggleDrawer("left")).toBe(false);
    dock.place("log", { kind: "grid" });
    expect(dock.toggleDrawer("bottom", true)).toBe(false);
  });

  it("closing a drawer's last panel closes the drawer and releases the panel", () => {
    const dock = make();
    dock.show("log");
    calls = [];
    dock.toggle("log");
    expect(calls).toEqual(["release log"]);
    expect(dock.where("log")).toBeNull();
    expect(drawerEl("bottom")!.style.display).toBe("none");
  });

  it("sizes a drawer from a drag on its inner edge, clamped", () => {
    const dock = make({ drawers: { bottom: { min: 100, maxFrac: 0.5 } } });
    container.getBoundingClientRect = () => ({ left: 0, top: 0, width: 1000, height: 800, right: 1000, bottom: 800, x: 0, y: 0, toJSON() {} }) as DOMRect;
    dock.show("log");
    const handle = drawerEl("bottom")!.querySelector<HTMLElement>(".dock-drawer-handle")!;
    handle.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }) as PointerEvent);
    document.dispatchEvent(Object.assign(new MouseEvent("pointermove", { clientY: 500 }), {}) as PointerEvent);
    expect(drawerEl("bottom")!.style.height).toBe("300px");
    document.dispatchEvent(new MouseEvent("pointermove", { clientY: 10 }) as PointerEvent);
    expect(drawerEl("bottom")!.style.height).toBe("400px");
    document.dispatchEvent(new MouseEvent("pointerup") as PointerEvent);
  });
});

describe("saved drawers", () => {
  it("restores each drawer's panels, active tab, size and open state", () => {
    const storage = fakeStorage();
    const first = make({ storage, key: KEY });
    first.show("log");
    first.place("files", { kind: "drawer", edge: "bottom" });
    first.place("page:guide", { kind: "drawer", edge: "right" });
    first.toggleDrawer("right", false);
    first.dispose();
    hosts = new Map();
    const second = make({ storage, key: KEY });
    expect(tabs("bottom")).toEqual(["log", "files"]);
    expect(hosts.get("files")!.style.display).toBe("");
    expect(hosts.get("log")!.style.display).toBe("none");
    expect(drawerEl("bottom")!.style.display).toBe("flex");
    expect(tabs("right")).toEqual(["page:guide"]);
    expect(drawerEl("right")!.style.display).toBe("none");
    expect(second.where("diff")).toEqual({ kind: "grid" });
  });

  it("restores a saved drawer size", () => {
    const storage = fakeStorage();
    const first = make({ storage, key: KEY });
    first.show("log");
    container.getBoundingClientRect = () => ({ left: 0, top: 0, width: 1000, height: 800 }) as DOMRect;
    const handle = drawerEl("bottom")!.querySelector<HTMLElement>(".dock-drawer-handle")!;
    handle.dispatchEvent(new MouseEvent("pointerdown") as PointerEvent);
    document.dispatchEvent(new MouseEvent("pointermove", { clientY: 450 }) as PointerEvent);
    document.dispatchEvent(new MouseEvent("pointerup") as PointerEvent);
    first.dispose();
    const saved = JSON.parse(storage.data[KEY]);
    expect(saved.v).toBe(2);
    expect(saved.drawers.bottom.size).toBe(350);
    make({ storage, key: KEY });
    expect(drawerEl("bottom")!.style.height).toBe("350px");
  });

  it("loads a version 1 save as grid only, then puts drawer panels in their drawers", () => {
    const storage = fakeStorage();
    const first = make({ storage, key: KEY, panels: REGISTRY.slice(0, 2) });
    first.dispose();
    const v1 = JSON.parse(storage.data[KEY]);
    storage.data[KEY] = JSON.stringify({ v: 1, panels: v1.panels, layout: v1.layout });
    const dock = make({ storage, key: KEY });
    expect(dock.where("diff")).toEqual({ kind: "grid" });
    expect(dock.where("log")).toEqual({ kind: "drawer", edge: "bottom" });
  });

  it("restores a resolved panel in the grid and drops one resolve no longer knows", () => {
    const storage = fakeStorage();
    const first = make({ storage, key: KEY });
    first.show("page:a");
    first.place("page:b", { kind: "drawer", edge: "left" });
    first.dispose();
    const dock = make({ storage, key: KEY, resolve: (id) => (id === "page:a" ? { id, title: "a" } : undefined) });
    expect(dock.where("page:a")).toEqual({ kind: "grid" });
    expect(dock.where("page:b")).toBeNull();
    expect(tabs("left")).toEqual([]);
  });

  it("reset empties the drawers and puts default drawer panels back", async () => {
    const storage = fakeStorage();
    const dock = make({ storage, key: KEY });
    dock.place("files", { kind: "drawer", edge: "left" });
    dock.place("log", { kind: "grid" });
    dock.show("page:x");
    calls = [];
    dock.reset();
    expect(dock.where("files")).toEqual({ kind: "grid" });
    expect(dock.where("log")).toEqual({ kind: "drawer", edge: "bottom" });
    expect(dock.where("page:x")).toBeNull();
    expect(calls).toContain("release page:x");
    await settle();
    expect(JSON.parse(storage.data[KEY]).drawers.bottom.panels).toEqual(["log"]);
  });
});

describe("move menus", () => {
  const items = () => [...document.querySelectorAll<HTMLButtonElement>(".dock-menu .dock-menu-item")].map((b) => b.textContent);
  const pick = (label: string) => [...document.querySelectorAll<HTMLButtonElement>(".dock-menu .dock-menu-item")].find((b) => b.textContent === label)!.click();

  it("a group's ⋯ button moves its active panel", () => {
    const dock = make();
    dock.show("files");
    const btn = dock.api.getPanel("files")!.group.element.querySelector<HTMLButtonElement>(".dock-move-btn")!;
    btn.click();
    expect(items()).toEqual(["Float", "Move to left drawer", "Move to right drawer", "Move to top drawer", "Move to bottom drawer", "Close"]);
    pick("Move to right drawer");
    expect(dock.where("files")).toEqual({ kind: "drawer", edge: "right" });
    expect(document.querySelector(".dock-menu")).toBeNull();
  });

  it("a drawer tab's ⋯ docks its panel, and leaves out the drawer it's in", () => {
    const dock = make();
    dock.show("log");
    drawerEl("bottom")!.querySelector<HTMLElement>('[data-panel="log"] .dock-drawer-tab-menu')!.click();
    expect(items()).toEqual(["Dock", "Float", "Move to left drawer", "Move to right drawer", "Move to top drawer", "Close"]);
    pick("Dock");
    expect(dock.where("log")).toEqual({ kind: "grid" });
  });

  it("moveMenu: false leaves dockview's headers alone", () => {
    make({ moveMenu: false });
    expect(container.querySelector(".dock-move-btn")).toBeNull();
  });

  it("the Panels menu says where each panel is and lists open resolved panels", () => {
    const dock = make();
    dock.place("files", { kind: "drawer", edge: "left" });
    dock.show("page:guide");
    const host = document.createElement("div");
    document.body.appendChild(host);
    dock.panelsMenu(host);
    host.querySelector<HTMLButtonElement>(".dock-panels-btn")!.click();
    const labels = [...host.querySelectorAll(".dock-panels-item")].map((b) => b.textContent);
    expect(labels).toEqual(["✓ Files · left drawer", "✓ Diff", "✓ Log · bottom drawer", "✓ guide", "  Reset layout"]);
    host.remove();
  });
});
