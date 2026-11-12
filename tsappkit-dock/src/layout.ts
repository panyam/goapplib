import type { DrawerEdge } from "./drawer.js";

// The DOM-free half of the dock: the panel registry's types, the saved layout, reconcile and prune,
// and theme resolution. It needs neither dockview nor a browser, so it unit-tests on its own.
// dock.ts feeds these to dockview-core.

/**
 * One panel the app can show. The registry is a list of these in a stable order, and it is saved
 * with every layout so a later load can tell a panel the app added since apart from one the reader
 * closed.
 */
export interface PanelDef {
  /** The panel's dockview id. It is also its component name and its saved name, so keep it stable. */
  id: string;
  /** The tab label. */
  title: string;
  /**
   * Whether a panel the registry gained after a layout was saved opens on that layout's next load.
   * Default `true`. Set `false` for a panel that should appear only in the Panels menu, so a new
   * feature doesn't crowd an existing reader's arrangement.
   */
  defaultOpen?: boolean;
  /**
   * Where the panel opens when it isn't in the layout yet: from reconcile, `show`, or the Panels
   * menu. When the reference panel isn't open, or this is absent, the panel's spec in the default
   * layout is tried next, and then the grid root.
   */
  position?: PanelPosition;
  /**
   * Where the panel goes when it opens on its own: on first run, on reconcile, from `show`, and on
   * Reset layout. Default `{kind: "grid"}`. A panel in the default layout's specs starts in the grid
   * unless this says a drawer or float.
   */
  defaultPlacement?: Placement;
}

/**
 * Where a panel lives. `grid` is dockview's grid, `float` a dockview floating group over it, and
 * `drawer` the drawer on that edge.
 */
export type Placement = { kind: "grid" } | { kind: "float" } | { kind: "drawer"; edge: DrawerEdge };

/** One drawer's saved state: its panels in tab order, the active one, its size, and whether it's open. */
export interface SavedDrawer {
  panels: string[];
  active?: string;
  size?: number;
  open?: boolean;
}

/** The saved drawers, by edge. An edge with no drawer is absent. */
export type SavedDrawers = Partial<Record<DrawerEdge, SavedDrawer>>;

/** A dockview `addPanel` position: a direction from a panel that is already open. */
export interface PanelPosition {
  direction: "left" | "right" | "above" | "below" | "within";
  referencePanel?: string;
}

/**
 * One `addPanel` call written as data. A default layout is a list of these, ordered so every
 * `referencePanel` is added before something refers to it. A spec without a position anchors its
 * panel at the grid root. `title` defaults to the registry's.
 */
export interface PanelSpec {
  id: string;
  title?: string;
  position?: PanelPosition;
}

/** The part of Web Storage the dock needs. `localStorage` satisfies it, and tests pass a fake. */
export interface DockStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

interface SavedLayout {
  v: 1 | 2;
  panels: string[];
  layout: unknown;
  drawers?: SavedDrawers;
}

/**
 * What `loadLayout` returns. `layout` is dockview's `toJSON()` blob. `savedPanels` is the registry
 * when it was saved, and it is empty for a bare blob from before saves carried one.
 */
export interface LoadedLayout {
  layout: unknown;
  savedPanels: string[];
  /** The drawers from a version 2 save. A version 1 or bare save has none, so it loads as grid only. */
  drawers: SavedDrawers;
}

/**
 * Saves dockview's `toJSON()` blob as `{v: 1, panels, layout}`, where `panels` is the registry's
 * ids. With `drawers` it saves `{v: 2, panels, layout, drawers}`. A storage that throws (quota,
 * private mode) is ignored, so the page keeps working and the layout just isn't saved.
 */
export function saveLayout(storage: DockStorage, key: string, registry: readonly PanelDef[], layout: unknown, drawers?: SavedDrawers): void {
  try {
    const panels = registry.map((p) => p.id);
    const saved: SavedLayout = drawers ? { v: 2, panels, layout, drawers } : { v: 1, panels, layout };
    storage.setItem(key, JSON.stringify(saved));
  } catch {
    // best effort
  }
}

/**
 * Reads a saved layout. It returns null when nothing is saved or the save isn't JSON, and the
 * caller applies its default layout in both cases. A bare dockview blob, saved before the registry
 * was saved with it, loads with an empty `savedPanels`.
 */
export function loadLayout(storage: DockStorage, key: string): LoadedLayout | null {
  let raw: string | null;
  try {
    raw = storage.getItem(key);
  } catch {
    return null;
  }
  if (!raw) return null;
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && "v" in parsed && "layout" in parsed) {
      const s = parsed as SavedLayout;
      const drawers = s.v === 2 && s.drawers && typeof s.drawers === "object" ? s.drawers : {};
      return { layout: s.layout, savedPanels: Array.isArray(s.panels) ? s.panels : [], drawers };
    }
    return { layout: parsed, savedPanels: [], drawers: {} };
  } catch {
    return null;
  }
}

/**
 * Returns the ids of panels to open on a restored layout. These are the registry's panels that
 * the saved registry lacks and that don't have `defaultOpen: false`. A panel the reader closed was
 * in the saved registry, so it stays closed. A bare save has an empty registry, so every panel
 * counts as new once; the caller skips the ones the layout already has open.
 */
export function panelsToReconcile(savedPanels: readonly string[], registry: readonly PanelDef[]): string[] {
  return registry.filter((p) => p.defaultOpen !== false && !savedPanels.includes(p.id)).map((p) => p.id);
}

/**
 * Returns the ids in `open` that the registry no longer has. A restored layout can name a panel a
 * later release removed, and without pruning it shows as an empty tab.
 */
export function panelsToPrune(open: readonly string[], registry: readonly PanelDef[]): string[] {
  const known = new Set(registry.map((p) => p.id));
  return open.filter((id) => !known.has(id));
}

/**
 * Maps a page's theme setting to light or dark. A `data-theme` of `light` or `dark` wins, and
 * anything else follows the OS, which the caller passes as `prefersDark`.
 */
export function resolveTheme(dataTheme: string | null, prefersDark: boolean): "dark" | "light" {
  if (dataTheme === "dark") return "dark";
  if (dataTheme === "light") return "light";
  return prefersDark ? "dark" : "light";
}

/**
 * Reads `resolveTheme` off the document: `data-theme` on `<html>`, then `prefers-color-scheme`.
 * It is `createDock`'s default `theme`.
 */
export function documentTheme(doc: Document = document): "dark" | "light" {
  const forced = doc.documentElement.getAttribute("data-theme");
  const prefersDark = doc.defaultView?.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
  return resolveTheme(forced, prefersDark);
}
