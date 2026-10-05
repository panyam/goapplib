import { parseLoad } from "./load";
import type { IslandSpec, PageSpec } from "./spec";

/**
 * Builds one island in `el`. `ctx` is the page's shared services, whatever
 * the app says they are; `bus` is the page's event bus.
 */
export type IslandFactory<Ctx, El, C, B> = (el: El, island: IslandSpec, ctx: Ctx, bus: B) => C;

const LAZY: unique symbol = Symbol("lazy island");

/** A registry entry whose factory is loaded when the island mounts. Made by lazy. */
export interface LazyIsland<Ctx, El, C, B> {
  readonly [LAZY]: () => Promise<IslandFactory<Ctx, El, C, B> | { default: IslandFactory<Ctx, El, C, B> }>;
}

/**
 * A registry entry that loads its island's module only when the island
 * mounts: `hero: lazy(() => import("./islands/hero"))`. With esbuild's
 * --splitting each such module is its own chunk, so a page downloads only the
 * islands its spec names, each when its `load` says. `load` resolves to the
 * factory or to a module whose default export is the factory.
 *
 * A lazy island always mounts late, even an eager one, since its chunk
 * arrives after the page has started; goapplib's page.Assets writes
 * modulepreload links for the eager ones so that wait is short.
 */
export function lazy<Ctx, El, C, B>(
  load: () => Promise<IslandFactory<Ctx, El, C, B> | { default: IslandFactory<Ctx, El, C, B> }>,
): LazyIsland<Ctx, El, C, B> {
  return { [LAZY]: load };
}

/**
 * The islands an entry can mount, by name: a factory, bundled with the entry,
 * or a lazy entry, loaded as its own chunk when the island mounts.
 */
export type Registry<Ctx, El, C, B> = Record<string, IslandFactory<Ctx, El, C, B> | LazyIsland<Ctx, El, C, B>>;

/** How mountIslands handles islands that shouldn't mount at once. */
export interface MountOptions<El, C> {
  /**
   * Gets each island whose `load` isn't eager, with the slot it will mount
   * in; call `mount` when it's time (IslandPage passes scheduleMount). An
   * island with a `load` parseLoad doesn't know is logged and mounted at once
   * instead, so it still shows up.
   */
  defer?: (island: IslandSpec, el: El, mount: () => void) => void;
  /** Gets what the factory built for each island mounted later: through `defer`, or from a lazy entry. */
  onLateMount?: (component: C, island: IslandSpec) => void;
  /**
   * Gets every island that mounts, eager or late, with its slot, as it
   * mounts (before onLateMount for a late one). IslandPage's debug overlay
   * uses it to show when each island arrived.
   */
  onMount?: (component: C, island: IslandSpec, el: El) => void;
  /**
   * Gets every island that won't mount, with a short reason ("not in the
   * registry", "no slot", "failed to load", "no factory", "factory threw"),
   * alongside the message `log` gets.
   */
  onSkip?: (island: IslandSpec, reason: string) => void;
}

/**
 * Mounts every island in `spec` into the element `findSlot` gives for its
 * slot, and returns what the factories built at once, in spec order. With
 * `options.defer`, an island whose `load` isn't eager is handed to it instead
 * and reported through `options.onLateMount` when it mounts; without it,
 * every island mounts now whatever its `load` says. A lazy entry is loaded
 * when its island would mount and is reported through `onLateMount` too, so
 * it's never in the returned list.
 *
 * `context` builds the page's shared services; it's called once, before the
 * first island mounts (eager or late), and not at all on a page with nothing
 * to mount, so a page without islands doesn't start what they'd share. An
 * island the registry doesn't know, a slot that isn't on the page, or a
 * factory that throws (now or later), or a lazy entry that fails to load, is
 * reported through `log` and skipped,
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
  const skip = (island: IslandSpec, reason: string, message: string) => {
    log(message);
    options.onSkip?.(island, reason);
  };
  let ctx: Ctx | undefined;
  for (const island of spec.islands) {
    const entry = Object.prototype.hasOwnProperty.call(registry, island.name) ? registry[island.name] : undefined;
    if (!entry) {
      skip(island, "not in the registry", `page spec: no island called "${island.name}" in this page's registry`);
      continue;
    }
    const el = findSlot(island.slot);
    if (el === null) {
      skip(island, "no slot", `page spec: island "${island.name}" wants slot "${island.slot}", which isn't on the page`);
      continue;
    }
    const build = (factory: IslandFactory<Ctx, El, C, B>): C | undefined => {
      try {
        ctx ??= context();
        return factory(el, island, ctx, bus);
      } catch (err) {
        skip(island, "factory threw", `page spec: island "${island.name}" failed to mount: ${message(err)}`);
        return undefined;
      }
    };
    const mountLate = () => {
      const late = (factory: IslandFactory<Ctx, El, C, B>) => {
        const c = build(factory);
        if (c === undefined) return;
        options.onMount?.(c, island, el);
        options.onLateMount?.(c, island);
      };
      if (typeof entry === "function") {
        late(entry);
        return;
      }
      // The executor turns a loader that throws into a rejection, logged like any other.
      new Promise<Awaited<ReturnType<(typeof entry)[typeof LAZY]>>>((resolve) => resolve(entry[LAZY]())).then(
        (m) => {
          const factory = typeof m === "function" ? m : m?.default;
          if (typeof factory === "function") late(factory);
          else skip(island, "no factory", `page spec: island "${island.name}" loaded, but its module has no factory (a default export or the function itself)`);
        },
        (err) => skip(island, "failed to load", `page spec: island "${island.name}" failed to load: ${message(err)}`),
      );
    };
    const strategy = parseLoad(island.load);
    if (strategy === null) {
      log(`page spec: island "${island.name}" has load "${island.load}", which isn't eager, idle, visible or media:<query>; mounting it now`);
    }
    if (options.defer && strategy !== null && strategy.kind !== "eager") {
      options.defer(island, el, mountLate);
      continue;
    }
    if (typeof entry !== "function") {
      mountLate();
      continue;
    }
    const c = build(entry);
    if (c !== undefined) {
      out.push(c);
      options.onMount?.(c, island, el);
    }
  }
  return out;
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
