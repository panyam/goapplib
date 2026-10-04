import { describe, expect, it } from "vitest";
import { lazy, mountIslands, type Registry } from "./mount";
import type { PageSpec } from "./spec";

type El = { slot: string };
const ctx = { tag: "ctx" };
const bus = { tag: "bus" };

const spec: PageSpec = {
  layout: "drawer",
  islands: [
    { name: "player", slot: "main", presentation: "page", config: { a: 1 } },
    { name: "unknown", slot: "side", config: {} },
    { name: "chat", slot: "missing", config: {} },
    { name: "chat", slot: "drawer", presentation: "drawer", config: {} },
  ],
};

describe("mountIslands", () => {
  it("mounts each island in its slot with its config and the page's context, in spec order", () => {
    const calls: unknown[][] = [];
    const registry: Registry<typeof ctx, El, string, typeof bus> = {
      player: (el, island, c, b) => (calls.push([el, island, c, b]), "player-component"),
      chat: (el, island) => (calls.push([el, island.presentation]), "chat-component"),
    };
    const logs: string[] = [];
    const out = mountIslands(spec, registry, (slot) => (slot === "missing" ? null : { slot }), () => ctx, bus, (m) => logs.push(m));
    expect(out).toEqual(["player-component", "chat-component"]);
    expect(calls).toEqual([
      [{ slot: "main" }, spec.islands[0], ctx, bus],
      [{ slot: "drawer" }, "drawer"],
    ]);
    // The unknown island and the missing slot are logged and skipped.
    expect(logs).toHaveLength(2);
    expect(logs.join("\n")).toMatch(/unknown/);
    expect(logs.join("\n")).toMatch(/missing/);
  });

  it("doesn't take an island name from the registry's prototype", () => {
    const logs: string[] = [];
    const out = mountIslands({ layout: "a", islands: [{ name: "toString", slot: "main", config: {} }] }, {}, (slot) => ({ slot }), () => ctx, bus, (m) => logs.push(m));
    expect(out).toEqual([]);
    expect(logs).toHaveLength(1);
  });

  it("carries on when a factory throws", () => {
    const logs: string[] = [];
    const out = mountIslands(
      spec,
      {
        player: () => {
          throw new Error("boom");
        },
        chat: () => "ok",
      },
      (slot) => ({ slot }),
      () => ctx,
      bus,
      (m) => logs.push(m),
    );
    expect(out).toEqual(["ok", "ok"]);
    expect(logs.some((m) => m.includes("boom"))).toBe(true);
  });

  it("builds the page's context once, and only when there's an island to mount", () => {
    let built = 0;
    const context = () => (built++, ctx);
    mountIslands({ layout: "index", islands: [] }, {}, () => ({ slot: "" }), context, bus, () => {});
    expect(built).toBe(0);
    mountIslands(spec, { player: () => "a", chat: () => "b" }, (slot) => ({ slot }), context, bus, () => {});
    expect(built).toBe(1);
  });

  it("hands an island that doesn't mount eagerly to defer, and reports it when it mounts", () => {
    const deferred: { name: string; mount: () => void }[] = [];
    const late: string[] = [];
    let built = 0;
    const spec: PageSpec = {
      layout: "a",
      islands: [
        { name: "below", slot: "bottom", load: "visible", config: {} },
        { name: "hero", slot: "top", config: {} },
      ],
    };
    const out = mountIslands(spec, { below: () => "below-c", hero: () => "hero-c" }, (slot) => ({ slot }), () => (built++, ctx), bus, () => {}, {
      defer: (island, _el, mount) => deferred.push({ name: island.name, mount }),
      onLateMount: (c, island) => late.push(`${island.name}:${c}`),
    });
    expect(out).toEqual(["hero-c"]);
    expect(deferred.map((d) => d.name)).toEqual(["below"]);
    expect(late).toEqual([]);
    deferred[0].mount();
    expect(late).toEqual(["below:below-c"]);
    expect(built).toBe(1);
  });

  it("builds the context on the first mount even when that one is late", () => {
    const deferred: (() => void)[] = [];
    let built = 0;
    const spec: PageSpec = { layout: "a", islands: [{ name: "below", slot: "bottom", load: "idle", config: {} }] };
    mountIslands(spec, { below: () => "c" }, (slot) => ({ slot }), () => (built++, ctx), bus, () => {}, {
      defer: (_i, _el, mount) => deferred.push(mount),
    });
    expect(built).toBe(0);
    deferred[0]();
    expect(built).toBe(1);
  });

  it("logs a load it doesn't know and mounts the island now", () => {
    const logs: string[] = [];
    const deferred: unknown[] = [];
    const spec: PageSpec = { layout: "a", islands: [{ name: "x", slot: "s", load: "lazy", config: {} }] };
    const out = mountIslands(spec, { x: () => "x-c" }, (slot) => ({ slot }), () => ctx, bus, (m) => logs.push(m), {
      defer: (...a) => deferred.push(a),
    });
    expect(out).toEqual(["x-c"]);
    expect(deferred).toEqual([]);
    expect(logs.join("\n")).toMatch(/lazy/);
  });

  it("logs a late factory that throws instead of throwing from the scheduler", () => {
    const logs: string[] = [];
    let mount = () => {};
    const spec: PageSpec = { layout: "a", islands: [{ name: "x", slot: "s", load: "visible", config: {} }] };
    mountIslands(spec, { x: () => { throw new Error("late boom"); } }, (slot) => ({ slot }), () => ctx, bus, (m) => logs.push(m), {
      defer: (_i, _el, m) => (mount = m),
    });
    expect(() => mount()).not.toThrow();
    expect(logs.join("\n")).toMatch(/late boom/);
  });

  it("mounts everything now when no defer is given, whatever the load says", () => {
    const spec: PageSpec = { layout: "a", islands: [{ name: "x", slot: "s", load: "visible", config: {} }] };
    expect(mountIslands(spec, { x: () => "x-c" }, (slot) => ({ slot }), () => ctx, bus, () => {})).toEqual(["x-c"]);
  });

  describe("lazy entries", () => {
    const flush = () => new Promise((r) => setTimeout(r, 0));

    it("loads an eager lazy island at once and reports it as a late mount", async () => {
      const late: string[] = [];
      let loads = 0;
      const spec: PageSpec = { layout: "a", islands: [{ name: "hero", slot: "top", config: {} }] };
      const registry: Registry<typeof ctx, El, string, typeof bus> = {
        hero: lazy(async () => (loads++, (el: El) => `hero in ${el.slot}`)),
      };
      const out = mountIslands(spec, registry, (slot) => ({ slot }), () => ctx, bus, () => {}, {
        defer: () => {
          throw new Error("an eager island isn't deferred");
        },
        onLateMount: (c, island) => late.push(`${island.name}:${c}`),
      });
      expect(out).toEqual([]);
      expect(loads).toBe(1);
      await flush();
      expect(late).toEqual(["hero:hero in top"]);
    });

    it("doesn't load a deferred lazy island until defer says to mount it", async () => {
      const late: string[] = [];
      let loads = 0;
      let mount = () => {};
      const spec: PageSpec = { layout: "a", islands: [{ name: "below", slot: "bottom", load: "visible", config: {} }] };
      mountIslands(spec, { below: lazy(async () => (loads++, { default: () => "below-c" })) }, (slot) => ({ slot }), () => ctx, bus, () => {}, {
        defer: (_i, _el, m) => (mount = m),
        onLateMount: (c) => late.push(c),
      });
      await flush();
      expect(loads).toBe(0);
      mount();
      expect(loads).toBe(1);
      await flush();
      expect(late).toEqual(["below-c"]);
    });

    it("logs a lazy island that fails to load or has no factory, and mounts the rest", async () => {
      const logs: string[] = [];
      const late: string[] = [];
      const spec: PageSpec = {
        layout: "a",
        islands: [
          { name: "broken", slot: "a", config: {} },
          { name: "empty", slot: "b", config: {} },
          { name: "throws", slot: "d", config: {} },
          { name: "fine", slot: "c", config: {} },
        ],
      };
      const registry = {
        broken: lazy<typeof ctx, El, string, typeof bus>(() => Promise.reject(new Error("chunk 404"))),
        throws: lazy<typeof ctx, El, string, typeof bus>(() => {
          throw new Error("no import()");
        }),
        empty: lazy<typeof ctx, El, string, typeof bus>(async () => ({}) as never),
        fine: () => "fine-c",
      };
      const out = mountIslands(spec, registry, (slot) => ({ slot }), () => ctx, bus, (m) => logs.push(m), {
        onLateMount: (c) => late.push(c),
      });
      expect(out).toEqual(["fine-c"]);
      await flush();
      expect(late).toEqual([]);
      expect(logs).toHaveLength(3);
      expect(logs.join("\n")).toMatch(/"broken" failed to load: chunk 404/);
      expect(logs.join("\n")).toMatch(/"empty".*no factory/);
      expect(logs.join("\n")).toMatch(/"throws" failed to load: no import\(\)/);
    });
  });

  it("reports every mount through onMount and every skipped island through onSkip", async () => {
    const mounts: string[] = [];
    const skips: string[] = [];
    let mountBelow = () => {};
    const spec: PageSpec = {
      layout: "a",
      islands: [
        { name: "hero", slot: "top", config: {} },
        { name: "below", slot: "bottom", load: "visible", config: {} },
        { name: "lazyOne", slot: "side", config: {} },
        { name: "ghost", slot: "foot", config: {} },
        { name: "hero", slot: "missing", config: {} },
        { name: "boom", slot: "x", config: {} },
        { name: "broken", slot: "y", config: {} },
      ],
    };
    const registry: Registry<typeof ctx, El, string, typeof bus> = {
      hero: () => "hero-c",
      below: () => "below-c",
      lazyOne: lazy(async () => () => "lazy-c"),
      boom: () => {
        throw new Error("boom");
      },
      broken: lazy(() => Promise.reject(new Error("404"))),
    };
    mountIslands(spec, registry, (slot) => (slot === "missing" ? null : { slot }), () => ctx, bus, () => {}, {
      defer: (_i, _el, m) => (mountBelow = m),
      onMount: (c, island, el) => mounts.push(`${island.name}:${c}@${el.slot}`),
      onSkip: (island, reason) => skips.push(`${island.name}@${island.slot}: ${reason}`),
    });
    expect(mounts).toEqual(["hero:hero-c@top"]);
    mountBelow();
    await new Promise((r) => setTimeout(r, 0));
    expect(mounts).toEqual(["hero:hero-c@top", "below:below-c@bottom", "lazyOne:lazy-c@side"]);
    expect(skips).toEqual(["ghost@foot: not in the registry", "hero@missing: no slot", "boom@x: factory threw", "broken@y: failed to load"]);
  });
});
