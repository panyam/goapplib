import { describe, expect, it } from "vitest";
import { EventBus } from "./EventBus";
import type { LCMComponent } from "./LCMComponent";
import { LifecycleController } from "./LifecycleController";

const tick = () => new Promise((r) => setTimeout(r, 0));

// Each component logs "<name>.<phase>" as a phase starts, and "<name>.<phase>.done" when an async
// phase finishes, so a test reads the order straight off the log.
function tree(log: string[]) {
  const comp = (name: string, children: LCMComponent[] = [], opts: { slowInit?: boolean; slowActivate?: boolean } = {}): LCMComponent => ({
    async performLocalInit() {
      log.push(`${name}.init`);
      if (opts.slowInit) {
        await tick();
        log.push(`${name}.init.done`);
      }
      return children;
    },
    setupDependencies() {
      log.push(`${name}.deps`);
    },
    async activate() {
      log.push(`${name}.activate`);
      if (opts.slowActivate) {
        await tick();
        log.push(`${name}.activate.done`);
      }
    },
    deactivate() {
      log.push(`${name}.deactivate`);
    },
  });
  const grandchild = comp("grandchild");
  const a = comp("a", [grandchild], { slowInit: true, slowActivate: true });
  const b = comp("b");
  return comp("root", [a, b]);
}

describe("LifecycleController", () => {
  it("runs each phase over the whole tree, level by level, before the next phase starts", async () => {
    const log: string[] = [];
    await new LifecycleController(new EventBus()).initializeFromRoot(tree(log));
    expect(log).toEqual([
      "root.init",
      // One component's performLocalInit finishes before the next one in its level starts.
      "a.init",
      "a.init.done",
      "b.init",
      "grandchild.init",
      "root.deps",
      "a.deps",
      "b.deps",
      "grandchild.deps",
      "root.activate",
      // Activations in a level run together, and the next level waits for all of them.
      "a.activate",
      "b.activate",
      "a.activate.done",
      "grandchild.activate",
    ]);
  });

  it("deactivates leaves first", async () => {
    const log: string[] = [];
    const c = new LifecycleController(new EventBus());
    await c.initializeFromRoot(tree(log));
    log.length = 0;
    await c.deactivateAll();
    expect(log).toEqual(["grandchild.deactivate", "a.deactivate", "b.deactivate", "root.deactivate"]);
  });

  it("doesn't wait for a Promise from setupDependencies", async () => {
    const log: string[] = [];
    const root: LCMComponent = {
      performLocalInit: () => [],
      async setupDependencies() {
        await tick();
        log.push("deps.done");
      },
      activate: () => void log.push("activate"),
      deactivate: () => {},
    };
    await new LifecycleController(new EventBus()).initializeFromRoot(root);
    await tick();
    expect(log).toEqual(["activate", "deps.done"]);
  });

  it("rejects a component returned by two parents", async () => {
    const log: string[] = [];
    const shared = tree(log);
    const parent = (children: LCMComponent[]): LCMComponent => ({
      performLocalInit: () => children,
      setupDependencies: () => {},
      activate: () => {},
      deactivate: () => {},
    });
    const root = parent([parent([shared]), parent([shared])]);
    await expect(new LifecycleController(new EventBus()).initializeFromRoot(root)).rejects.toThrow(/exactly one parent/);
  });

  it("stops at the first phase that throws, so nothing activates", async () => {
    const log: string[] = [];
    const root: LCMComponent = {
      performLocalInit: () => {
        throw new Error("no slot");
      },
      setupDependencies: () => void log.push("deps"),
      activate: () => void log.push("activate"),
      deactivate: () => {},
    };
    await expect(new LifecycleController(new EventBus()).initializeFromRoot(root)).rejects.toThrow("no slot");
    expect(log).toEqual([]);
  });
});
