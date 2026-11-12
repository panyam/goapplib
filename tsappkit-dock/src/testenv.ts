// Test-only. jsdom has no ResizeObserver, and dockview-core watches its container with one. The
// stub records each observer so a test can report a size with `resizeObserved`.
const observers = new Set<StubResizeObserver>();

class StubResizeObserver {
  readonly targets = new Set<Element>();
  constructor(readonly cb: ResizeObserverCallback) {
    observers.add(this);
  }
  observe(el: Element): void {
    this.targets.add(el);
  }
  unobserve(el: Element): void {
    this.targets.delete(el);
  }
  disconnect(): void {
    this.targets.clear();
    observers.delete(this);
  }
}
globalThis.ResizeObserver = StubResizeObserver as unknown as typeof ResizeObserver;

/** Calls every observer watching `el`, as a browser would after `el` changes size. */
export function resizeObserved(el: Element): void {
  for (const o of [...observers]) {
    if (!o.targets.has(el)) continue;
    const contentRect = { width: el.clientWidth, height: el.clientHeight } as DOMRectReadOnly;
    o.cb([{ target: el, contentRect } as ResizeObserverEntry], o as unknown as ResizeObserver);
  }
}

/** Gives `el` a layout size, which jsdom never computes. */
export function sized(el: HTMLElement, width = 1000, height = 800): void {
  Object.defineProperty(el, "clientWidth", { value: width, configurable: true });
  Object.defineProperty(el, "clientHeight", { value: height, configurable: true });
}
