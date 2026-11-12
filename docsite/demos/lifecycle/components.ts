// A page with a store, a counter that shows the store's count, and a button inside the counter that
// bumps it. trace() writes each lifecycle call to the log under the demo.
import { BaseComponent, BasePage, type EventBus, type LCMComponent } from "../../../tsappkit/src";

export type Trace = (component: string, call: string) => void;

const $ = (id: string) => document.getElementById(id)!;

export class Store extends BaseComponent {
  count = 0;

  constructor(bus: EventBus, private trace: Trace) {
    super("store", $("store"), bus);
  }

  performLocalInit(): LCMComponent[] {
    this.trace("store", "performLocalInit");
    return [];
  }

  setupDependencies() {
    this.trace("store", "setupDependencies");
  }

  // Loads its first count asynchronously, the way a fetch would.
  async activate() {
    this.trace("store", "activate");
    await new Promise((r) => setTimeout(r, 50));
    this.set(3);
    this.trace("store", "activate finished");
  }

  set(n: number) {
    this.count = n;
    this.emit("count", n, this);
  }
}

class BumpButton extends BaseComponent {
  store!: Store;

  constructor(bus: EventBus, private trace: Trace) {
    super("bump", $("bump"), bus);
  }

  performLocalInit(): LCMComponent[] {
    this.trace("button", "performLocalInit");
    return [];
  }

  setupDependencies() {
    this.trace("button", "setupDependencies");
  }

  activate() {
    this.trace("button", "activate");
    this.rootElement.addEventListener("click", () => this.store.set(this.store.count + 1));
  }
}

export class Counter extends BaseComponent {
  store!: Store;
  private button: BumpButton;

  constructor(bus: EventBus, private trace: Trace) {
    super("counter", $("counter"), bus);
    this.button = new BumpButton(bus, trace);
  }

  performLocalInit(): LCMComponent[] {
    this.trace("counter", "performLocalInit");
    return [this.button];
  }

  // The page set this.store before this ran, and every component exists by now,
  // so the counter can pass the store down to its own child.
  setupDependencies() {
    this.trace("counter", "setupDependencies");
    this.button.store = this.store;
  }

  activate() {
    this.trace("counter", "activate");
    this.addSubscription("count");
  }

  handleBusEvent(type: string, n: number) {
    if (type !== "count") return;
    this.trace("counter", `got count ${n}`);
    this.findElement("output")!.textContent = String(n);
  }
}

export class DemoPage extends BasePage {
  readonly store: Store;
  private counter: Counter;

  constructor(private trace: Trace) {
    super("lifecycle-demo");
    this.store = new Store(this.eventBus, trace);
    this.counter = new Counter(this.eventBus, trace);
  }

  protected initializeSpecificComponents(): LCMComponent[] {
    this.trace("page", "performLocalInit");
    return [this.store, this.counter];
  }

  setupDependencies() {
    this.trace("page", "setupDependencies");
    this.counter.store = this.store;
  }

  protected bindSpecificEvents() {
    this.trace("page", "activate");
  }
}

// Mounted after the page has started, the way a deferred island is. Nothing calls its
// setupDependencies with other components, so it takes them in its constructor, and it reads the
// count in activate because the store's first "count" event went out before it subscribed.
export class LateBadge extends BaseComponent {
  constructor(bus: EventBus, private store: Store, private trace: Trace) {
    super("badge", $("badge"), bus);
  }

  performLocalInit(): LCMComponent[] {
    this.trace("badge", "performLocalInit");
    return [];
  }

  setupDependencies() {
    this.trace("badge", "setupDependencies");
  }

  activate() {
    this.trace("badge", "activate");
    this.show(this.store.count);
    this.addSubscription("count");
  }

  handleBusEvent(type: string, n: number) {
    if (type === "count") this.show(n);
  }

  private show(n: number) {
    this.rootElement.textContent = `late badge: ${n}`;
  }
}
