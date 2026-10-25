// The tour's DOM: a dimmed layer with holes cut by an even-odd clip-path, a ring round each target,
// and the step's panel. A clipped-away area isn't hit-tested, so input in a hole reaches the element
// under it untouched (hover, focus, drag), and input anywhere else lands on the layer and stops there.
import { holePath, placePanel, type Rect, unionRects } from "./geometry";

/** The panel's button labels, for an app that isn't in English. */
export interface TourLabels {
  next: string;
  done: string;
  showMe: string;
  close: string;
}

export const DEFAULT_LABELS: TourLabels = { next: "Next", done: "Done", showMe: "Show me", close: "End tour" };

/** What the panel shows for one step. */
export interface PanelView {
  index: number;
  count: number;
  title?: string;
  text: string;
  next: boolean;
  showMe: boolean;
}

const STYLE_ATTR = "data-tsappkit-tour-style";

// Every color and size is a custom property with a fallback, so an app themes the tour by setting
// them on :root (or on .tsappkit-tour) without fighting this sheet's specificity.
const CSS = `
.tsappkit-tour { position: fixed; inset: 0; z-index: var(--tsappkit-tour-z, 2147483000); pointer-events: none; }
.tsappkit-tour-dim { position: absolute; inset: 0; pointer-events: auto; background: var(--tsappkit-tour-dim, rgba(2, 6, 23, .6)); }
.tsappkit-tour-ring { position: absolute; pointer-events: none; border-radius: var(--tsappkit-tour-ring-radius, 6px);
  box-shadow: 0 0 0 2px var(--tsappkit-tour-accent, #3b82f6); }
.tsappkit-tour-panel { position: absolute; pointer-events: auto; box-sizing: border-box;
  width: min(var(--tsappkit-tour-panel-width, 360px), calc(100vw - 32px)); padding: 12px 16px 14px;
  border-radius: var(--tsappkit-tour-panel-radius, 10px); font: var(--tsappkit-tour-font, 14px/1.5 system-ui, sans-serif);
  background: var(--tsappkit-tour-panel-bg, #ffffff); color: var(--tsappkit-tour-panel-fg, #0f172a);
  box-shadow: 0 12px 32px rgba(0, 0, 0, .35); }
.tsappkit-tour-head { display: flex; align-items: center; justify-content: space-between; gap: 8px;
  font-size: .85em; opacity: .75; }
.tsappkit-tour-close { all: unset; cursor: pointer; font-size: 1.4em; line-height: 1; padding: 0 4px; }
.tsappkit-tour-close:focus-visible, .tsappkit-tour-actions button:focus-visible {
  outline: 2px solid var(--tsappkit-tour-accent, #3b82f6); outline-offset: 2px; }
.tsappkit-tour-title { font-weight: 600; margin: 4px 0 0; }
.tsappkit-tour-text { margin: 4px 0 0; }
.tsappkit-tour-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; }
.tsappkit-tour-actions:empty { display: none; }
.tsappkit-tour-actions button { font: inherit; cursor: pointer; padding: 4px 12px; border-radius: 6px;
  border: 1px solid var(--tsappkit-tour-accent, #3b82f6); background: transparent; color: inherit; }
.tsappkit-tour-actions .tsappkit-tour-next { background: var(--tsappkit-tour-accent, #3b82f6); color: var(--tsappkit-tour-accent-fg, #ffffff); }
@media (prefers-color-scheme: dark) {
  .tsappkit-tour-panel { background: var(--tsappkit-tour-panel-bg, #1e293b); color: var(--tsappkit-tour-panel-fg, #e2e8f0); }
}
`;

// Pointer input that misses the holes stops at the overlay, so an app's document-level listeners
// (close-on-outside-click, say) don't see it either.
const SWALLOWED = ["pointerdown", "pointerup", "mousedown", "mouseup", "click", "dblclick", "contextmenu"];

export class TourOverlay {
  readonly root: HTMLElement;
  private readonly dim: HTMLElement;
  private readonly rings: HTMLElement;
  readonly panel: HTMLElement;
  private readonly count: HTMLElement;
  private readonly title: HTMLElement;
  private readonly text: HTMLElement;
  private readonly actions: HTMLElement;
  private key = "";

  constructor(
    private readonly doc: Document,
    private readonly labels: TourLabels,
    private readonly on: { close(): void; next(): void; showMe(): void },
  ) {
    if (!doc.head.querySelector(`style[${STYLE_ATTR}]`)) {
      const style = doc.createElement("style");
      style.setAttribute(STYLE_ATTR, "");
      style.textContent = CSS;
      doc.head.prepend(style);
    }
    const el = (tag: string, cls: string, parent?: HTMLElement) => {
      const e = doc.createElement(tag);
      e.className = cls;
      parent?.append(e);
      return e;
    };
    this.root = el("div", "tsappkit-tour");
    this.dim = el("div", "tsappkit-tour-dim", this.root);
    this.rings = el("div", "tsappkit-tour-rings", this.root);
    this.panel = el("div", "tsappkit-tour-panel", this.root);
    this.panel.setAttribute("role", "dialog");
    this.panel.setAttribute("aria-label", "Tour");
    const head = el("div", "tsappkit-tour-head", this.panel);
    this.count = el("span", "tsappkit-tour-count", head);
    const close = el("button", "tsappkit-tour-close", head) as HTMLButtonElement;
    close.type = "button";
    close.textContent = "×";
    close.setAttribute("aria-label", labels.close);
    close.addEventListener("click", () => this.on.close());
    this.title = el("div", "tsappkit-tour-title", this.panel);
    this.text = el("p", "tsappkit-tour-text", this.panel);
    this.text.setAttribute("aria-live", "polite");
    this.actions = el("div", "tsappkit-tour-actions", this.panel);

    for (const type of SWALLOWED) this.root.addEventListener(type, (e) => e.stopPropagation());
    // Scrolling outside the holes would move the page under a reader who can't otherwise reach it.
    const block = (e: Event) => e.preventDefault();
    this.dim.addEventListener("wheel", block, { passive: false });
    this.dim.addEventListener("touchmove", block, { passive: false });
    doc.body.append(this.root);
  }

  /** Puts a step's text and buttons in the panel. */
  show(step: string, view: PanelView): void {
    this.root.dataset.tourStep = step;
    this.count.textContent = `${view.index + 1} of ${view.count}`;
    this.title.textContent = view.title ?? "";
    this.title.hidden = !view.title;
    this.text.textContent = view.text;
    this.actions.replaceChildren();
    const button = (label: string, cls: string, click: () => void) => {
      const b = this.doc.createElement("button");
      b.type = "button";
      b.className = cls;
      b.textContent = label;
      b.addEventListener("click", click);
      this.actions.append(b);
    };
    if (view.showMe) button(this.labels.showMe, "tsappkit-tour-showme", () => this.on.showMe());
    if (view.next) {
      const last = view.index + 1 === view.count;
      button(last ? this.labels.done : this.labels.next, "tsappkit-tour-next", () => this.on.next());
    }
    this.key = "";
  }

  /**
   * Cuts one hole per rect (already padded, in viewport coordinates) and places the panel clear of
   * them. Cheap to call every frame: it only touches the DOM when the holes, the viewport or the
   * panel's size changed.
   */
  setHoles(rects: readonly Rect[]): void {
    const vw = this.doc.documentElement.clientWidth;
    const vh = this.doc.documentElement.clientHeight;
    const pw = this.panel.offsetWidth;
    const ph = this.panel.offsetHeight;
    const key = `${vw}x${vh} ${pw}x${ph} ` + rects.map((r) => `${r.x},${r.y},${r.width},${r.height}`).join(" ");
    if (key === this.key) return;
    this.key = key;

    const holes = unionRects(rects);
    this.dim.style.clipPath = `path(evenodd, "${holePath(vw, vh, holes)}")`;
    this.root.dataset.tourHoles = String(holes.length);
    // One ring per hole after the union, so padded rows that overlap get one outline, not one each.
    while (this.rings.children.length > holes.length) this.rings.lastElementChild!.remove();
    while (this.rings.children.length < holes.length) {
      const ring = this.doc.createElement("div");
      ring.className = "tsappkit-tour-ring";
      this.rings.append(ring);
    }
    holes.forEach((r, i) => {
      const s = (this.rings.children[i] as HTMLElement).style;
      s.left = `${r.x}px`;
      s.top = `${r.y}px`;
      s.width = `${r.width}px`;
      s.height = `${r.height}px`;
    });
    const at = placePanel(rects, vw, vh, pw, ph);
    this.panel.style.left = `${at.x}px`;
    this.panel.style.top = `${at.y}px`;
  }

  destroy(): void {
    this.root.remove();
  }
}
