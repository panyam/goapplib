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
});
