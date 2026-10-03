import { BasePage } from "../BasePage";
import type { EventBus } from "../EventBus";
import type { LCMComponent } from "../LCMComponent";
import { mountIslands, type Registry } from "./mount";
import { readSpec, SPEC_ELEMENT_ID, type PageSpec } from "./spec";

/**
 * A page whose islands come from the page spec. A subclass says which
 * islands it can mount (`registry`) and builds the services they share
 * (`makeContext`); the spec says which islands this page gets and where.
 *
 * `makeContext` is called once with the spec, before the first island
 * mounts, and not at all on a page with no islands to mount. An app whose
 * spec carries more than islands (its Go type embeds page.Spec) reads those
 * fields in `readExtension`, and they arrive typed as `Ext` on the spec
 * `makeContext` gets.
 *
 * A page with no readable `#page-spec` mounts nothing and warns. Subclasses
 * that override initializeSpecificComponents call super and add to what it
 * returns.
 */
export abstract class IslandPage<Ctx, Ext extends object = {}> extends BasePage {
  protected abstract registry(): Registry<Ctx, HTMLElement, LCMComponent, EventBus>;
  protected abstract makeContext(spec: PageSpec & Ext): Ctx;

  /**
   * The app's own fields from the parsed spec. The default reads none.
   * Throwing makes the spec unreadable, so nothing mounts.
   */
  protected readExtension(raw: Record<string, unknown>): Ext {
    return {} as Ext;
  }

  protected override initializeSpecificComponents(): LCMComponent[] {
    const spec = readSpec(document.getElementById(SPEC_ELEMENT_ID)?.textContent, (raw) => this.readExtension(raw));
    if (!spec) {
      console.warn(`page spec: no readable #${SPEC_ELEMENT_ID} on this page, so nothing is mounted`);
      return [];
    }
    return mountIslands(
      spec,
      this.registry(),
      (slot) => document.querySelector<HTMLElement>(`[data-slot="${slot}"]`),
      () => this.makeContext(spec),
      this.eventBus,
      (message) => console.warn(message),
    );
  }
}
