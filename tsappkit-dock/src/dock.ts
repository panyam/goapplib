import { DockviewComponent, themeDark, themeLight } from "dockview-core";
import type { DockviewApi, DockviewGroupPanel, IContentRenderer, IHeaderActionsRenderer } from "dockview-core";
import { DRAWER_DEFAULT, type DrawerEdge } from "./drawer.js";
import { DrawerView, type DrawerConfig } from "./drawerView.js";
import {
  documentTheme,
  loadLayout,
  panelsToReconcile,
  saveLayout,
  type DockStorage,
  type PanelDef,
  type PanelPosition,
  type PanelSpec,
  type Placement,
  type SavedDrawers,
} from "./layout.js";

const EDGES: readonly DrawerEdge[] = ["left", "right", "top", "bottom"];

/**
 * Where a panel is: a `Placement`, or a dockview popout window. Popouts come from `dock.api`, since
 * the dock doesn't place panels there itself (see the dock experiments page for why).
 */
export type PanelLocation = Placement | { kind: "popout" };

/** What `moved` gets besides the panel and its old and new place. */
export interface MoveInfo {
  /**
   * True when the panel's element is in a different document from the last time the dock saw it,
   * which happens on the way into a popout window and on the way back. A canvas can't draw again
   * once the window it was in has closed, so rebuild canvases (and WebGL contexts) when this is
   * true. Moves within one document leave canvases working.
   */
  documentChanged: boolean;
}

/** What `createDock` needs. */
export interface DockOptions {
  /** The element dockview renders into. It needs a size, as dockview fills it. */
  container: HTMLElement;
  /** The panel registry, in a stable order. These are the panels the Panels menu lists. */
  panels: readonly PanelDef[];
  /**
   * Describes a panel the registry doesn't list, such as one per document the reader opens.
   * Return undefined for an id that isn't one. A resolvable panel opens with `show(id)`, can be
   * placed anywhere, and is kept when a saved layout names it. The Panels menu lists it while it's
   * open.
   */
  resolve?: (id: string) => PanelDef | undefined;
  /**
   * The arrangement used when nothing is saved, after a rejected save, and on `reset`. Either
   * `addPanel` specs as data, or a function that builds it on the api, for an app that sizes
   * groups after adding them. Registry panels whose `defaultPlacement` is a drawer or a float go
   * there afterwards, whichever form this takes.
   */
  defaultLayout: readonly PanelSpec[] | ((api: DockviewApi) => void);
  /**
   * Returns the element a panel shows. The dock calls it each time the panel arrives somewhere: in
   * the grid, in a float, or in a drawer. Return the same element every time if its content should
   * survive a move, a close and a reopen. It's only called with ids the registry or `resolve` knows.
   */
  hostFor: (id: string) => HTMLElement;
  /**
   * Called when a panel leaves where it was, with the element `hostFor` gave it, already detached.
   * Closing a panel calls it, and so does moving it between the grid and a drawer, before `hostFor`
   * is called for the new place. Moving it between the grid and a float doesn't, since dockview
   * keeps the element. Clearing the layout, as `reset` does, calls it for every open panel.
   */
  release?: (id: string, el: HTMLElement) => void;
  /**
   * Called after a panel ends up somewhere new, with its element already in place. `to` and
   * `from` are where it is and where it was, and null means closed. It covers every way a panel
   * moves: `place`, `show`, `toggle`, Reset layout, the reader dragging a tab into a float, a popout
   * made through `dock.api`, and a popout window closing. It isn't called for the layout the dock
   * starts with. See `MoveInfo.documentChanged` for the one move that breaks a canvas.
   */
  moved?: (id: string, el: HTMLElement, to: PanelLocation | null, from: PanelLocation | null, info: MoveInfo) => void;
  /**
   * Called when a drawer opens, closes, or shows a different tab, with whether it's open and its
   * active panel afterwards. A phone page uses it to highlight the button for the panel on show.
   * It covers the reader's taps, drags and Escape as well as `show`, `place`, `toggleDrawer` and
   * Reset layout, and isn't called for the drawers the dock starts with.
   */
  drawerChanged?: (edge: DrawerEdge, open: boolean, active: string | undefined) => void;
  /** Where the layout is saved, usually `localStorage`. Null or absent: nothing is saved. */
  storage?: DockStorage | null;
  /** The storage key. Required with `storage`. */
  key?: string;
  /**
   * Returns the dock's theme. It is read at creation, when `<html data-theme>` changes, when the
   * OS colour scheme changes, and on `applyTheme()`. Default: `documentTheme()`.
   */
  theme?: () => "light" | "dark";
  /** Each edge's drawer size and bounds. An edge left out gets the defaults from drawer.ts. */
  drawers?: Partial<Record<DrawerEdge, DrawerConfig>>;
  /**
   * The element drawers are laid over, and the box their sizes are measured against. Default
   * `container`. The dock makes it `position: relative` if it's static.
   */
  drawerHost?: HTMLElement;
  /**
   * Whether each dockview group gets a ⋯ button with Dock, Float and the four drawers for its
   * active panel. Default true. Drawer tabs always have one.
   */
  moveMenu?: boolean;
}

/** A dock made by `createDock`. */
export interface Dock {
  /** The dockview api, for anything the dock doesn't wrap. */
  readonly api: DockviewApi;
  /**
   * Brings a panel to the front wherever it is, opening its drawer if it's in one. A closed panel
   * opens at its `defaultPlacement`. It returns false, and does nothing, for an id that neither the
   * registry nor `resolve` knows.
   */
  show(id: string): boolean;
  /**
   * Moves a panel to `target` and brings it to the front. A closed panel opens there. False for an
   * unknown id.
   */
  place(id: string, target: Placement): boolean;
  /** Where a panel is now, or null when it's closed. A panel in a dockview popout reports `popout`. */
  where(id: string): PanelLocation | null;
  /** Opens a closed panel or closes an open one. False for an unknown id. */
  toggle(id: string): boolean;
  /**
   * Opens or closes the drawer on `edge`, or flips it when `open` is absent. An empty drawer stays
   * closed. Returns whether it's open afterwards.
   */
  toggleDrawer(edge: DrawerEdge, open?: boolean): boolean;
  /**
   * Forgets the saved layout and applies the default one, drawers included. Panels keep their
   * `hostFor` elements.
   */
  reset(): void;
  /** Reads `theme` again and applies it. For a theme setting the dock doesn't watch. */
  applyTheme(): void;
  /**
   * Adds a Panels menu button to `host`. Its list has one item per registry panel and per open
   * resolved panel, each toggling its panel, with a ✓ on the open ones and where a panel is when
   * it's not in the grid, and then Reset layout. Returns a function that removes it. The elements
   * carry `dock-panels-*` classes and no styles beyond showing and hiding the list.
   */
  panelsMenu(host: HTMLElement): () => void;
  /**
   * Saves the layout, stops watching the theme, removes the menus and drawers, and disposes of
   * dockview. Panels in drawers are released. Changes are otherwise saved a microtask after
   * dockview reports them. A dock whose container never had a size saves nothing, here or before,
   * since its grid has no real size to save. A second call does nothing.
   */
  dispose(): void;
}

const placementLabel = (p: PanelLocation | null): string =>
  !p || p.kind === "grid" ? "" : p.kind === "drawer" ? ` · ${p.edge} drawer` : ` · ${p.kind}`;

const TOWARD_EDGE: Record<DrawerEdge, PanelPosition["direction"]> = { left: "left", right: "right", top: "above", bottom: "below" };

/**
 * Builds a dockview dock from a panel registry. It restores the saved layout when there is one,
 * drawers included, opens the panels added to the registry since it was saved, and closes the
 * ones nothing knows any more. Drawers come back with their panels, tab and size, but closed,
 * unless their `DrawerConfig.restoreOpen` is set. A save that dockview rejects is removed and the
 * default layout applied. Every change after that is saved, once the container has a size.
 *
 * Content lives outside dockview. `hostFor` hands an element to a panel when it arrives somewhere,
 * and `release` hands it back when it leaves, so an app can keep a panel's content alive while the
 * panel is closed or moving between the grid and a drawer.
 */
export function createDock(opts: DockOptions): Dock {
  const { container, panels, hostFor, release } = opts;
  const storage = opts.storage ?? null;
  const key = opts.key ?? "";
  if (storage && !key) throw new Error("createDock: storage needs a key");
  const theme = opts.theme ?? (() => documentTheme(container.ownerDocument));
  const doc = container.ownerDocument;
  const byId = new Map(panels.map((p) => [p.id, p]));
  const specs = Array.isArray(opts.defaultLayout) ? (opts.defaultLayout as readonly PanelSpec[]) : null;
  const cleanups: (() => void)[] = [];
  const drawerHost = opts.drawerHost ?? container;
  const drawers = new Map<DrawerEdge, DrawerView>();
  let disposed = false;
  // Set while the dock rebuilds itself, so the drawers' change hooks don't save a half-built state.
  let quiet = 0;

  const defOf = (id: string): PanelDef | undefined => {
    const known = byId.get(id);
    if (known) return known;
    const def = opts.resolve?.(id);
    return def && def.id === id ? def : undefined;
  };

  const renderer = (id: string): IContentRenderer => {
    if (!defOf(id)) return { element: doc.createElement("div"), init: () => {} };
    const element = hostFor(id);
    return { element, init: () => {}, dispose: () => release?.(id, element) };
  };

  const component = new DockviewComponent(container, {
    createComponent: (o) => renderer(o.id),
    // "always" keeps a background tab's content attached, only hidden, in a layer dockview
    // positions over the group. The default detaches it, and an app that finds its content by id
    // at startup would miss the panels behind a tab. The layer also means dragging a tab between
    // groups doesn't re-parent the content, so an iframe in it doesn't reload.
    defaultRenderer: "always",
    theme: theme() === "dark" ? themeDark : themeLight,
    ...(opts.moveMenu === false ? {} : { createRightHeaderActionComponent: () => headerMenu() }),
  });
  const api = component.api;

  // ---- where panels are ----

  const drawerWith = (id: string): DrawerView | undefined => [...drawers.values()].find((d) => d.has(id));

  const where = (id: string): PanelLocation | null => {
    const d = drawerWith(id);
    if (d) return { kind: "drawer", edge: d.edge };
    const panel = api.getPanel(id);
    if (!panel) return null;
    const type = panel.group.api.location.type;
    return type === "floating" ? { kind: "float" } : type === "popout" ? { kind: "popout" } : { kind: "grid" };
  };

  const elementOf = (id: string): HTMLElement | undefined =>
    drawerWith(id)?.elementOf(id) ?? (api.getPanel(id)?.view.content.element as HTMLElement | undefined);

  const same = (a: PanelLocation | null, b: Placement): boolean =>
    !!a && a.kind === b.kind && (a.kind !== "drawer" || (b.kind === "drawer" && a.edge === b.edge));

  // ---- drawers ----

  const drawer = (edge: DrawerEdge): DrawerView => {
    let d = drawers.get(edge);
    if (!d) {
      if (doc.defaultView?.getComputedStyle(drawerHost).position === "static") drawerHost.style.position = "relative";
      d = new DrawerView(edge, drawerHost, opts.drawers?.[edge] ?? {}, { changed: () => changed(), menu: (anchor, id) => moveMenu(anchor, id) });
      drawers.set(edge, d);
    }
    return d;
  };

  const drawerStates = (): SavedDrawers => {
    const out: SavedDrawers = {};
    // A drawer is saved once it has held a panel or been resized, so an empty one keeps its size.
    for (const [edge, d] of drawers) if (d.ids.length || d.size !== (opts.drawers?.[edge]?.size ?? DRAWER_DEFAULT)) out[edge] = d.state();
    return out;
  };

  // ---- moving panels ----

  const usable = (p?: PanelPosition): p is PanelPosition => !!p && (!p.referencePanel || !!api.getPanel(p.referencePanel));
  const inGrid = (p?: PanelPosition): p is PanelPosition =>
    !!p && (!p.referencePanel || (api.getPanel(p.referencePanel)?.group.api.location.type === "grid" && api.getPanel(p.referencePanel)!.group.api.isVisible));

  // Puts a panel in the grid, never in a float, a popout, or a popout's hidden placeholder. A position
  // counts only if its reference panel is in the grid. Without one the panel tabs into the active group, or the first grid group
  // when the active one is a float or a popout, and goes to the grid's edge when the grid is empty.
  const addToGrid = (id: string, prefer?: PanelPosition): void => {
    const def = defOf(id)!;
    const spec = specs?.find((s) => s.id === id);
    const position = [prefer, def.position, spec?.position].find(inGrid);
    if (position) {
      api.addPanel({ id, component: id, title: def.title, position });
      return;
    }
    // While a group is popped out, dockview keeps a hidden placeholder for it in the grid, and a
    // panel tabbed into that would be invisible.
    const showing = (g?: DockviewGroupPanel) => !!g && g.api.location.type === "grid" && g.api.isVisible;
    const active = api.activeGroup;
    const group = showing(active) ? active : api.groups.find(showing);
    api.addPanel({
      id,
      component: id,
      title: def.title,
      position: group ? { referenceGroup: group, direction: "within" } : { direction: "right" },
    });
  };

  const enter = (id: string, target: Placement): void => {
    if (target.kind === "drawer") {
      drawer(target.edge).add(id, defOf(id)!.title, hostFor(id));
      return;
    }
    if (!api.getPanel(id)) addToGrid(id);
    const panel = api.getPanel(id)!;
    if (target.kind === "float" && panel.group.api.location.type !== "floating") api.addFloatingGroup(panel);
  };

  const leave = (id: string): void => {
    const d = drawerWith(id);
    if (d) {
      const el = d.remove(id);
      if (el) release?.(id, el);
      return;
    }
    const panel = api.getPanel(id);
    if (panel) api.removePanel(panel);
  };

  const reveal = (id: string): void => {
    const d = drawerWith(id);
    if (d) {
      d.activate(id);
      d.setOpen(true);
      return;
    }
    api.getPanel(id)?.api.setActive();
  };

  // Runs a move with saving and moved held back, then does both once, so a move that passes through
  // a closed state on its way (leave, then enter) reports only where it ended up.
  const batch = (fn: () => void): void => {
    quiet++;
    try {
      fn();
    } finally {
      quiet--;
    }
    changed();
  };

  const place = (id: string, target: Placement): boolean => {
    if (!defOf(id)) return false;
    batch(() => placeNow(id, target));
    return true;
  };

  const placeNow = (id: string, target: Placement): void => {
    const now = where(id);
    if (!same(now, target)) {
      if (now?.kind === "drawer" && target.kind === "grid") {
        // Docking from a drawer puts the panel where the drawer was: on its edge, at its size.
        const size = drawer(now.edge).pixels;
        leave(id);
        addToGrid(id, { direction: TOWARD_EDGE[now.edge] });
        const horizontal = now.edge === "left" || now.edge === "right";
        api.getPanel(id)?.api.setSize(horizontal ? { width: size } : { height: size });
      } else {
        // Grid to float stays inside dockview, which moves the group and keeps the element. Every
        // other move goes out through release and back in through hostFor.
        if (!(now?.kind === "grid" && target.kind === "float")) leave(id);
        enter(id, target);
      }
    }
    reveal(id);
  };

  const show = (id: string): boolean => {
    const def = defOf(id);
    if (!def) return false;
    batch(() => {
      if (!where(id)) enter(id, def.defaultPlacement ?? { kind: "grid" });
      reveal(id);
    });
    return true;
  };

  const toggle = (id: string): boolean => {
    if (!defOf(id)) return false;
    if (!where(id)) return show(id);
    batch(() => leave(id));
    return true;
  };

  // ---- default layout, restore, reset ----

  const applyDefault = (): void => {
    const elsewhere = (def?: PanelDef) => !!def?.defaultPlacement && def.defaultPlacement.kind === "drawer";
    if (specs) {
      for (const spec of specs) {
        const def = byId.get(spec.id);
        if (!def || elsewhere(def) || api.getPanel(spec.id)) continue;
        const position = usable(spec.position) ? spec.position : undefined;
        api.addPanel({ id: spec.id, component: spec.id, title: spec.title ?? def.title, ...(position ? { position } : {}) });
      }
    } else {
      (opts.defaultLayout as (api: DockviewApi) => void)(api);
    }
    for (const def of panels) {
      const p = def.defaultPlacement;
      if (!p || p.kind === "grid") continue;
      if (p.kind === "drawer" && !where(def.id)) enter(def.id, p);
      if (p.kind === "float" && (where(def.id)?.kind ?? "grid") === "grid") enter(def.id, p);
    }
  };

  const clearDrawers = (): void => {
    for (const d of drawers.values()) {
      for (const id of d.ids) leave(id);
      d.setOpen(false, false);
      d.setSize(opts.drawers?.[d.edge]?.size ?? DRAWER_DEFAULT, false);
    }
  };

  const restoreDrawers = (saved: SavedDrawers): void => {
    for (const edge of EDGES) {
      const s = saved[edge];
      if (!s) continue;
      const d = drawer(edge);
      if (typeof s.size === "number" && s.size > 0) d.setSize(s.size, false);
      for (const id of Array.isArray(s.panels) ? s.panels : []) {
        const def = defOf(id);
        if (def && !where(id)) d.add(id, def.title, hostFor(id));
      }
      if (s.active) d.activate(s.active, false);
      // A drawer is somewhere a reader visits and leaves, so a fresh page starts with it closed.
      if (s.open && opts.drawers?.[edge]?.restoreOpen) d.setOpen(true, false);
    }
  };

  const restore = (): void => {
    const saved = storage ? loadLayout(storage, key) : null;
    if (!saved) {
      applyDefault();
      return;
    }
    try {
      api.fromJSON(saved.layout as Parameters<DockviewApi["fromJSON"]>[0]);
      for (const panel of [...api.panels]) if (!defOf(panel.id)) api.removePanel(panel);
      restoreDrawers(saved.drawers);
      for (const id of panelsToReconcile(saved.savedPanels, panels)) {
        if (!where(id)) enter(id, byId.get(id)!.defaultPlacement ?? { kind: "grid" });
      }
    } catch (err) {
      console.warn("tsappkit-dock: saved layout rejected, using the default", err);
      storage?.removeItem(key);
      clearDrawers();
      api.clear();
      applyDefault();
    }
  };

  const save = (): void => {
    if (quiet || disposed || !storage || !laidOut) return;
    saveLayout(storage, key, panels, api.toJSON(), drawerStates());
  };

  // What `moved` compares against: each panel's last place, element and document.
  const seen = new Map<string, { at: PanelLocation | null; el?: HTMLElement; doc?: Document }>();
  const key0 = (p: PanelLocation | null) => JSON.stringify(p);
  // What `drawerChanged` compares against: each drawer's last "open active".
  const drawerSeen = new Map<DrawerEdge, string>();

  // Compares every panel's place and document with the last ones seen, and calls `moved` for each
  // that differs. Running it after every change, whatever made it, catches moves made through
  // dock.api and by dockview itself as well as the dock's own.
  const sync = (notify = true): void => {
    if (disposed || quiet) return;
    const ids = new Set([...panels.map((p) => p.id), ...api.panels.map((p) => p.id), ...[...drawers.values()].flatMap((d) => d.ids), ...seen.keys()]);
    for (const id of ids) {
      if (!defOf(id)) continue;
      const at = where(id);
      const prev = seen.get(id);
      const el = at ? elementOf(id) : prev?.el;
      const doc = at && el ? el.ownerDocument : prev?.doc;
      seen.set(id, { at, el, doc });
      if (!notify || !opts.moved || !el) continue;
      if (key0(at) === key0(prev?.at ?? null) && doc === prev?.doc) continue;
      opts.moved(id, el, at, prev?.at ?? null, { documentChanged: !!prev?.doc && doc !== prev.doc });
    }
    for (const [edge, d] of drawers) {
      const now = `${d.open} ${d.active ?? ""}`;
      const before = drawerSeen.get(edge) ?? "false ";
      drawerSeen.set(edge, now);
      if (notify && now !== before) opts.drawerChanged?.(edge, d.open, d.active);
    }
  };

  const changed = (): void => {
    save();
    sync();
  };

  const reset = (): void => {
    quiet++;
    try {
      storage?.removeItem(key);
      clearDrawers();
      api.clear();
      applyDefault();
    } finally {
      quiet--;
    }
    changed();
  };

  // dockview sizes its grid from a ResizeObserver a frame after it's built, and reports 100 by 100
  // until then. A save in that window writes the groups squashed into 100 px, and the next visit
  // restores an even split (#148). So the dock lays dockview out from the container before
  // restoring, and while the container has no size (hidden, say) it holds every save until it has.
  const layOut = (): boolean => {
    const w = container.clientWidth;
    const h = container.clientHeight;
    if (w <= 0 || h <= 0) return false;
    api.layout(w, h);
    return true;
  };
  let laidOut = layOut();
  if (!laidOut) {
    const ro = new ResizeObserver(() => {
      if (disposed || !layOut()) return;
      laidOut = true;
      ro.disconnect();
      save();
    });
    ro.observe(container);
    cleanups.push(() => ro.disconnect());
  }

  quiet++;
  try {
    restore();
  } finally {
    quiet--;
  }
  // Save what restore built, so the save names today's registry and a rejected save is replaced,
  // and note where everything starts without calling moved for it.
  save();
  sync(false);
  const sub = api.onDidLayoutChange(() => changed());
  cleanups.push(() => sub.dispose());

  // ---- theme ----

  const applyTheme = (): void => component.updateOptions({ theme: theme() === "dark" ? themeDark : themeLight });
  const view = doc.defaultView;
  if (view?.MutationObserver) {
    const mo = new view.MutationObserver(applyTheme);
    mo.observe(doc.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    cleanups.push(() => mo.disconnect());
  }
  const mq = view?.matchMedia?.("(prefers-color-scheme: dark)");
  if (mq?.addEventListener) {
    mq.addEventListener("change", applyTheme);
    cleanups.push(() => mq.removeEventListener("change", applyTheme));
  }

  const escape = (e: KeyboardEvent): void => {
    if (e.key !== "Escape") return;
    closeMenu();
    for (const d of drawers.values()) d.setOpen(false);
  };
  doc.addEventListener("keydown", escape);
  cleanups.push(() => doc.removeEventListener("keydown", escape));

  // ---- menus ----

  let menu: HTMLElement | null = null;
  const closeMenu = (): void => {
    menu?.remove();
    menu = null;
  };
  const outside = (e: Event): void => {
    if (menu && !menu.contains(e.target as Node)) closeMenu();
  };
  doc.addEventListener("pointerdown", outside, true);
  cleanups.push(() => doc.removeEventListener("pointerdown", outside, true), closeMenu);

  const moveItems = (id: string): [string, Placement | null][] => {
    const now = where(id);
    const items: [string, Placement | null][] = [];
    if (now?.kind !== "grid") items.push(["Dock", { kind: "grid" }]);
    if (now?.kind !== "float") items.push(["Float", { kind: "float" }]);
    for (const edge of EDGES) {
      if (!(now?.kind === "drawer" && now.edge === edge)) items.push([`Move to ${edge} drawer`, { kind: "drawer", edge }]);
    }
    items.push(["Close", null]);
    return items;
  };

  // The ⋯ menu for one panel: Dock, Float, the four drawers and Close, leaving out where it is.
  const moveMenu = (anchor: HTMLElement, id: string): void => {
    closeMenu();
    const list = doc.createElement("div");
    list.className = "dock-menu";
    list.setAttribute("role", "menu");
    list.dataset.panel = id;
    const r = anchor.getBoundingClientRect();
    Object.assign(list.style, { position: "fixed", left: `${Math.round(r.left)}px`, top: `${Math.round(r.bottom + 2)}px`, zIndex: "calc(var(--dv-overlay-z-index, 999) + 300)" });
    for (const [label, target] of moveItems(id)) {
      const b = doc.createElement("button");
      b.type = "button";
      b.className = "dock-menu-item";
      b.setAttribute("role", "menuitem");
      b.textContent = label;
      b.addEventListener("click", () => {
        closeMenu();
        if (target) place(id, target);
        else toggle(id);
      });
      list.appendChild(b);
    }
    doc.body.appendChild(list);
    menu = list;
  };

  function headerMenu(): IHeaderActionsRenderer {
    const btn = doc.createElement("button");
    btn.type = "button";
    btn.className = "dock-move-btn";
    btn.textContent = "⋯";
    btn.title = "Move this panel";
    let group: { activePanel?: { id: string } } | undefined;
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      const id = group?.activePanel?.id;
      if (id && defOf(id)) moveMenu(btn, id);
    });
    return {
      element: btn,
      init: (params) => {
        group = params.group as unknown as { activePanel?: { id: string } };
      },
      dispose: () => btn.remove(),
    };
  }

  const panelsMenu = (host: HTMLElement): (() => void) => {
    const btn = doc.createElement("button");
    btn.type = "button";
    btn.className = "dock-panels-btn";
    btn.textContent = "Panels ▾";
    btn.setAttribute("aria-haspopup", "true");
    btn.setAttribute("aria-expanded", "false");
    const list = doc.createElement("div");
    list.className = "dock-panels-list";
    list.style.display = "none";
    const setOpen = (show: boolean): void => {
      list.style.display = show ? "block" : "none";
      btn.setAttribute("aria-expanded", String(show));
    };
    const item = (label: string, cls: string, act: () => void): HTMLButtonElement => {
      const b = doc.createElement("button");
      b.type = "button";
      b.className = cls;
      b.textContent = label;
      b.addEventListener("click", () => {
        act();
        setOpen(false);
      });
      return b;
    };
    const rebuild = (): void => {
      const open = new Set([...api.panels.map((p) => p.id), ...[...drawers.values()].flatMap((d) => d.ids)]);
      const listed = [...panels, ...[...open].filter((id) => !byId.has(id)).map((id) => defOf(id)).filter((d): d is PanelDef => !!d)];
      list.replaceChildren(
        ...listed.map((def) => {
          const at = where(def.id);
          return item(`${at ? "✓" : "  "} ${def.title}${placementLabel(at)}`, "dock-panels-item", () => toggle(def.id));
        }),
        item("  Reset layout", "dock-panels-item dock-panels-reset", reset),
      );
    };
    btn.addEventListener("click", () => {
      const show = list.style.display === "none";
      if (show) rebuild();
      setOpen(show);
    });
    const close = (e: Event): void => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !host.contains(e.target as Node)) setOpen(false);
    };
    doc.addEventListener("click", close);
    doc.addEventListener("keydown", close);
    host.append(btn, list);
    const remove = (): void => {
      doc.removeEventListener("click", close);
      doc.removeEventListener("keydown", close);
      btn.remove();
      list.remove();
    };
    cleanups.push(remove);
    return remove;
  };

  return {
    api,
    show,
    place,
    where,
    toggle,
    toggleDrawer(edge, open) {
      const d = drawers.get(edge);
      if (!d) return false;
      d.setOpen(open ?? !d.open);
      return d.open;
    },
    reset,
    applyTheme,
    panelsMenu,
    dispose() {
      if (disposed) return;
      // dockview reports layout changes a microtask late, so save now or lose the last change.
      save();
      disposed = true;
      for (const c of cleanups.splice(0)) c();
      for (const d of drawers.values()) {
        for (const id of d.ids) {
          const el = d.remove(id);
          if (el) release?.(id, el);
        }
        d.dispose();
      }
      component.dispose();
    },
  };
}
