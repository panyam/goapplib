import { describe, expect, it } from "vitest";
import { mountIslands, type Registry } from "./mount";
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
});

