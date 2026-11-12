import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createDock, type Dock, type DockOptions } from "./dock.js";
import type { PanelDef, PanelSpec } from "./layout.js";

const REGISTRY: PanelDef[] = [
  { id: "files", title: "Files" },
  { id: "diff", title: "Diff" },
  { id: "log", title: "Log", defaultPlacement: { kind: "drawer", edge: "bottom" } },
];
const LAYOUT: PanelSpec[] = [{ id: "diff" }, { id: "files", position: { direction: "left", referencePanel: "diff" } }];

let container: HTMLElement;
let hosts: Map<string, HTMLElement>;
let moves: string[];
const docks: Dock[] = [];
const settle = () => new Promise((r) => setTimeout(r, 0));
const at = (p: { kind: string; edge?: string } | null) => (p ? (p.edge ? `${p.kind}:${p.edge}` : p.kind) : "closed");

function make(over: Partial<DockOptions> = {}): Dock {
  const dock = createDock({
    container,
    panels: REGISTRY,
    defaultLayout: LAYOUT,
    resolve: (id) => (id.startsWith("page:") ? { id, title: id } : undefined),
    hostFor: (id) => hosts.get(id) ?? (hosts.set(id, Object.assign(document.createElement("div"), { id: `host-${id}` })), hosts.get(id)!),
    moved: (id, el, to, from, info) => {
      expect(el).toBe(hosts.get(id));
      moves.push(`${id} ${at(from)}→${at(to)}${info.documentChanged ? " doc" : ""}`);
    },
    theme: () => "light",
    ...over,
  });
  docks.push(dock);
  return dock;
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  hosts = new Map();
  moves = [];
});

afterEach(() => {
  for (const d of docks.splice(0)) d.dispose();
  container.remove();
});

describe("moved", () => {
  it("isn't called for the layout the dock starts with", async () => {
    make();
    await settle();
    expect(moves).toEqual([]);
  });

  it("reports each place, with the element already there", () => {
    const dock = make({
      moved: (id, el, to) => {
        moves.push(`${id}→${at(to)} attached=${el.isConnected}`);
      },
    });
    dock.place("files", { kind: "drawer", edge: "left" });
    dock.place("files", { kind: "float" });
    dock.place("files", { kind: "grid" });
    expect(moves).toEqual(["files→drawer:left attached=true", "files→float attached=true", "files→grid attached=true"]);
  });

  it("reports show, toggle and a runtime panel opening and closing", () => {
    const dock = make();
    dock.toggle("files");
    dock.show("files");
    dock.show("log");
    dock.show("page:x");
    dock.toggle("page:x");
    expect(moves).toEqual(["files grid→closed", "files closed→grid", "page:x closed→grid", "page:x grid→closed"]);
  });

  it("reports a move made through dock.api, once dockview says the layout changed", async () => {
    const dock = make();
    dock.api.addFloatingGroup(dock.api.getPanel("diff")!);
    expect(moves).toEqual([]);
    await settle();
    expect(moves).toEqual(["diff grid→float"]);
  });

  it("reports Reset layout as one net move per panel", () => {
    const dock = make();
    dock.place("files", { kind: "drawer", edge: "right" });
    dock.place("log", { kind: "grid" });
    moves = [];
    dock.reset();
    expect(moves.sort()).toEqual(["files drawer:right→grid", "log grid→drawer:bottom"]);
  });

  it("doesn't report a panel whose place didn't change", async () => {
    const dock = make();
    dock.show("diff");
    dock.place("diff", { kind: "grid" });
    dock.toggleDrawer("bottom");
    await settle();
    expect(moves).toEqual([]);
  });

  it("says documentChanged only when the element's document changed", () => {
    const dock = make();
    dock.place("files", { kind: "drawer", edge: "left" });
    dock.place("files", { kind: "grid" });
    expect(moves).toEqual(["files grid→drawer:left", "files drawer:left→grid"]);
    // Stand in for a popout: the element moves into another document, then anything that makes the
    // dock look again (here a drawer opening) reports it.
    const other = document.implementation.createHTMLDocument("popout");
    other.body.appendChild(other.adoptNode(hosts.get("files")!));
    dock.toggleDrawer("bottom");
    expect(moves[moves.length - 1]).toBe("files grid→grid doc");
  });
});
