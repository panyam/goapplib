// The drawer element: one per edge, laid over the dock with a scrim behind it. It holds the panels
// placed in it as tabs, and dock.ts moves their elements in and out. Styles are inline and minimal
// (position, size, the scrim), so a drawer works with no CSS; everything else hangs off the
// dock-drawer-* classes for the app to style.
import { clampSize, drawerSize, DRAWER_DEFAULT, type DrawerBounds, type DrawerEdge } from "./drawer.js";

/** What one drawer saves: its panels in tab order, the active one, its size, and whether it's open. */
export interface DrawerState {
  panels: string[];
  active?: string;
  size?: number;
  open?: boolean;
}

/** Per-edge drawer settings for `createDock`. */
export interface DrawerConfig extends DrawerBounds {
  /** The size, in px, when nothing is saved. Default `DRAWER_DEFAULT`. */
  size?: number;
}

interface Tab {
  id: string;
  el: HTMLElement;
  button: HTMLButtonElement;
}

// dockview stacks floating groups and always-rendered panel content at --dv-overlay-z-index (999)
// and up to 100 above it, so a drawer and its scrim go above all of that.
const Z_SCRIM = "calc(var(--dv-overlay-z-index, 999) + 200)";
const Z_DRAWER = "calc(var(--dv-overlay-z-index, 999) + 201)";

const HORIZONTAL: Record<DrawerEdge, boolean> = { left: true, right: true, top: false, bottom: false };

export class DrawerView {
  readonly root: HTMLElement;
  readonly scrim: HTMLElement;
  readonly tabStrip: HTMLElement;
  readonly body: HTMLElement;
  readonly handle: HTMLElement;
  private tabs: Tab[] = [];
  private activeId?: string;
  private isOpen = false;
  private px: number;

  constructor(
    readonly edge: DrawerEdge,
    private readonly host: HTMLElement,
    private readonly config: DrawerConfig,
    private readonly hooks: {
      changed: () => void;
      menu: (anchor: HTMLElement, id: string) => void;
    },
  ) {
    const doc = host.ownerDocument;
    this.px = config.size ?? DRAWER_DEFAULT;

    this.scrim = doc.createElement("div");
    this.scrim.className = `dock-drawer-scrim dock-drawer-scrim-${edge}`;
    Object.assign(this.scrim.style, { position: "absolute", inset: "0", background: "rgba(0,0,0,.25)", zIndex: Z_SCRIM, display: "none" });
    this.scrim.addEventListener("click", () => this.setOpen(false));

    this.root = doc.createElement("aside");
    this.root.className = `dock-drawer dock-drawer-${edge}`;
    this.root.dataset.edge = edge;
    Object.assign(this.root.style, {
      position: "absolute",
      zIndex: Z_DRAWER,
      display: "none",
      flexDirection: "column",
      background: "var(--dv-group-view-background-color, Canvas)",
      color: "var(--dv-activegroup-visiblepanel-tab-color, CanvasText)",
      boxShadow: "0 0 16px rgba(0,0,0,.35)",
      [edge]: "0",
      ...(HORIZONTAL[edge] ? { top: "0", bottom: "0" } : { left: "0", right: "0" }),
    });

    this.tabStrip = doc.createElement("div");
    this.tabStrip.className = "dock-drawer-tabs";
    this.tabStrip.setAttribute("role", "tablist");
    Object.assign(this.tabStrip.style, { display: "flex", gap: "2px", alignItems: "center", flex: "none" });

    this.body = doc.createElement("div");
    this.body.className = "dock-drawer-body";
    Object.assign(this.body.style, { flex: "1", minHeight: "0", overflow: "auto", position: "relative" });

    this.handle = doc.createElement("div");
    this.handle.className = "dock-drawer-handle";
    const inner = { left: "right", right: "left", top: "bottom", bottom: "top" }[edge];
    Object.assign(this.handle.style, {
      position: "absolute",
      [inner]: "-3px",
      ...(HORIZONTAL[edge] ? { top: "0", bottom: "0", width: "6px", cursor: "col-resize" } : { left: "0", right: "0", height: "6px", cursor: "row-resize" }),
      touchAction: "none",
    });
    this.handle.addEventListener("pointerdown", (e) => this.drag(e));

    this.root.append(this.tabStrip, this.body, this.handle);
    host.append(this.scrim, this.root);
    this.applySize();
  }

  get ids(): string[] {
    return this.tabs.map((t) => t.id);
  }

  get active(): string | undefined {
    return this.activeId;
  }

  get open(): boolean {
    return this.isOpen;
  }

  get size(): number {
    return this.px;
  }

  /** The element a panel in this drawer shows, or undefined if it isn't here. */
  elementOf(id: string): HTMLElement | undefined {
    return this.tabs.find((t) => t.id === id)?.el;
  }

  has(id: string): boolean {
    return this.tabs.some((t) => t.id === id);
  }

  /** Adds a panel as a tab, or does nothing if it's already here. It doesn't open the drawer. */
  add(id: string, title: string, el: HTMLElement): void {
    if (this.has(id)) return;
    const doc = this.host.ownerDocument;
    const button = doc.createElement("button");
    button.type = "button";
    button.className = "dock-drawer-tab";
    button.setAttribute("role", "tab");
    button.dataset.panel = id;
    const label = doc.createElement("span");
    label.textContent = title;
    const more = doc.createElement("span");
    more.className = "dock-drawer-tab-menu";
    more.textContent = "⋯";
    more.title = "Move this panel";
    more.addEventListener("click", (e) => {
      e.stopPropagation();
      this.hooks.menu(more, id);
    });
    button.append(label, " ", more);
    button.addEventListener("click", () => this.activate(id));
    this.tabStrip.appendChild(button);
    el.style.display = "none";
    this.body.appendChild(el);
    this.tabs.push({ id, el, button });
    if (!this.activeId) this.activate(id, false);
    this.hooks.changed();
  }

  /** Takes a panel out and returns its element, detached. The next tab becomes active. */
  remove(id: string): HTMLElement | undefined {
    const i = this.tabs.findIndex((t) => t.id === id);
    if (i < 0) return undefined;
    const [tab] = this.tabs.splice(i, 1);
    tab.button.remove();
    tab.el.remove();
    tab.el.style.display = "";
    if (this.activeId === id) {
      this.activeId = undefined;
      const next = this.tabs[Math.min(i, this.tabs.length - 1)];
      if (next) this.activate(next.id, false);
    }
    if (this.tabs.length === 0) this.setOpen(false, false);
    this.hooks.changed();
    return tab.el;
  }

  activate(id: string, notify = true): void {
    if (!this.has(id)) return;
    this.activeId = id;
    for (const t of this.tabs) {
      const on = t.id === id;
      t.el.style.display = on ? "" : "none";
      t.button.classList.toggle("dock-drawer-tab-active", on);
      t.button.setAttribute("aria-selected", String(on));
    }
    if (notify) this.hooks.changed();
  }

  setOpen(open: boolean, notify = true): void {
    const show = open && this.tabs.length > 0;
    if (show === this.isOpen) return;
    this.isOpen = show;
    this.root.style.display = show ? "flex" : "none";
    this.scrim.style.display = show ? "block" : "none";
    if (show) this.applySize();
    if (notify) this.hooks.changed();
  }

  setSize(px: number, notify = true): void {
    this.px = Math.round(px);
    this.applySize();
    if (notify) this.hooks.changed();
  }

  state(): DrawerState {
    return { panels: this.ids, active: this.activeId, size: this.px, open: this.isOpen };
  }

  dispose(): void {
    this.scrim.remove();
    this.root.remove();
  }

  // Clamps against the host as it is now, so a drawer saved on a big window still fits a small one.
  private applySize(): void {
    const rect = this.host.getBoundingClientRect();
    const span = HORIZONTAL[this.edge] ? rect.width : rect.height;
    const px = span > 0 ? clampSize(this.px, span, this.config) : this.px;
    this.root.style[HORIZONTAL[this.edge] ? "width" : "height"] = `${px}px`;
  }

  private drag(e: PointerEvent): void {
    e.preventDefault();
    const move = (ev: PointerEvent): void => {
      const rect = this.host.getBoundingClientRect();
      const horizontal = HORIZONTAL[this.edge];
      const pointer = horizontal ? ev.clientX - rect.left : ev.clientY - rect.top;
      const span = horizontal ? rect.width : rect.height;
      this.setSize(drawerSize(this.edge, pointer, span, this.config), false);
    };
    const up = (): void => {
      this.handle.ownerDocument.removeEventListener("pointermove", move);
      this.handle.ownerDocument.removeEventListener("pointerup", up);
      this.root.classList.remove("dock-drawer-resizing");
      this.hooks.changed();
    };
    this.root.classList.add("dock-drawer-resizing");
    this.handle.ownerDocument.addEventListener("pointermove", move);
    this.handle.ownerDocument.addEventListener("pointerup", up);
  }
}
