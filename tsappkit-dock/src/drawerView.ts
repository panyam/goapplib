// The drawer element: one per edge, laid over the dock with a scrim behind it. It holds the panels
// placed in it as tabs, and dock.ts moves their elements in and out. Styles are inline and minimal
// (position, size, the scrim), so a drawer works with no CSS; everything else hangs off the
// dock-drawer-* classes for the app to style.
import { closesOnRelease, drawerPixels, DRAWER_DEFAULT, DRAWER_MIN, type DrawerBounds, type DrawerEdge } from "./drawer.js";

/** What one drawer saves: its panels in tab order, the active one, its size, and whether it's open. */
export interface DrawerState {
  panels: string[];
  active?: string;
  size?: number;
  open?: boolean;
}

/** Per-edge drawer settings for `createDock`. */
export interface DrawerConfig extends DrawerBounds {
  /**
   * The size when nothing is saved. Over 1 it's px. Up to 1 it's a share of the drawer host on
   * the drawer's axis, so `0.65` makes a bottom sheet 65 percent of the dock's height. Default
   * `DRAWER_DEFAULT`.
   */
  size?: number;
  /**
   * Whether the drawer shows its tab strip. Default true. Turn it off when the app picks the panel
   * with its own buttons, such as a phone page's bottom bar calling `dock.show(id)`. The panels are
   * still tabs underneath, and the drawer still drags from its edge.
   */
  tabs?: boolean;
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
// +1 when the anchored edge is at the far end of its axis (right, bottom), so a pointer moving
// toward it moves in the positive direction.
const TOWARD: Record<DrawerEdge, 1 | -1> = { left: -1, right: 1, top: -1, bottom: 1 };
// A tab-strip press becomes a drag once it has moved this far, so a tap still picks a tab.
const DRAG_SLOP = 6;
// Only the last move counts toward a flick, and only if it came this recently before the release.
const FLICK_WINDOW_MS = 100;

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
    Object.assign(this.tabStrip.style, { display: config.tabs === false ? "none" : "flex", gap: "2px", alignItems: "center", flex: "none" });

    this.body = doc.createElement("div");
    this.body.className = "dock-drawer-body";
    Object.assign(this.body.style, { flex: "1", minHeight: "0", overflow: "auto", position: "relative" });

    this.handle = doc.createElement("div");
    this.handle.className = "dock-drawer-handle";
    const inner = { left: "right", right: "left", top: "bottom", bottom: "top" }[edge];
    // A finger needs more than the 6px a mouse gets.
    const thick = doc.defaultView?.matchMedia?.("(pointer: coarse)").matches ? 24 : 6;
    Object.assign(this.handle.style, {
      position: "absolute",
      [inner]: `-${thick / 2}px`,
      ...(HORIZONTAL[edge] ? { top: "0", bottom: "0", width: `${thick}px`, cursor: "col-resize" } : { left: "0", right: "0", height: `${thick}px`, cursor: "row-resize" }),
      touchAction: "none",
    });
    this.handle.addEventListener("pointerdown", (e) => this.drag(e, 0));
    // A bottom drawer's tab strip runs along its inner edge, so it drags like the handle, the way a
    // phone sheet drags from its header.
    if (edge === "bottom") {
      this.tabStrip.style.touchAction = "none";
      this.tabStrip.addEventListener("pointerdown", (e) => this.drag(e, DRAG_SLOP));
    }

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

  /** The size as configured, saved or dragged: px, or a share of the host when it's up to 1. */
  get size(): number {
    return this.px;
  }

  /** The size in px against the host as it is now, or unclamped while the host has no size. */
  get pixels(): number {
    const span = this.span();
    return span > 0 ? drawerPixels(this.px, span, this.config) : this.px;
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

  setSize(size: number, notify = true): void {
    this.px = size > 1 ? Math.round(size) : size;
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

  private span(): number {
    const rect = this.host.getBoundingClientRect();
    return HORIZONTAL[this.edge] ? rect.width : rect.height;
  }

  // Clamps against the host as it is now, so a drawer saved on a big window still fits a small one.
  // A host with no size yet (not laid out) gets a share as a percentage and px unclamped.
  private applySize(): void {
    const span = this.span();
    const css = span > 0 ? `${drawerPixels(this.px, span, this.config)}px` : this.px <= 1 ? `${this.px * 100}%` : `${this.px}px`;
    this.root.style[HORIZONTAL[this.edge] ? "width" : "height"] = css;
  }

  // A drag on the inner edge. It resizes while the edge is above `min`. Below it the drawer slides
  // toward its edge under the pointer, and letting go there, or flicking a finger toward the edge,
  // closes it at the size it had before the drag. `slop` is how far the pointer moves before this
  // counts as a drag, so a tap on a tab in the strip still picks the tab.
  private drag(e: PointerEvent, slop: number): void {
    if (e.button > 0) return;
    const doc = this.host.ownerDocument;
    const horizontal = HORIZONTAL[this.edge];
    const toward = TOWARD[this.edge];
    // Only a finger or a pen flicks. A synthetic or unknown pointer counts as a mouse.
    const touch = e.pointerType === "touch" || e.pointerType === "pen";
    const min = this.config.min ?? DRAWER_MIN;
    const at = (ev: PointerEvent) => (horizontal ? ev.clientX : ev.clientY);
    const start = at(e);
    const before = this.px;
    let dragging = false;
    let fromEdge = Number.NaN;
    let last = { pos: start, t: e.timeStamp };
    let speed = 0;

    const begin = (): void => {
      dragging = true;
      this.root.classList.add("dock-drawer-resizing");
    };
    if (slop === 0) {
      e.preventDefault();
      begin();
    }
    const move = (ev: PointerEvent): void => {
      const pos = at(ev);
      if (!dragging) {
        if (Math.abs(pos - start) < slop) return;
        begin();
      }
      ev.preventDefault();
      const rect = this.host.getBoundingClientRect();
      const span = horizontal ? rect.width : rect.height;
      const pointer = pos - (horizontal ? rect.left : rect.top);
      fromEdge = toward > 0 ? span - pointer : pointer;
      const dt = ev.timeStamp - last.t;
      if (dt > 0) speed = (toward * (pos - last.pos)) / dt;
      last = { pos, t: ev.timeStamp };
      const px = drawerPixels(fromEdge, span, this.config);
      this.setSize(px, false);
      const slide = Math.max(0, px - fromEdge);
      this.root.style.transform = slide > 0 ? `translate${horizontal ? "X" : "Y"}(${toward * slide}px)` : "";
    };
    const end = (ev: PointerEvent): void => {
      doc.removeEventListener("pointermove", move);
      doc.removeEventListener("pointerup", end);
      doc.removeEventListener("pointercancel", end);
      if (!dragging) return;
      this.root.classList.remove("dock-drawer-resizing");
      this.root.style.transform = "";
      // The click that follows a drag on the tab strip would otherwise pick a tab.
      if (slop > 0) {
        const swallow = (c: Event): void => {
          c.stopPropagation();
          c.preventDefault();
        };
        this.tabStrip.addEventListener("click", swallow, { capture: true, once: true });
        setTimeout(() => this.tabStrip.removeEventListener("click", swallow, { capture: true }), 0);
      }
      const fresh = ev.timeStamp - last.t <= FLICK_WINDOW_MS;
      if (ev.type === "pointerup" && Number.isFinite(fromEdge) && closesOnRelease(fromEdge, min, touch && fresh ? speed : 0)) {
        this.setSize(before, false);
        this.setOpen(false);
        return;
      }
      this.hooks.changed();
    };
    doc.addEventListener("pointermove", move);
    doc.addEventListener("pointerup", end);
    doc.addEventListener("pointercancel", end);
  }
}
