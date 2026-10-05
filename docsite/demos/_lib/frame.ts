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
`;
document.head.prepend(style);

/** Marks the demo as working, for run.mjs (data-demo="ready" on <html>). */
export function ready(): void {
  document.documentElement.dataset.demo = "ready";
}

/** Marks the demo as broken, with why, for run.mjs and for a reader looking at the frame. */
export function failed(why: unknown): void {
  document.documentElement.dataset.demo = `error: ${why instanceof Error ? why.message : String(why)}`;
}

addEventListener("error", (e) => failed(e.error ?? e.message));
addEventListener("unhandledrejection", (e) => failed(e.reason));
