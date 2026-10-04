/**
 * When an island mounts: its `load` in the page spec (goapplib's
 * page.Island.Load), read and waited for. Every strategy mounts once; an
 * island isn't unmounted when its media query stops matching.
 */

/** A parsed `load`. An empty or missing one is eager. */
export type LoadStrategy =
  | { kind: "eager" }
  | { kind: "idle" }
  | { kind: "visible" }
  | { kind: "media"; query: string };

/**
 * The strategy `load` names, or null when it isn't one of the forms Go's
 * Validate accepts (`eager`, `idle`, `visible`, `media:<query>`), compared
 * exactly.
 */
export function parseLoad(load: string | undefined): LoadStrategy | null {
  switch (load ?? "") {
    case "":
    case "eager":
      return { kind: "eager" };
    case "idle":
      return { kind: "idle" };
    case "visible":
      return { kind: "visible" };
  }
  const query = load!.startsWith("media:") ? load!.slice("media:".length) : "";
  return query.trim() ? { kind: "media", query } : null;
}

/**
 * The browser APIs scheduleMount waits on, so tests can stand them in.
 * `requestIdleCallback` is optional because Safari doesn't have it.
 */
export interface LoadEnv {
  requestIdleCallback?: (cb: () => void) => unknown;
  setTimeout: (cb: () => void, ms?: number) => unknown;
  IntersectionObserver: typeof IntersectionObserver;
  matchMedia: (query: string) => MediaQueryList;
}

/**
 * Calls `mount` once, when `strategy` says the island in `el` should mount:
 * at once for eager; when the browser is idle (or on the next task, without
 * requestIdleCallback) for idle; the first time any of `el` enters the
 * viewport for visible; and at once if the query matches, or else on the
 * first change that makes it match, for media.
 */
export function scheduleMount(strategy: LoadStrategy, el: Element, mount: () => void, env: LoadEnv = window): void {
  switch (strategy.kind) {
    case "eager":
      mount();
      return;
    case "idle":
      if (env.requestIdleCallback) env.requestIdleCallback(mount);
      else env.setTimeout(mount, 1);
      return;
    case "visible": {
      let done = false;
      const observer = new env.IntersectionObserver((entries) => {
        if (done || !entries.some((e) => e.isIntersecting)) return;
        done = true;
        observer.disconnect();
        mount();
      });
      observer.observe(el);
      return;
    }
    case "media": {
      const mq = env.matchMedia(strategy.query);
      if (mq.matches) {
        mount();
        return;
      }
      const onChange = (e: { matches: boolean }) => {
        if (!e.matches) return;
        mq.removeEventListener("change", onChange);
        mount();
      };
      mq.addEventListener("change", onChange);
      return;
    }
  }
}
