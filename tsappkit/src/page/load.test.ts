import { describe, expect, it } from "vitest";
import { parseLoad, scheduleMount, type LoadEnv } from "./load";

// A stand-in for the browser APIs scheduleMount waits on, which jsdom doesn't have.
function fakeEnv() {
  const idle: (() => void)[] = [];
  const observers: { target: unknown; cb: (entries: { isIntersecting: boolean }[]) => void; connected: boolean }[] = [];
  const queries = new Map<string, { matches: boolean; listeners: ((e: { matches: boolean }) => void)[] }>();
  const env: LoadEnv = {
    requestIdleCallback: (cb: () => void) => void idle.push(cb),
    setTimeout: (cb: () => void) => void idle.push(cb),
    IntersectionObserver: class {
      private o: (typeof observers)[number];
      constructor(cb: (entries: { isIntersecting: boolean }[]) => void) {
        this.o = { target: null, cb, connected: true };
        observers.push(this.o);
      }
      observe(target: unknown) {
        this.o.target = target;
      }
      disconnect() {
        this.o.connected = false;
      }
    } as unknown as LoadEnv["IntersectionObserver"],
    matchMedia: (q: string) => {
      const mq = queries.get(q) ?? { matches: false, listeners: [] };
      queries.set(q, mq);
      return {
        get matches() {
          return mq.matches;
        },
        addEventListener: (_: string, l: (e: { matches: boolean }) => void) => void mq.listeners.push(l),
        removeEventListener: (_: string, l: (e: { matches: boolean }) => void) => {
          mq.listeners = mq.listeners.filter((x) => x !== l);
        },
      } as unknown as MediaQueryList;
    },
  };
  const setMedia = (q: string, matches: boolean) => {
    const mq = queries.get(q) ?? { matches, listeners: [] };
    mq.matches = matches;
    queries.set(q, mq);
    for (const l of [...mq.listeners]) l({ matches });
  };
  return { env, idle, observers, queries, setMedia };
}

describe("parseLoad", () => {
  it("reads the four forms, with empty meaning eager", () => {
    expect(parseLoad(undefined)).toEqual({ kind: "eager" });
    expect(parseLoad("")).toEqual({ kind: "eager" });
    expect(parseLoad("eager")).toEqual({ kind: "eager" });
    expect(parseLoad("idle")).toEqual({ kind: "idle" });
    expect(parseLoad("visible")).toEqual({ kind: "visible" });
    expect(parseLoad("media:(max-width: 600px)")).toEqual({ kind: "media", query: "(max-width: 600px)" });
  });

  it("is null for anything else, as Go's Validate would reject it", () => {
    for (const bad of ["lazy", "Eager", "media:", "media:  ", "visible "]) expect(parseLoad(bad)).toBeNull();
  });
});

describe("scheduleMount", () => {
  it("mounts an eager island at once", () => {
    const { env } = fakeEnv();
    let n = 0;
    scheduleMount({ kind: "eager" }, {} as Element, () => n++, env);
    expect(n).toBe(1);
  });

  it("mounts an idle island when the browser is idle", () => {
    const { env, idle } = fakeEnv();
    let n = 0;
    scheduleMount({ kind: "idle" }, {} as Element, () => n++, env);
    expect(n).toBe(0);
    idle.forEach((cb) => cb());
    expect(n).toBe(1);
  });

  it("falls back to setTimeout where there's no requestIdleCallback", () => {
    const { env, idle } = fakeEnv();
    let n = 0;
    scheduleMount({ kind: "idle" }, {} as Element, () => n++, { ...env, requestIdleCallback: undefined });
    idle.forEach((cb) => cb());
    expect(n).toBe(1);
  });

  it("mounts a visible island on its first intersection, once, and stops watching", () => {
    const { env, observers } = fakeEnv();
    const el = { id: "slot" } as unknown as Element;
    let n = 0;
    scheduleMount({ kind: "visible" }, el, () => n++, env);
    expect(observers).toHaveLength(1);
    expect(observers[0].target).toBe(el);
    observers[0].cb([{ isIntersecting: false }]);
    expect(n).toBe(0);
    observers[0].cb([{ isIntersecting: true }]);
    observers[0].cb([{ isIntersecting: true }]);
    expect(n).toBe(1);
    expect(observers[0].connected).toBe(false);
  });

  it("mounts a media island at once when the query already matches", () => {
    const { env, setMedia } = fakeEnv();
    setMedia("(max-width: 600px)", true);
    let n = 0;
    scheduleMount({ kind: "media", query: "(max-width: 600px)" }, {} as Element, () => n++, env);
    expect(n).toBe(1);
  });

  it("mounts a media island once, on the first change that matches", () => {
    const { env, setMedia, queries } = fakeEnv();
    let n = 0;
    scheduleMount({ kind: "media", query: "(max-width: 600px)" }, {} as Element, () => n++, env);
    expect(n).toBe(0);
    setMedia("(max-width: 600px)", false);
    expect(n).toBe(0);
    setMedia("(max-width: 600px)", true);
    setMedia("(max-width: 600px)", false);
    setMedia("(max-width: 600px)", true);
    expect(n).toBe(1);
    expect(queries.get("(max-width: 600px)")!.listeners).toHaveLength(0);
  });
});
