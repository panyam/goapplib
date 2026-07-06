import { render } from 'solid-js/web';
import type { JSX } from 'solid-js';
import { BaseComponent, EventBus } from '@panyam/tsappkit';

/**
 * Mounts a Solid tree inside the tsappkit component lifecycle.
 *
 * The Solid root is created in `activate`, not the constructor, so the render
 * function can close over dependencies injected during `setupDependencies`
 * (e.g. a presenter). It is disposed in `deactivate`, which runs Solid's
 * `onCleanup` handlers and removes the inserted nodes.
 *
 * The island owns its `rootElement`'s children while active, so give it a
 * dedicated element. This class is the only place framework (Solid) reactivity
 * lives; tsappkit core and any presenter behind it stay framework-neutral.
 */
export class SolidIsland extends BaseComponent {
  private disposeRoot?: () => void;

  constructor(
    componentId: string,
    rootElement: HTMLElement,
    private readonly renderRoot: () => JSX.Element,
    eventBus: EventBus | null = null,
    debugMode: boolean = false,
  ) {
    super(componentId, rootElement, eventBus, debugMode);
  }

  /**
   * Mount the Solid tree into `rootElement`. Idempotent: calling it again while
   * already mounted is a no-op (the existing root is kept, not remounted).
   */
  public activate(): void {
    if (this.disposeRoot) return;
    this.disposeRoot = render(this.renderRoot, this.rootElement);
  }

  /**
   * Dispose the Solid tree (runs `onCleanup`, removes the inserted nodes), then
   * defer to the base cleanup (clears the `data-component` marker).
   */
  public deactivate(): void {
    this.disposeRoot?.();
    this.disposeRoot = undefined;
    super.deactivate();
  }
}
