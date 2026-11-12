// The DOM-free half of a drawer: its size from a drag on any of the four edges, clamped and saved,
// and whether letting go of that drag closes it. drawerView.ts is the element.

/** The page edge a drawer is anchored to. */
export type DrawerEdge = "left" | "right" | "top" | "bottom";

/** The smallest a drawer gets, in px, unless the caller passes its own `min`. */
export const DRAWER_MIN = 280;
/** The largest share of the viewport a drawer takes, unless the caller passes its own `maxFrac`. */
export const DRAWER_MAX_FRAC = 0.9;
/** A drawer's size when nothing is saved. */
export const DRAWER_DEFAULT = 420;

/** Bounds for `clampSize` and `drawerSize`. */
export interface DrawerBounds {
  min?: number;
  maxFrac?: number;
}

/**
 * Clamps `px` to `[min, viewport * maxFrac]` and rounds it. A viewport too small for the minimum
 * gets the minimum, so the drawer can overflow a tiny window rather than vanish.
 */
export function clampSize(px: number, viewport: number, bounds: DrawerBounds = {}): number {
  const min = bounds.min ?? DRAWER_MIN;
  const max = Math.max(min, Math.floor(viewport * (bounds.maxFrac ?? DRAWER_MAX_FRAC)));
  return Math.min(max, Math.max(min, Math.round(px)));
}

/**
 * Turns a drag on a drawer's inner edge into its size. The size is the distance from the pointer
 * to the anchored edge, then clamped. Pass the pointer's `clientX` and the viewport's width for a
 * left or right drawer, and `clientY` and the height for a top or bottom one.
 */
export function drawerSize(edge: DrawerEdge, pointer: number, viewport: number, bounds?: DrawerBounds): number {
  const fromEdge = edge === "left" || edge === "top" ? pointer : viewport - pointer;
  return clampSize(fromEdge, viewport, bounds);
}

/**
 * Turns a configured or saved size into px against the viewport on the drawer's axis, clamped. A
 * size up to 1 is a share of the viewport, so `0.65` makes a phone sheet that covers 65 percent of
 * the screen whatever its height. A larger size is px.
 */
export function drawerPixels(size: number, viewport: number, bounds?: DrawerBounds): number {
  return clampSize(size <= 1 ? size * viewport : size, viewport, bounds);
}

/**
 * How fast, in px per ms toward the anchored edge, a touch drag has to be moving when it's let go
 * to close the drawer wherever it ends.
 */
export const DRAWER_FLICK = 0.5;

/**
 * Whether letting go of a drag on a drawer's inner edge closes the drawer. It closes when the edge
 * ends nearer the anchored edge than half of `min`, or when `speed` (px per ms toward the anchored
 * edge, negative away from it) is over `DRAWER_FLICK`. Pass a speed of 0 for a mouse, so a quick
 * resize on a desktop doesn't close anything.
 */
export function closesOnRelease(fromEdge: number, min: number, speed: number): boolean {
  return fromEdge < min / 2 || speed > DRAWER_FLICK;
}

/** The part of Web Storage drawer sizes need. A null storage means sizes aren't saved. */
export interface DrawerStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

/**
 * Reads a saved size, or `fallback` when nothing is saved or the value isn't a positive number.
 * It doesn't clamp, since the viewport may have changed; clamp against the live one when applying.
 */
export function loadDrawerSize(storage: DrawerStorage | null, key: string, fallback = DRAWER_DEFAULT): number {
  let raw: string | null = null;
  try {
    raw = storage?.getItem(key) ?? null;
  } catch {
    return fallback;
  }
  const n = raw == null ? NaN : parseInt(raw, 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

/** Saves a size, rounded. A storage that throws is ignored. */
export function saveDrawerSize(storage: DrawerStorage | null, key: string, px: number): void {
  try {
    storage?.setItem(key, String(Math.round(px)));
  } catch {
    // best effort
  }
}
