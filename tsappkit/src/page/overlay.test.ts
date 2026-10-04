import { beforeEach, describe, expect, it } from "vitest";
import { IslandOverlay } from "./overlay";

describe("IslandOverlay", () => {
  beforeEach(() => {
    document.head.innerHTML = "";
    document.body.innerHTML = `<section data-slot="top"><p data-fallback>loading</p></section>`;
  });

  it("labels a slot waiting, then mounted with its time, without touching its children", () => {
    const el = document.querySelector<HTMLElement>('[data-slot="top"]')!;
    const overlay = new IslandOverlay(document, () => 212.4);
    overlay.waiting({ name: "below", slot: "top", load: "visible", config: {} }, el);
    expect(el.dataset.islandDebug).toBe("below · top · visible · waiting");
    expect(el.dataset.islandState).toBe("waiting");
    overlay.mounted({ name: "below", slot: "top", load: "visible", config: {} }, el);
    expect(el.dataset.islandDebug).toBe("below · top · visible · mounted 212 ms");
    expect(el.dataset.islandState).toBe("mounted");
    expect(el.innerHTML).toBe("<p data-fallback=\"\">loading</p>");
  });

  it("calls an island with no load eager, and labels a failure with its reason", () => {
    const el = document.querySelector<HTMLElement>('[data-slot="top"]')!;
    new IslandOverlay().failed({ name: "ghost", slot: "top", config: {} }, el, "not in the registry");
    expect(el.dataset.islandDebug).toBe("ghost · top · eager · not in the registry");
    expect(el.dataset.islandState).toBe("failed");
  });

  it("adds its stylesheet once", () => {
    new IslandOverlay();
    new IslandOverlay();
    expect(document.querySelectorAll("#island-overlay-style")).toHaveLength(1);
  });
});
