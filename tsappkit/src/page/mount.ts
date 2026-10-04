import { parseLoad } from "./load";
import type { IslandSpec, PageSpec } from "./spec";

/**
 * Builds one island in `el`. `ctx` is the page's shared services, whatever
 * the app says they are; `bus` is the page's event bus.
 */
export type IslandFactory<Ctx, El, C, B> = (el: El, island: IslandSpec, ctx: Ctx, bus: B) => C;

/** The islands an entry can mount, by name. An entry bundles only what its registry names. */
export type Registry<Ctx, El, C, B> = Record<string, IslandFactory<Ctx, El, C, B>>;

/** How mountIslands handles islands that shouldn't mount at once. */
export interface MountOptions<El, C> {
  /**
   * Gets each island whose `load` isn't eager, with the slot it will mount
   * in; call `mount` when it's time (IslandPage passes scheduleMount). An
   * island with a `load` parseLoad doesn't know is logged and mounted at once
   * instead, so it still shows up.
   */
  defer?: (island: IslandSpec, el: El, mount: () => void) => void;
  /** Gets what the factory built for each island mounted later through `defer`. */
  onLateMount?: (component: C, island: IslandSpec) => void;
}

/**
 * Mounts every island in `spec` into the element `findSlot` gives for its
 * slot, and returns what the factories built at once, in spec order. With
 * `options.defer`, an island whose `load` isn't eager is handed to it instead
 * and reported through `options.onLateMount` when it mounts; without it,
 * every island mounts now whatever its `load` says.
 *
 * `context` builds the page's shared services; it's called once, before the
 * first island mounts (eager or late), and not at all on a page with nothing
 * to mount, so a page without islands doesn't start what they'd share. An
 * island the registry doesn't know, a slot that isn't on the page, or a
 * factory that throws (now or later) is reported through `log` and skipped,
 * so one bad entry doesn't take the rest of the page down with it.
 *
 * Plain types throughout (no DOM), so it runs under node in tests and on a
 * bare page without BasePage.
 */
export function mountIslands<Ctx, El, C, B>(
  spec: PageSpec,
  registry: Registry<Ctx, El, C, B>,
  findSlot: (slot: string) => El | null,
  context: () => Ctx,
  bus: B,
  log: (message: string) => void,
  options: MountOptions<El, C> = {},
): C[] {
  const out: C[] = [];
  let ctx: Ctx | undefined;
  for (const island of spec.islands) {
    const factory = Object.prototype.hasOwnProperty.call(registry, island.name) ? registry[island.name] : undefined;
    if (!factory) {
      log(`page spec: no island called "${island.name}" in this page's registry`);
      continue;
    }
    const el = findSlot(island.slot);
    if (el === null) {
      log(`page spec: island "${island.name}" wants slot "${island.slot}", which isn't on the page`);
      continue;
    }
    const build = (): C | undefined => {
      try {
        ctx ??= context();
        return factory(el, island, ctx, bus);
      } catch (err) {
        log(`page spec: island "${island.name}" failed to mount: ${err instanceof Error ? err.message : String(err)}`);
        return undefined;
      }
    };
    const strategy = parseLoad(island.load);
    if (strategy === null) {
      log(`page spec: island "${island.name}" has load "${island.load}", which isn't eager, idle, visible or media:<query>; mounting it now`);
    }
    if (options.defer && strategy !== null && strategy.kind !== "eager") {
      options.defer(island, el, () => {
        const c = build();
        if (c !== undefined) options.onLateMount?.(c, island);
      });
      continue;
    }
    const c = build();
    if (c !== undefined) out.push(c);
  }
  return out;
}
