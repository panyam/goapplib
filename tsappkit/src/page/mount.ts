import type { IslandSpec, PageSpec } from "./spec";

/**
 * Builds one island in `el`. `ctx` is the page's shared services, whatever
 * the app says they are; `bus` is the page's event bus.
 */
export type IslandFactory<Ctx, El, C, B> = (el: El, island: IslandSpec, ctx: Ctx, bus: B) => C;

/** The islands an entry can mount, by name. An entry bundles only what its registry names. */
export type Registry<Ctx, El, C, B> = Record<string, IslandFactory<Ctx, El, C, B>>;

/**
 * Mounts every island in `spec` into the element `findSlot` gives for its
 * slot, and returns what the factories built, in spec order. `context` builds
 * the page's shared services; it's called once, before the first island
 * mounts, and not at all on a page with nothing to mount, so a page without
 * islands doesn't start what they'd share. An island the registry doesn't
 * know, a slot that isn't on the page, or a factory that throws is reported
 * through `log` and skipped, so one bad entry doesn't take the rest of the
 * page down with it.
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
    try {
      ctx ??= context();
      out.push(factory(el, island, ctx, bus));
    } catch (err) {
      log(`page spec: island "${island.name}" failed to mount: ${err instanceof Error ? err.message : String(err)}`);
    }
  }
  return out;
}
