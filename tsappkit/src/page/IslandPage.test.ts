import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LCMComponent } from "../LCMComponent";
import { IslandPage } from "./IslandPage";
import type { PageSpec } from "./spec";

type Ctx = { built: number };
type Ext = { things: string[] };

const mounted: { name: string; el: HTMLElement; config: unknown; ctx: Ctx }[] = [];
const contexts: (PageSpec & Ext)[] = [];

const island = (name: string) => (el: HTMLElement, spec: { config: unknown }, ctx: Ctx) => {
  mounted.push({ name, el, config: spec.config, ctx });
  return { name } as unknown as LCMComponent;
};

const scheduled: { load: string | undefined; slot: string | undefined; mount: () => void }[] = [];

class TestPage extends IslandPage<Ctx, Ext> {
  protected registry() {
    return { player: island("player"), chat: island("chat") };
  }
  protected readExtension(raw: Record<string, unknown>): Ext {
    return { things: Array.isArray(raw.things) ? raw.things.map(String) : [] };
  }
  protected makeContext(spec: PageSpec & Ext): Ctx {
    contexts.push(spec);
    return { built: contexts.length };
  }
  mount() {
    return this.initializeSpecificComponents();
  }
  protected override scheduleLoad(load: Parameters<IslandPage<Ctx>["scheduleLoad"]>[0], el: HTMLElement, mount: () => void) {
    if (load.kind === "eager") mount();
    else scheduled.push({ load: load.kind, slot: el.dataset.slot, mount });
  }
}

function page(spec: string | null, slots: string[]) {
  document.body.innerHTML =
    (spec === null ? "" : `<script type="application/json" id="page-spec">${spec}</script>`) +
    slots.map((s) => `<section data-slot="${s}"></section>`).join("");
  return new TestPage("test");
}

describe("IslandPage", () => {
  let warn: ReturnType<typeof vi.spyOn>;
  beforeEach(() => {
    // jsdom has no matchMedia, and BasePage's ThemeManager asks it for the colour scheme.
    window.matchMedia ??= ((query: string) => ({ matches: false, media: query, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
    mounted.length = 0;
    contexts.length = 0;
    scheduled.length = 0;
    warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  });
  afterEach(() => warn.mockRestore());

  it("mounts each island from the spec into its data-slot, sharing one context", () => {
    const spec = JSON.stringify({
      layout: "drawer",
      islands: [
        { name: "player", slot: "main", config: { a: 1 } },
        { name: "nobody", slot: "side" },
        { name: "chat", slot: "drawer" },
      ],
      things: ["x"],
    });
    const out = page(spec, ["main", "side", "drawer"]).mount();
    expect(out).toHaveLength(2);
    expect(mounted.map((m) => [m.name, m.el.dataset.slot, m.config])).toEqual([
      ["player", "main", { a: 1 }],
      ["chat", "drawer", {}],
    ]);
    expect(mounted[0].ctx).toBe(mounted[1].ctx);
    expect(contexts).toHaveLength(1);
    expect(contexts[0].things).toEqual(["x"]);
    expect(warn.mock.calls.flat().join("\n")).toMatch(/nobody/);
  });

  it("mounts nothing, and builds no context, without a readable spec", () => {
    for (const spec of [null, "not json"]) {
      const p = page(spec, ["main"]);
      warn.mockClear();
      expect(p.mount()).toEqual([]);
      expect(warn).toHaveBeenCalledOnce();
      expect(String(warn.mock.calls[0][0])).toMatch(/page-spec/);
    }
    expect(contexts).toHaveLength(0);
  });

  it("waits to mount a deferred island, then runs it through the lifecycle", async () => {
    const phases: string[] = [];
    class LatePage extends TestPage {
      protected registry() {
        return {
          ...super.registry(),
          below: () =>
            ({
              performLocalInit: () => (phases.push("init"), []),
              setupDependencies: () => void phases.push("deps"),
              activate: () => void phases.push("activate"),
              deactivate: () => {},
            }) as LCMComponent,
        };
      }
    }
    document.body.innerHTML =
      `<script type="application/json" id="page-spec">${JSON.stringify({
        layout: "a",
        islands: [
          { name: "player", slot: "main" },
          { name: "below", slot: "bottom", load: "visible" },
        ],
      })}</script>` + `<section data-slot="main"></section><section data-slot="bottom"></section>`;
    const out = new LatePage("late").mount();
    expect(out).toHaveLength(1);
    expect(scheduled.map((s) => [s.load, s.slot])).toEqual([["visible", "bottom"]]);
    expect(phases).toEqual([]);
    scheduled[0].mount();
    await vi.waitFor(() => expect(phases).toEqual(["init", "deps", "activate"]));
  });

  it("labels each slot with the island overlay only when the URL asks for ?islands", () => {
    const spec = JSON.stringify({
      layout: "a",
      islands: [
        { name: "player", slot: "main" },
        { name: "chat", slot: "drawer", load: "visible" },
        { name: "nobody", slot: "side" },
      ],
    });
    page(spec, ["main", "drawer", "side"]).mount();
    expect(document.querySelectorAll("[data-island-debug]")).toHaveLength(0);

    window.history.replaceState({}, "", "/?islands");
    try {
      page(spec, ["main", "drawer", "side"]).mount();
      const label = (slot: string) => document.querySelector<HTMLElement>(`[data-slot="${slot}"]`)!.dataset.islandDebug;
      expect(label("main")).toMatch(/^player · main · eager · mounted \d+ ms$/);
      expect(label("drawer")).toBe("chat · drawer · visible · waiting");
      expect(label("side")).toBe("nobody · side · eager · not in the registry");
      scheduled.at(-1)!.mount();
      expect(label("drawer")).toMatch(/^chat · drawer · visible · mounted \d+ ms$/);
    } finally {
      window.history.replaceState({}, "", "/");
    }
  });
});
