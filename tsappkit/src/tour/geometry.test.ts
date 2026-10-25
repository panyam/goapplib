import { describe, expect, it } from "vitest";
import { holePath, placePanel, type Rect, unionRects } from "./geometry";

const area = (rs: Rect[]) => rs.reduce((s, r) => s + r.width * r.height, 0);
const disjoint = (rs: Rect[]) =>
  rs.every((a, i) =>
    rs.every((b, j) => i === j || a.x + a.width <= b.x || b.x + b.width <= a.x || a.y + a.height <= b.y || b.y + b.height <= a.y),
  );

describe("unionRects", () => {
  it("leaves one rect alone and drops empty ones", () => {
    expect(unionRects([{ x: 1, y: 2, width: 3, height: 4 }, { x: 0, y: 0, width: 0, height: 9 }])).toEqual([
      { x: 1, y: 2, width: 3, height: 4 },
    ]);
  });

  it("turns padded table rows, which overlap, into one hole", () => {
    const rows = [0, 1, 2].map((i) => ({ x: -4, y: i * 20 - 4, width: 108, height: 28 }));
    expect(unionRects(rows)).toEqual([{ x: -4, y: -4, width: 108, height: 68 }]);
  });

  it("covers exactly the union of overlapping rects, in disjoint pieces", () => {
    const rs = unionRects([
      { x: 0, y: 0, width: 10, height: 10 },
      { x: 5, y: 5, width: 10, height: 10 },
    ]);
    expect(disjoint(rs)).toBe(true);
    expect(area(rs)).toBe(175);
  });

  it("keeps rects that don't touch as separate holes", () => {
    expect(unionRects([{ x: 0, y: 0, width: 5, height: 5 }, { x: 50, y: 50, width: 5, height: 5 }])).toHaveLength(2);
  });
});

describe("holePath", () => {
  it("draws the layer and then each hole, for an even-odd fill", () => {
    expect(holePath(100, 50, [{ x: 10, y: 20, width: 30, height: 5 }])).toBe("M0 0H100V50H0ZM10 20h30v5h-30Z");
  });
});

describe("placePanel", () => {
  const overlaps = (p: { x: number; y: number }, r: Rect, pw: number, ph: number) =>
    p.x < r.x + r.width && r.x < p.x + pw && p.y < r.y + r.height && r.y < p.y + ph;

  it("goes below the holes when there's room", () => {
    expect(placePanel([{ x: 100, y: 100, width: 50, height: 20 }], 1200, 800, 300, 100)).toEqual({ x: 100, y: 132 });
  });

  it("goes above the holes when they're at the bottom", () => {
    expect(placePanel([{ x: 100, y: 700, width: 50, height: 20 }], 1200, 800, 300, 100)).toEqual({ x: 100, y: 588 });
  });

  it("stays clear of holes that fill the middle of the viewport", () => {
    const holes = [{ x: 0, y: 100, width: 1200, height: 600 }];
    const p = placePanel(holes, 1200, 800, 300, 80);
    expect(holes.some((r) => overlaps(p, r, 300, 80))).toBe(false);
  });

  it("pins to the bottom on a narrow viewport, or the top when the bottom is taken", () => {
    expect(placePanel([{ x: 0, y: 0, width: 400, height: 50 }], 400, 800, 368, 120)).toEqual({ x: 16, y: 664 });
    expect(placePanel([{ x: 0, y: 700, width: 400, height: 50 }], 400, 800, 368, 120)).toEqual({ x: 16, y: 16 });
  });

  it("centres when there are no holes", () => {
    expect(placePanel([], 1200, 800, 300, 100)).toEqual({ x: 450, y: 350 });
  });
});
