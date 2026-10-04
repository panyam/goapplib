/**
 * The page spec: which islands a page mounts, where, and with what config.
 * The server writes it into the page as JSON (goapplib's page.Spec, through
 * templates/page/Islands.html); this reads it back.
 */

/** The id of the script element the server writes the spec into. */
export const SPEC_ELEMENT_ID = "page-spec";

/** One island: `name` picks its factory, `slot` the element it mounts in (`[data-slot]`). */
export interface IslandSpec {
  name: string;
  slot: string;
  /** How the layout shows it (the app's own words: "page", "panel", "drawer"), when it draws differently in each. */
  presentation?: string;
  /** Handed to the factory as is. Always an object. */
  config: Record<string, unknown>;
  /**
   * When it mounts: `eager` (also when absent), `idle`, `visible` or
   * `media:<query>` (see parseLoad). IslandPage waits for it; mountIslands
   * does when given a `defer`.
   */
  load?: string;
}

export interface PageSpec {
  /** The arrangement of slots, for state kept per layout. */
  layout: string;
  islands: IslandSpec[];
}

/**
 * Reads an app's own fields from the parsed spec, the ones its Go type adds
 * by embedding page.Spec. Gets the whole object; returns what the app wants
 * kept, already checked.
 */
export type SpecExtension<Ext> = (raw: Record<string, unknown>) => Ext;

// Slot names go into an attribute selector, so they must stay plain (as Go checks).
const SLOT = /^[a-z][a-z0-9-]*$/;

/**
 * The spec in `text`, or null when there is none or it isn't one. Islands
 * without a name or with a slot name that isn't plain are dropped, and a
 * config that isn't an object becomes {}, so what comes back can be mounted.
 *
 * `extend`, when given, reads the app's own fields; they're merged into the
 * result, but `layout` and `islands` always come from this reading. If it
 * throws, the spec is unreadable and the result is null.
 */
export function readSpec(text: string | null | undefined): PageSpec | null;
export function readSpec<Ext extends object>(text: string | null | undefined, extend: SpecExtension<Ext>): (PageSpec & Ext) | null;
export function readSpec<Ext extends object>(text: string | null | undefined, extend?: SpecExtension<Ext>): PageSpec | (PageSpec & Ext) | null {
  if (!text) return null;
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return null;
  }
  if (!isObject(raw) || typeof raw.layout !== "string" || !Array.isArray(raw.islands)) return null;
  const islands: IslandSpec[] = [];
  for (const is of raw.islands) {
    if (!isObject(is) || typeof is.name !== "string" || !is.name || typeof is.slot !== "string" || !SLOT.test(is.slot)) continue;
    islands.push({
      name: is.name,
      slot: is.slot,
      ...(typeof is.presentation === "string" && { presentation: is.presentation }),
      config: isObject(is.config) ? is.config : {},
      ...(typeof is.load === "string" && { load: is.load }),
    });
  }
  const spec: PageSpec = { layout: raw.layout, islands };
  if (!extend) return spec;
  let ext: Ext;
  try {
    ext = extend(raw);
  } catch {
    return null;
  }
  return { ...ext, ...spec };
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}
