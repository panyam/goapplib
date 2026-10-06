// What every demo frame shares: the site's theme and the signal run.mjs waits for. Importing this
// module applies the theme; a demo calls ready() once it works, or failed() with why it doesn't.

// The site is dark unless the reader chose light (templates/BasePage.html). The frame is same-origin,
// so it reads the same choice rather than the OS's color scheme.
let light = false;
try {
  light = localStorage.getItem("theme") === "light";
} catch {
  // Storage blocked: stay dark, like the site.
}
const style = document.createElement("style");
style.textContent = `
  :root { color-scheme: ${light ? "light" : "dark"}; }
  body { margin: 0; padding: 1rem; font: 15px/1.5 system-ui, sans-serif;
    background: ${light ? "#ffffff" : "#08130f"}; color: ${light ? "#0f172a" : "#e2e8f0"}; }
  button { font: inherit; padding: .3rem .9rem; }
  .demo-failed { color: ${light ? "#b91c1c" : "#fca5a5"}; font-weight: 600; margin: 0 0 .5rem; }
`;
document.head.prepend(style);

declare const DEMO_ASSETS: { wasm?: string; exec?: string; worker?: string };

/**
 * Where a demo with a wasm/ directory finds its built pieces (build.mjs), versioned by content, for
 * startWorker's options: `{ ...assets, ns: "files" }`.
 */
export const assets = DEMO_ASSETS as { wasm: string; exec: string; worker: string };

// The frame is as tall as what's in it. It's same-origin with the page, so it can size its own
// iframe; the height a page passes to {{ demo }} is only the first guess, before this runs.
const fit = () => {
  const el = window.frameElement as HTMLElement | null;
  if (el) el.style.height = `${document.documentElement.scrollHeight}px`;
};
new ResizeObserver(fit).observe(document.documentElement);

/** Marks the demo as working, for run.mjs (data-demo="ready" on <html>). */
export function ready(): void {
  document.documentElement.dataset.demo = "ready";
}

/** Marks the demo as broken, with why, for run.mjs and for a reader looking at the frame. */
export function failed(why: unknown): void {
  const msg = why instanceof Error ? why.message : String(why);
  document.documentElement.dataset.demo = `error: ${msg}`;
  const p = document.createElement("p");
  p.className = "demo-failed";
  p.textContent = `This demo stopped: ${msg}`;
  document.body.prepend(p);
}

addEventListener("error", (e) => failed(e.error ?? e.message));
addEventListener("unhandledrejection", (e) => failed(e.reason));
