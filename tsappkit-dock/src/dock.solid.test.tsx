// Solid and vanilla panels in one dock. The dock only moves elements, so these check the pattern
// the README gives a Solid app: mount once into the element hostFor returns, and keep it.
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createSignal, onCleanup } from "solid-js";
import { render } from "solid-js/web";
import { createDock, type Dock } from "./dock.js";
import type { DockStorage, PanelDef, PanelSpec } from "./layout.js";

const KEY = "solid-dock";
const PANELS: PanelDef[] = [
  { id: "counter", title: "Counter" },
  { id: "notes", title: "Notes" },
  { id: "log", title: "Log" },
];
const LAYOUT: PanelSpec[] = [
  { id: "counter" },
  { id: "notes", position: { direction: "right", referencePanel: "counter" } },
  { id: "log", position: { direction: "below", referencePanel: "counter" } },
];

function fakeStorage(): DockStorage {
  const data: Record<string, string> = {};
  return {
    getItem: (k) => (k in data ? data[k] : null),
    setItem: (k, v) => {
      data[k] = v;
    },
    removeItem: (k) => {
      delete data[k];
    },
  };
}

// status stands in for app state outside the panel, such as a presenter's, which keeps changing
// while the panel is closed.
const [status, setStatus] = createSignal("idle");

function Counter() {
  const [n, setN] = createSignal(0);
  return (
    <button class="count" onClick={() => setN(n() + 1)}>
      clicked {n()} ({status()})
    </button>
  );
}

let container: HTMLElement;
let hosts: Map<string, { el: HTMLElement; dispose?: () => void }>;
let mounts: Record<string, number>;
let cleanups: Record<string, number>;
const docks: Dock[] = [];

// The app's side of the contract: build each host once, mount Solid into it once. The log panel
// is torn down on release, so it shows the other choice an app can make.
function hostFor(id: string): HTMLElement {
  const known = hosts.get(id);
  if (known) return known.el;
  const el = document.createElement("div");
  mounts[id] = (mounts[id] ?? 0) + 1;
  let dispose: (() => void) | undefined;
  if (id === "counter") dispose = render(() => <Counter />, el);
  if (id === "log")
    dispose = render(() => {
      onCleanup(() => (cleanups.log = (cleanups.log ?? 0) + 1));
      return <p class="log">log</p>;
    }, el);
  if (id === "notes") {
    const input = document.createElement("input");
    input.className = "notes";
    el.appendChild(input);
  }
  hosts.set(id, { el, dispose });
  return el;
}

function release(id: string): void {
  if (id !== "log") return;
  hosts.get(id)?.dispose?.();
  hosts.delete(id);
}

function make(storage: DockStorage | null = null): Dock {
  const dock = createDock({ container, panels: PANELS, defaultLayout: LAYOUT, hostFor, release, storage, key: KEY, theme: () => "light" });
  docks.push(dock);
  return dock;
}

const q = <T extends Element>(sel: string) => container.querySelectorAll<T>(sel);

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  hosts = new Map();
  mounts = {};
  cleanups = {};
  setStatus("idle");
});

afterEach(() => {
  for (const d of docks.splice(0)) d.dispose();
  for (const h of hosts.values()) h.dispose?.();
  container.remove();
});

describe("Solid and vanilla panels", () => {
  it("renders a Solid tree and a vanilla element in their panels, and Solid's handlers work in the dock", () => {
    make();
    expect(q(".count")).toHaveLength(1);
    expect(q(".notes")).toHaveLength(1);
    q<HTMLButtonElement>(".count")[0].click();
    q<HTMLButtonElement>(".count")[0].click();
    expect(q(".count")[0].textContent).toBe("clicked 2 (idle)");
  });

  it("keeps a closed Solid panel's state and mounts it once across close and reopen", () => {
    const dock = make();
    q<HTMLButtonElement>(".count")[0].click();
    const button = q(".count")[0];
    dock.toggle("counter");
    expect(q(".count")).toHaveLength(0);
    // The tree is detached, not disposed, so it still follows state that changes meanwhile.
    setStatus("busy");
    dock.show("counter");
    expect(q(".count")).toHaveLength(1);
    expect(q(".count")[0]).toBe(button);
    expect(button.textContent).toBe("clicked 1 (busy)");
    expect(mounts.counter).toBe(1);
  });

  it("keeps a vanilla panel's input value across close and reopen", () => {
    const dock = make();
    q<HTMLInputElement>(".notes")[0].value = "draft";
    dock.toggle("notes");
    dock.show("notes");
    expect(q<HTMLInputElement>(".notes")[0].value).toBe("draft");
    expect(mounts.notes).toBe(1);
  });

  it("disposes a Solid tree in release and mounts a fresh one when the panel reopens", () => {
    const dock = make();
    dock.toggle("log");
    expect(cleanups.log).toBe(1);
    expect(q(".log")).toHaveLength(0);
    dock.show("log");
    expect(q(".log")).toHaveLength(1);
    expect(mounts.log).toBe(2);
  });

  it("keeps Solid state through Reset layout without a second tree", () => {
    const dock = make(fakeStorage());
    q<HTMLButtonElement>(".count")[0].click();
    dock.reset();
    expect(q(".count")).toHaveLength(1);
    expect(q(".count")[0].textContent).toBe("clicked 1 (idle)");
    expect(mounts.counter).toBe(1);
  });

  it("mounts Solid panels into a restored layout", () => {
    const storage = fakeStorage();
    const first = make(storage);
    first.toggle("notes");
    first.dispose();
    for (const h of hosts.values()) h.dispose?.();
    hosts = new Map();
    mounts = {};
    make(storage);
    expect(q(".count")).toHaveLength(1);
    expect(q(".notes")).toHaveLength(0);
    expect(mounts).toEqual({ counter: 1, log: 1 });
  });
});
