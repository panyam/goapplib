import { describe, it, expect } from "vitest";
import {
  loadLayout,
  panelsToPrune,
  panelsToReconcile,
  resolveTheme,
  saveLayout,
  type DockStorage,
  type PanelDef,
} from "./layout.js";

const KEY = "test-dock";
const REGISTRY: PanelDef[] = [
  { id: "files", title: "Files" },
  { id: "diff", title: "Diff" },
  { id: "ask", title: "Ask" },
  { id: "reason", title: "Why" },
];
const IDS = REGISTRY.map((p) => p.id);

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

describe("saved layout", () => {
  it("round-trips a layout blob through the versioned wrapper", () => {
    const s = fakeStorage();
    saveLayout(s, KEY, REGISTRY, { grid: { root: 1 } });
    expect(loadLayout(s, KEY)).toEqual({ layout: { grid: { root: 1 } }, savedPanels: IDS });
  });

  it("stamps the save with a version and the current registry", () => {
    const s = fakeStorage();
    saveLayout(s, KEY, REGISTRY, { any: "blob" });
    expect(JSON.parse(s.data[KEY])).toEqual({ v: 1, panels: IDS, layout: { any: "blob" } });
  });

  it("loads a bare pre-versioning blob with an empty registry", () => {
    const s = fakeStorage();
    s.data[KEY] = JSON.stringify({ grid: { root: 2 } });
    expect(loadLayout(s, KEY)).toEqual({ layout: { grid: { root: 2 } }, savedPanels: [] });
  });

  it("returns null when absent", () => {
    expect(loadLayout(fakeStorage(), KEY)).toBeNull();
  });

  it("returns null rather than throwing on a corrupt blob", () => {
    const s = fakeStorage();
    s.data[KEY] = "{not json";
    expect(loadLayout(s, KEY)).toBeNull();
  });

  it("ignores a storage that throws", () => {
    const broken: DockStorage = {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("quota");
      },
      removeItem: () => {},
    };
    expect(() => saveLayout(broken, KEY, REGISTRY, {})).not.toThrow();
    expect(loadLayout(broken, KEY)).toBeNull();
  });
});

describe("panelsToReconcile", () => {
  it("returns panels added to the registry since the save", () => {
    expect(panelsToReconcile(IDS.filter((p) => p !== "ask"), REGISTRY)).toEqual(["ask"]);
  });

  it("reconciles nothing when the saved registry matches", () => {
    expect(panelsToReconcile(IDS, REGISTRY)).toEqual([]);
  });

  it("treats a bare save (empty registry) as every panel new", () => {
    expect(panelsToReconcile([], REGISTRY)).toEqual(IDS);
  });

  it("leaves a new panel with defaultOpen: false for the Panels menu", () => {
    const registry = [...REGISTRY, { id: "log", title: "Log", defaultOpen: false }, { id: "map", title: "Map" }];
    expect(panelsToReconcile(IDS, registry)).toEqual(["map"]);
  });
});

describe("panelsToPrune", () => {
  it("returns open panels the registry no longer has", () => {
    expect(panelsToPrune(["files", "old", "diff", "gone"], REGISTRY)).toEqual(["old", "gone"]);
  });

  it("prunes nothing when every open panel is registered", () => {
    expect(panelsToPrune(["diff", "files"], REGISTRY)).toEqual([]);
  });
});

describe("resolveTheme", () => {
  it("honors a forced data-theme over the OS scheme", () => {
    expect(resolveTheme("dark", false)).toBe("dark");
    expect(resolveTheme("light", true)).toBe("light");
  });

  it("follows the OS scheme when unforced", () => {
    expect(resolveTheme(null, true)).toBe("dark");
    expect(resolveTheme(null, false)).toBe("light");
  });
});
