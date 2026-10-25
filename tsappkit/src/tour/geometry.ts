// The tour overlay's geometry, kept free of the DOM so it can be tested without layout: the holes
// as one clip path, and where the step's panel goes so it covers none of them.

/** A rectangle in viewport (client) coordinates. */
export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

/**
 * Splits the union of rects into disjoint rects. The overlay's clip path is even-odd, so two holes
 * that overlap (padded table rows always do) would dim their overlap again; disjoint pieces don't.
 */
export function unionRects(rects: readonly Rect[]): Rect[] {
  const rs = rects.filter((r) => r.width > 0 && r.height > 0);
  if (rs.length <= 1) return rs.map((r) => ({ ...r }));
  const edges = (pick: (r: Rect) => [number, number]) =>
    [...new Set(rs.flatMap(pick))].sort((a, b) => a - b);
  const xs = edges((r) => [r.x, r.x + r.width]);
  const ys = edges((r) => [r.y, r.y + r.height]);
  const covered = (cx: number, cy: number) =>
    rs.some((r) => cx > r.x && cx < r.x + r.width && cy > r.y && cy < r.y + r.height);

  const out: Rect[] = [];
  for (let j = 0; j + 1 < ys.length; j++) {
    const y = ys[j], height = ys[j + 1] - ys[j], cy = y + height / 2;
    let start: number | null = null;
    for (let i = 0; i + 1 < xs.length; i++) {
      const inside = covered((xs[i] + xs[i + 1]) / 2, cy);
      if (inside && start === null) start = xs[i];
      if (!inside && start !== null) {
        out.push({ x: start, y, width: xs[i] - start, height });
        start = null;
      }
    }
    if (start !== null) out.push({ x: start, y, width: xs[xs.length - 1] - start, height });
  }
  return mergeBands(out);
}

// Joins a piece to the one directly above it when they span the same columns, so a column of
// table rows becomes one rect rather than one per row.
function mergeBands(rs: Rect[]): Rect[] {
  const out: Rect[] = [];
  for (const r of rs) {
    const above = out.find((o) => o.x === r.x && o.width === r.width && o.y + o.height === r.y);
    if (above) above.height += r.height;
    else out.push({ ...r });
  }
  return out;
}

/** The even-odd path for a layer of width w and height h with one hole per (disjoint) rect. */
export function holePath(w: number, h: number, holes: readonly Rect[]): string {
  const n = (v: number) => Math.round(v * 10) / 10;
  let d = `M0 0H${n(w)}V${n(h)}H0Z`;
  for (const r of holes) d += `M${n(r.x)} ${n(r.y)}h${n(r.width)}v${n(r.height)}h${n(-r.width)}Z`;
  return d;
}

/** Grows a rect by pad on every side. */
export function pad(r: Rect, by: number): Rect {
  return { x: r.x - by, y: r.y - by, width: r.width + 2 * by, height: r.height + 2 * by };
}

const overlap = (a: Rect, b: Rect) =>
  Math.max(0, Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x)) *
  Math.max(0, Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y));

/** Viewports narrower than this get the panel pinned across the top or bottom. */
export const NARROW = 600;

/**
 * Where the panel (pw by ph) goes in a vw by vh viewport: beside the holes (below, above, right,
 * left of their bounding box, in that order), else a viewport corner, else whichever of those covers
 * the least of them. Under NARROW it's pinned to the bottom, or the top when the bottom is taken.
 * Every spot keeps gutter from the viewport's edges.
 */
export function placePanel(
  holes: readonly Rect[],
  vw: number,
  vh: number,
  pw: number,
  ph: number,
  gutter = 16,
  gap = 12,
): { x: number; y: number } {
  const clampX = (x: number) => Math.max(gutter, Math.min(x, vw - pw - gutter));
  const clampY = (y: number) => Math.max(gutter, Math.min(y, vh - ph - gutter));
  const spots: { x: number; y: number }[] = [];
  if (vw < NARROW) {
    spots.push({ x: gutter, y: vh - ph - gutter }, { x: gutter, y: gutter });
  } else if (holes.length === 0) {
    return { x: clampX((vw - pw) / 2), y: clampY((vh - ph) / 2) };
  } else {
    const left = Math.min(...holes.map((r) => r.x));
    const top = Math.min(...holes.map((r) => r.y));
    const right = Math.max(...holes.map((r) => r.x + r.width));
    const bottom = Math.max(...holes.map((r) => r.y + r.height));
    // Beside the box only when it fits on that side; the other axis slides to stay in view.
    if (bottom + gap + ph <= vh - gutter) spots.push({ x: clampX(left), y: bottom + gap });
    if (top - gap - ph >= gutter) spots.push({ x: clampX(left), y: top - gap - ph });
    if (right + gap + pw <= vw - gutter) spots.push({ x: right + gap, y: clampY(top) });
    if (left - gap - pw >= gutter) spots.push({ x: left - gap - pw, y: clampY(top) });
    const [x0, x1, y0, y1] = [gutter, vw - pw - gutter, gutter, vh - ph - gutter];
    spots.push({ x: x1, y: y1 }, { x: x0, y: y1 }, { x: x1, y: y0 }, { x: x0, y: y0 });
  }
  const covers = (s: { x: number; y: number }) =>
    holes.reduce((sum, r) => sum + overlap(r, { x: s.x, y: s.y, width: pw, height: ph }), 0);
  return spots.find((s) => covers(s) === 0) ?? spots.reduce((a, b) => (covers(b) < covers(a) ? b : a));
}
