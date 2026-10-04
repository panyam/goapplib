import type { IslandSpec } from "./spec";

const STYLE_ID = "island-overlay-style";

// The label is drawn by ::after from an attribute, so the overlay adds no
// children to a slot the island owns, and an island clearing its slot on
// mount (SolidIsland does) doesn't take the label with it.
const CSS = `
[data-island-debug] { position: relative; outline: 2px dashed #1565c0; outline-offset: -2px; }
[data-island-debug]::after {
  content: attr(data-island-debug); position: absolute; top: 0; right: 0; z-index: 2147483647;
  font: 11px/1.4 ui-monospace, monospace; padding: 1px 6px; background: #1565c0; color: #fff; pointer-events: none;
}
[data-island-state="mounted"] { outline-color: #2e7d32; }
[data-island-state="mounted"]::after { background: #2e7d32; }
[data-island-state="failed"] { outline-color: #c62828; }
[data-island-state="failed"]::after { background: #c62828; }
`;

/**
 * Outlines each island's slot and labels it with its name, slot, load
 * strategy and state: `hero · top · eager · mounted 212 ms`,
 * `below · bottom · visible · waiting`, or `ghost · foot · eager · not in the
 * registry`. Times are from navigation start (performance.now()).
 *
 * IslandPage turns it on with `?islands` in the URL (see
 * showIslandOverlay). It's a development aid: while it's on, every labelled
 * slot is `position: relative`, which can move an island's absolutely
 * positioned content.
 */
export class IslandOverlay {
  constructor(
    private readonly doc: Document = document,
    private readonly now: () => number = () => performance.now(),
  ) {
    if (!doc.getElementById(STYLE_ID)) {
      const style = doc.createElement("style");
      style.id = STYLE_ID;
      style.textContent = CSS;
      doc.head.appendChild(style);
    }
  }

  /** The island has a slot and hasn't mounted yet. */
  waiting(island: IslandSpec, el: HTMLElement): void {
    this.label(island, el, "waiting", "waiting");
  }

  /** The island has just mounted. */
  mounted(island: IslandSpec, el: HTMLElement): void {
    this.label(island, el, "mounted", `mounted ${Math.round(this.now())} ms`);
  }

  /** The island won't mount, for `reason`. */
  failed(island: IslandSpec, el: HTMLElement, reason: string): void {
    this.label(island, el, "failed", reason);
  }

  private label(island: IslandSpec, el: HTMLElement, state: string, text: string): void {
    el.dataset.islandState = state;
    el.dataset.islandDebug = [island.name, island.slot, island.load || "eager", text].join(" · ");
  }
}
