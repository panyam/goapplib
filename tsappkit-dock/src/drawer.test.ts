import { describe, it, expect } from "vitest";
import { clampSize, closesOnRelease, drawerPixels, drawerSize, loadDrawerSize, saveDrawerSize, DRAWER_FLICK, DRAWER_MIN, DRAWER_DEFAULT, type DrawerStorage } from "./drawer.js";

function fakeStorage(seed: Record<string, string> = {}): DrawerStorage & { data: Record<string, string> } {
  const data = { ...seed };
  return {
    data,
    getItem: (k) => (k in data ? data[k] : null),
    setItem: (k, v) => {
      data[k] = v;
    },
  };
}

describe("clampSize", () => {
  it("keeps a size within [min, viewport*0.9]", () => {
    expect(clampSize(500, 1400)).toBe(500);
  });
  it("floors at DRAWER_MIN", () => {
    expect(clampSize(50, 1400)).toBe(DRAWER_MIN);
  });
  it("caps at 90% of the viewport", () => {
    expect(clampSize(2000, 1000)).toBe(900);
  });
  it("pins to the minimum on a tiny viewport", () => {
    expect(clampSize(1000, 200)).toBe(DRAWER_MIN);
  });
  it("takes its own bounds", () => {
    expect(clampSize(50, 800, { min: 120 })).toBe(120);
    expect(clampSize(700, 800, { maxFrac: 0.5 })).toBe(400);
  });
});

describe("drawerSize", () => {
  it("measures a left drawer from the left edge", () => {
    expect(drawerSize("left", 500, 1400)).toBe(500);
  });
  it("measures a right drawer from the right edge (viewport - clientX)", () => {
    expect(drawerSize("right", 900, 1400)).toBe(500);
  });
  it("floors a right drawer at DRAWER_MIN when the pointer nears the right edge", () => {
    expect(drawerSize("right", 1390, 1400)).toBe(DRAWER_MIN);
  });
  it("caps a right drawer at 90% of the viewport when the pointer nears the left edge", () => {
    expect(drawerSize("right", 0, 1000)).toBe(900);
  });
  it("measures a top drawer from the top edge", () => {
    expect(drawerSize("top", 300, 900, { min: 100 })).toBe(300);
  });
  it("measures a bottom drawer from the bottom edge (viewport - clientY)", () => {
    expect(drawerSize("bottom", 600, 900, { min: 100 })).toBe(300);
    expect(drawerSize("bottom", 0, 900)).toBe(810);
  });
});

describe("drawer size persistence", () => {
  it("round-trips a saved size", () => {
    const s = fakeStorage();
    saveDrawerSize(s, "k", 512);
    expect(loadDrawerSize(s, "k")).toBe(512);
  });
  it("falls back to the default when absent or unparseable", () => {
    expect(loadDrawerSize(fakeStorage(), "k")).toBe(DRAWER_DEFAULT);
    expect(loadDrawerSize(fakeStorage({ k: "junk" }), "k")).toBe(DRAWER_DEFAULT);
    expect(loadDrawerSize(fakeStorage(), "k", 200)).toBe(200);
  });
  it("tolerates a null storage", () => {
    expect(loadDrawerSize(null, "k")).toBe(DRAWER_DEFAULT);
    expect(() => saveDrawerSize(null, "k", 400)).not.toThrow();
  });
});

describe("drawerPixels", () => {
  it("reads a size up to 1 as a share of the viewport", () => {
    expect(drawerPixels(0.65, 844)).toBe(549);
  });
  it("reads a size over 1 as px", () => {
    expect(drawerPixels(420, 1000)).toBe(420);
  });
  it("clamps a share like a px size", () => {
    expect(drawerPixels(0.1, 1000)).toBe(DRAWER_MIN);
    expect(drawerPixels(1, 1000)).toBe(900);
  });
});

describe("closesOnRelease", () => {
  it("closes when the edge ends past half the minimum", () => {
    expect(closesOnRelease(139, 280, 0)).toBe(true);
    expect(closesOnRelease(141, 280, 0)).toBe(false);
  });
  it("closes on a flick toward the edge, wherever it ends", () => {
    expect(closesOnRelease(400, 280, DRAWER_FLICK + 0.01)).toBe(true);
    expect(closesOnRelease(400, 280, DRAWER_FLICK - 0.01)).toBe(false);
  });
  it("ignores a flick away from the edge", () => {
    expect(closesOnRelease(400, 280, -2)).toBe(false);
  });
});
