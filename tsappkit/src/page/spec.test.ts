import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { readSpec } from "./spec";

describe("readSpec", () => {
  it("reads the spec goapplib's page.Spec writes (page/testdata/spec.json)", () => {
    const text = readFileSync(join(__dirname, "../../../page/testdata/spec.json"), "utf8");
    expect(readSpec(text)).toEqual({
      layout: "drawer",
      islands: [
        { name: "player", slot: "main", presentation: "page", config: { note: "</script>", urls: ["/a.json"] } },
        { name: "chat", slot: "side", config: {} },
      ],
    });
  });

  it("is null when there's no spec or it isn't one", () => {
    for (const bad of [null, undefined, "", "not json", "null", "[]", '{"islands":[]}', '{"layout":"drawer"}', '{"layout":3,"islands":[]}']) {
      expect(readSpec(bad), String(bad)).toBeNull();
    }
  });

  it("drops islands it couldn't mount and cleans the rest", () => {
    const spec = readSpec(
      JSON.stringify({
        layout: "drawer",
        islands: [
          { name: "player", slot: "main" },
          { slot: "orphan" },
          { name: "x", slot: 'main"] , body' },
          { name: "chat", slot: "drawer", presentation: 7, config: "nope" },
          "junk",
        ],
      }),
    );
    expect(spec).toEqual({
      layout: "drawer",
      islands: [
        { name: "player", slot: "main", config: {} },
        { name: "chat", slot: "drawer", config: {} },
      ],
    });
  });

  it("leaves an app's own fields out unless it reads them", () => {
    expect(readSpec('{"layout":"a","islands":[],"things":[1]}')).toEqual({ layout: "a", islands: [] });
  });

  it("merges in what an extension reads, without letting it replace layout or islands", () => {
    const text = JSON.stringify({ layout: "a", islands: [{ name: "p", slot: "main" }], things: [1, "x", 2] });
    const spec = readSpec(text, (raw) => ({
      things: Array.isArray(raw.things) ? raw.things.filter((t): t is number => typeof t === "number") : [],
      layout: "overridden",
    }));
    expect(spec).toEqual({ layout: "a", islands: [{ name: "p", slot: "main", config: {} }], things: [1, 2] });
  });

  it("is null when the extension throws", () => {
    const spec = readSpec('{"layout":"a","islands":[]}', () => {
      throw new Error("bad things");
    });
    expect(spec).toBeNull();
  });
});
