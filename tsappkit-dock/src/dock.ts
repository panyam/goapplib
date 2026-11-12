import { DockviewComponent, themeDark, themeLight } from "dockview-core";
import type { DockviewApi, IContentRenderer } from "dockview-core";
import {
  documentTheme,
  loadLayout,
  panelsToPrune,
  panelsToReconcile,
  saveLayout,
  type DockStorage,
  type PanelDef,
  type PanelPosition,
  type PanelSpec,
} from "./layout.js";

/** What `createDock` needs. */
export interface DockOptions {
  /** The element dockview renders into. It needs a size, as dockview fills it. */
  container: HTMLElement;
  /** The panel registry, in a stable order. Only these ids get content. */
  panels: readonly PanelDef[];
  /**
   * The arrangement used when nothing is saved, after a rejected save, and on `reset`. Either
   * `addPanel` specs as data, or a function that builds it on the api, for an app that sizes
   * groups after adding them.
   */
  defaultLayout: readonly PanelSpec[] | ((api: DockviewApi) => void);
  /**
   * Returns the element a panel shows. The dock calls it each time the panel enters the grid, so
   * return the same element every time if its content should survive a close and reopen. It is
   * only called with registry ids.
   */
  hostFor: (id: string) => HTMLElement;
  /**
   * Called when a panel leaves the grid, with the element `hostFor` gave it. dockview has already
   * detached it, so this is where an app parks the element somewhere hidden, or tears its content
   * down. Clearing the layout, as `reset` does, calls it for every open panel.
   */
  release?: (id: string, el: HTMLElement) => void;
  /** Where the layout is saved, usually `localStorage`. Null or absent: nothing is saved. */
  storage?: DockStorage | null;
  /** The storage key. Required with `storage`. */
  key?: string;
  /**
   * Returns the dock's theme. It is read at creation, when `<html data-theme>` changes, when the
   * OS colour scheme changes, and on `applyTheme()`. Default: `documentTheme()`.
   */
  theme?: () => "light" | "dark";
}

/** A dock made by `createDock`. */
export interface Dock {
  /** The dockview api, for anything the dock doesn't wrap. */
  readonly api: DockviewApi;
  /**
   * Brings a panel to the front, opening it first if it's closed. It returns false, and does
   * nothing, for an id the registry lacks.
   */
  show(id: string): boolean;
  /** Opens a closed panel or closes an open one. False for an id the registry lacks. */
  toggle(id: string): boolean;
  /** Forgets the saved layout and applies the default one. Panels keep their `hostFor` elements. */
  reset(): void;
  /** Reads `theme` again and applies it. For a theme setting the dock doesn't watch. */
  applyTheme(): void;
  /**
   * Adds a Panels menu button to `host`. Its list has one item per registry panel, each toggling
   * its panel, with a ✓ on the open ones, and then Reset layout. Returns a function that removes
   * it. The elements carry `dock-panels-*` classes and no styles beyond showing and hiding the
   * list, so the app styles them.
   */
  panelsMenu(host: HTMLElement): () => void;
  /**
   * Saves the layout, stops watching the theme, removes any Panels menus, and disposes of
   * dockview. Changes are otherwise saved a microtask after dockview reports them. A second call
   * does nothing.
   */
  dispose(): void;
}

/**
 * Builds a dockview dock from a panel registry. It restores the saved layout when there is one,
 * opens the panels added to the registry since it was saved, and closes the ones the registry
 * dropped. A save that dockview rejects is removed and the default layout applied. Every layout
 * change after that is saved.
 *
 * Content lives outside dockview. `hostFor` hands an element to a panel when it enters the grid,
 * and `release` hands it back when it leaves, so an app can keep a panel's content alive while
 * the panel is closed.
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
  let disposed = false;

  const renderer = (id: string): IContentRenderer => {
    if (!byId.has(id)) return { element: doc.createElement("div"), init: () => {} };
    const element = hostFor(id);
    return { element, init: () => {}, dispose: () => release?.(id, element) };
  };

  const component = new DockviewComponent(container, {
    createComponent: (o) => renderer(o.id),
    // "always" keeps a background tab's content attached, only hidden. The default detaches it,
    // and an app that finds its content by id at startup would miss the panels behind a tab.
    defaultRenderer: "always",
    theme: theme() === "dark" ? themeDark : themeLight,
  });
  const api = component.api;

  const usable = (p?: PanelPosition): p is PanelPosition => !!p && (!p.referencePanel || !!api.getPanel(p.referencePanel));

  const open = (id: string): void => {
    const def = byId.get(id)!;
    const spec = specs?.find((s) => s.id === id);
    const position = [def.position, spec?.position].find(usable);
    api.addPanel({ id, component: id, title: def.title, ...(position ? { position } : {}) });
  };

  const applyDefault = (): void => {
    if (!specs) {
      (opts.defaultLayout as (api: DockviewApi) => void)(api);
      return;
    }
    for (const spec of specs) {
      const def = byId.get(spec.id);
      if (!def || api.getPanel(spec.id)) continue;
      const position = usable(spec.position) ? spec.position : undefined;
      api.addPanel({ id: spec.id, component: spec.id, title: spec.title ?? def.title, ...(position ? { position } : {}) });
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
      for (const id of panelsToPrune(api.panels.map((p) => p.id), panels)) {
        const panel = api.getPanel(id);
        if (panel) api.removePanel(panel);
      }
      for (const id of panelsToReconcile(saved.savedPanels, panels)) {
        if (!api.getPanel(id)) open(id);
      }
    } catch (err) {
      console.warn("tsappkit-dock: saved layout rejected, using the default", err);
      storage?.removeItem(key);
      api.clear();
      applyDefault();
    }
  };

  restore();
  if (storage) {
    // Save what restore built, so the save names today's registry and a rejected save is replaced.
    saveLayout(storage, key, panels, api.toJSON());
    const sub = api.onDidLayoutChange(() => saveLayout(storage, key, panels, api.toJSON()));
    cleanups.push(() => sub.dispose());
  }

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

  const reset = (): void => {
    storage?.removeItem(key);
    api.clear();
    applyDefault();
  };

  const toggle = (id: string): boolean => {
    if (!byId.has(id)) return false;
    const panel = api.getPanel(id);
    if (panel) api.removePanel(panel);
    else open(id);
    return true;
  };

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
      list.replaceChildren(
        ...panels.map((def) =>
          item(`${api.getPanel(def.id) ? "✓" : "  "} ${def.title}`, "dock-panels-item", () => toggle(def.id)),
        ),
        item("  Reset layout", "dock-panels-item dock-panels-reset", reset),
      );
    };
    btn.addEventListener("click", () => {
      const show = list.style.display === "none";
      if (show) rebuild();
      setOpen(show);
    });
    const outside = (e: Event): void => {
      if (!host.contains(e.target as Node)) setOpen(false);
    };
    const escape = (e: KeyboardEvent): void => {
      if (e.key === "Escape") setOpen(false);
    };
    doc.addEventListener("click", outside);
    doc.addEventListener("keydown", escape);
    host.append(btn, list);
    const remove = (): void => {
      doc.removeEventListener("click", outside);
      doc.removeEventListener("keydown", escape);
      btn.remove();
      list.remove();
    };
    cleanups.push(remove);
    return remove;
  };

  return {
    api,
    show(id) {
      if (!byId.has(id)) return false;
      if (!api.getPanel(id)) open(id);
      api.getPanel(id)?.api.setActive();
      return true;
    },
    toggle,
    reset,
    applyTheme,
    panelsMenu,
    dispose() {
      if (disposed) return;
      disposed = true;
      for (const c of cleanups.splice(0)) c();
      // dockview reports layout changes a microtask late, so save now or lose the last change.
      if (storage) saveLayout(storage, key, panels, api.toJSON());
      component.dispose();
    },
  };
}
