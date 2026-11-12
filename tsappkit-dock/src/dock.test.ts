import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createDock, type Dock, type DockOptions } from "./dock.js";
import type { DockStorage, PanelDef, PanelSpec } from "./layout.js";

const KEY = "test-dock";
const REGISTRY: PanelDef[] = [
  { id: "files", title: "Files" },
  { id: "diff", title: "Diff" },
  { id: "ask", title: "Ask", position: { direction: "right", referencePanel: "diff" } },
];
const LAYOUT: PanelSpec[] = [
  { id: "diff" },
  { id: "files", position: { direction: "left", referencePanel: "diff" } },
  { id: "ask", position: { direction: "right", referencePanel: "diff" } },
];

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
let released: string[];
const docks: Dock[] = [];

function make(over: Partial<DockOptions> = {}): Dock {
  const dock = createDock({
    container,
    panels: REGISTRY,
    defaultLayout: LAYOUT,
    hostFor: (id) => {
      let el = hosts.get(id);
      if (!el) {
        el = document.createElement("div");
        el.dataset.host = id;
        hosts.set(id, el);
      }
      return el;
    },
    release: (id) => released.push(id),
    theme: () => "light",
    ...over,
  });
  docks.push(dock);
  return dock;
}

const openIds = (d: Dock) => d.api.panels.map((p) => p.id).sort();
// dockview reports layout changes on a microtask, and the dock saves when it hears one.
const settle = () => new Promise((r) => setTimeout(r, 0));

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  hosts = new Map();
  released = [];
});

afterEach(() => {
  for (const d of docks.splice(0)) d.dispose();
  container.remove();
  document.documentElement.removeAttribute("data-theme");
});

describe("createDock", () => {
  it("applies the default layout when nothing is saved", () => {
    const dock = make({ storage: fakeStorage(), key: KEY });
    expect(openIds(dock)).toEqual(["ask", "diff", "files"]);
    expect(dock.api.getPanel("files")!.view.content.element).toBe(hosts.get("files"));
  });

  it("restores a saved arrangement", () => {
    const storage = fakeStorage();
    const first = make({ storage, key: KEY });
    first.api.removePanel(first.api.getPanel("ask")!);
    first.dispose();
    const second = make({ storage, key: KEY });
    expect(openIds(second)).toEqual(["diff", "files"]);
  });

  it("opens a panel the registry gained after the save, and keeps a closed one closed", () => {
    const storage = fakeStorage();
    const first = make({ storage, key: KEY, panels: REGISTRY.slice(0, 2), defaultLayout: LAYOUT.slice(0, 2) });
    first.api.removePanel(first.api.getPanel("files")!);
    first.dispose();
    const second = make({ storage, key: KEY });
    expect(openIds(second)).toEqual(["ask", "diff"]);
  });

  it("closes a saved panel the registry dropped", () => {
    const storage = fakeStorage();
    make({ storage, key: KEY }).dispose();
    released = [];
    const second = make({ storage, key: KEY, panels: REGISTRY.slice(0, 2) });
    expect(openIds(second)).toEqual(["diff", "files"]);
    expect(released).toEqual([]);
  });

  it("falls back to the default layout and drops a save dockview rejects", async () => {
    const storage = fakeStorage();
    storage.data[KEY] = JSON.stringify({ v: 1, panels: ["diff"], layout: { grid: "nonsense" } });
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const dock = make({ storage, key: KEY });
    expect(openIds(dock)).toEqual(["ask", "diff", "files"]);
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
    await settle();
    expect(JSON.parse(storage.data[KEY]).layout.grid).not.toBe("nonsense");
  });

  it("calls release when a panel closes and hands the same element back when it reopens", () => {
    const dock = make();
    const el = hosts.get("ask");
    dock.toggle("ask");
    expect(released).toEqual(["ask"]);
    expect(dock.api.getPanel("ask")).toBeUndefined();
    dock.show("ask");
    expect(dock.api.getPanel("ask")!.view.content.element).toBe(el);
  });

  it("show opens a closed panel at its position and makes it active", () => {
    const dock = make();
    dock.toggle("files");
    expect(dock.show("files")).toBe(true);
    expect(dock.api.activePanel?.id).toBe("files");
    dock.show("diff");
    expect(dock.api.activePanel?.id).toBe("diff");
    expect(dock.show("nope")).toBe(false);
  });

  it("reset forgets the save and applies the default, releasing every panel", async () => {
    const storage = fakeStorage();
    const dock = make({ storage, key: KEY });
    dock.toggle("ask");
    released = [];
    dock.reset();
    expect(openIds(dock)).toEqual(["ask", "diff", "files"]);
    expect(released.sort()).toEqual(["diff", "files"]);
    await settle();
    expect(JSON.parse(storage.data[KEY]).panels).toEqual(["files", "diff", "ask"]);
  });

  it("takes a function as the default layout", () => {
    const dock = make({ defaultLayout: (api) => api.addPanel({ id: "diff", component: "diff", title: "Diff" }) });
    expect(openIds(dock)).toEqual(["diff"]);
  });

  it("gives an unregistered id an empty element without calling hostFor", () => {
    const hostFor = vi.fn((id: string) => document.createElement("div"));
    const dock = make({ hostFor, defaultLayout: (api) => api.addPanel({ id: "stray", component: "stray" }) });
    expect(dock.api.getPanel("stray")).toBeDefined();
    expect(hostFor).not.toHaveBeenCalled();
  });

  it("follows <html data-theme>", async () => {
    const dock = make({ theme: undefined });
    const themed = () => container.querySelector(".dockview-theme-dark") !== null || container.classList.contains("dockview-theme-dark");
    expect(themed()).toBe(false);
    document.documentElement.setAttribute("data-theme", "dark");
    await settle();
    expect(themed()).toBe(true);
    dock.dispose();
  });

  it("requires a key with storage", () => {
    expect(() => make({ storage: fakeStorage() })).toThrow(/key/);
  });
});

describe("panelsMenu", () => {
  it("lists every panel with a ✓ on the open ones, toggles them, and resets", () => {
    const dock = make({ storage: fakeStorage(), key: KEY });
    dock.toggle("ask");
    const host = document.createElement("div");
    document.body.appendChild(host);
    const remove = dock.panelsMenu(host);
    const btn = host.querySelector<HTMLButtonElement>(".dock-panels-btn")!;
    const list = host.querySelector<HTMLElement>(".dock-panels-list")!;
    btn.click();
    expect(list.style.display).toBe("block");
    const items = [...list.querySelectorAll<HTMLButtonElement>(".dock-panels-item")].map((b) => b.textContent);
    expect(items).toEqual(["✓ Files", "✓ Diff", "   Ask", "  Reset layout"]);

    list.querySelectorAll<HTMLButtonElement>(".dock-panels-item")[2].click();
    expect(dock.api.getPanel("ask")).toBeDefined();
    expect(list.style.display).toBe("none");

    btn.click();
    list.querySelectorAll<HTMLButtonElement>(".dock-panels-item")[0].click();
    expect(dock.api.getPanel("files")).toBeUndefined();

    btn.click();
    list.querySelector<HTMLButtonElement>(".dock-panels-reset")!.click();
    expect(openIds(dock)).toEqual(["ask", "diff", "files"]);

    btn.click();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(list.style.display).toBe("none");

    remove();
    expect(host.children.length).toBe(0);
    host.remove();
  });
});
