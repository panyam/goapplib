// The panels the dock experiments share: a canvas that counts the pointer and resize events it gets,
// a Solid counter, and a plain-DOM notes box. Each is built once and handed out by hostFor, the way
// the dock guide says to, and each counts how often it was handed out and handed back.
import { createSignal } from "solid-js";
import { render } from "solid-js/web";

export interface Counts {
  hostFor: number;
  release: number;
  pointer: number;
  resize: number;
}

export const counts = new Map<string, Counts>();
const tally = (id: string): Counts => {
  let c = counts.get(id);
  if (!c) counts.set(id, (c = { hostFor: 0, release: 0, pointer: 0, resize: 0 }));
  return c;
};

export const [solidCount, setSolidCount] = createSignal(0);
const hosts = new Map<string, HTMLElement>();

const redraws = new Map<string, () => void>();

function canvasPanel(id: string): HTMLElement {
  const el = document.createElement("div");
  el.className = "fx-canvas";
  el.appendChild(newCanvas(id, document));
  const draw = () => {
    const canvas = el.querySelector("canvas")!;
    const r = el.getBoundingClientRect();
    canvas.width = Math.max(1, Math.round(r.width));
    canvas.height = Math.max(1, Math.round(r.height));
    const g = canvas.getContext("2d");
    if (!g) return;
    g.fillStyle = "#0f172a";
    g.fillRect(0, 0, canvas.width, canvas.height);
    g.strokeStyle = "#22c55e";
    g.lineWidth = 2;
    for (let x = 20; x < canvas.width; x += 60) g.strokeRect(x, canvas.height / 2 - 15, 30, 30);
    g.fillStyle = "#e2e8f0";
    g.font = "13px system-ui";
    g.fillText(`canvas ${canvas.width} × ${canvas.height}`, 12, 20);
  };
  redraws.set(id, draw);
  // Drawing in the observer callback resizes the canvas inside it, which the browser reports as a
  // ResizeObserver loop, so the redraw waits a frame.
  new ResizeObserver(() => {
    tally(id).resize++;
    requestAnimationFrame(draw);
  }).observe(el);
  return el;
}

function solidPanel(): HTMLElement {
  const el = document.createElement("div");
  el.className = "fx-pad";
  render(
    () => (
      <>
        <p class="fx-tag">Solid</p>
        <button class="count" onClick={() => setSolidCount(solidCount() + 1)}>
          clicked {solidCount()}
        </button>
      </>
    ),
    el,
  );
  return el;
}

function notesPanel(): HTMLElement {
  const el = document.createElement("div");
  el.className = "fx-pad";
  el.innerHTML = `<p class="fx-tag">plain DOM</p><textarea class="notes" rows="5" placeholder="notes"></textarea>`;
  return el;
}

function listPanel(): HTMLElement {
  const el = document.createElement("div");
  el.className = "fx-pad";
  el.innerHTML = `<p class="fx-tag">Checks</p><ul class="fx-list">${["U1.3 unconnected", "VCC no decoupling", "J2 no footprint", "R7 value missing"]
    .map((t) => `<li>${t}</li>`)
    .join("")}</ul>`;
  return el;
}

function newCanvas(id: string, doc: Document): HTMLCanvasElement {
  const canvas = doc.createElement("canvas");
  canvas.addEventListener("pointerdown", () => tally(id).pointer++);
  return canvas;
}

/**
 * Swaps a canvas panel's <canvas> for a new one made in the panel's current document, and draws it.
 * A canvas that has been in a popout window can't draw once the window closes, so the lab calls this
 * from the dock's moved hook when documentChanged is true.
 */
export function rebuildCanvas(id: string): void {
  const el = hosts.get(id);
  const old = el?.querySelector("canvas");
  if (!el || !old) return;
  old.replaceWith(newCanvas(id, el.ownerDocument));
  redraws.get(id)?.();
}

/** The dock's hostFor for the experiments: the same element every time, built on first use. */
export function hostFor(id: string): HTMLElement {
  tally(id).hostFor++;
  let el = hosts.get(id);
  if (!el) {
    el = id.startsWith("canvas") ? canvasPanel(id) : id === "solid" ? solidPanel() : id === "checks" ? listPanel() : notesPanel();
    hosts.set(id, el);
  }
  return el;
}

/** The dock's release for the experiments: counts, and keeps the element for the next hostFor. */
export function release(id: string): void {
  tally(id).release++;
}

export const host = (id: string): HTMLElement | undefined => hosts.get(id);

/** A results table the experiments fill in, one row per question. */
export function results(table: HTMLTableElement): (key: string, question: string, answer: string | number | boolean) => void {
  const rows = new Map<string, HTMLTableRowElement>();
  return (key, question, answer) => {
    let row = rows.get(key);
    if (!row) {
      row = table.insertRow();
      row.dataset.key = key;
      row.insertCell().textContent = question;
      row.insertCell();
      rows.set(key, row);
    }
    const text = typeof answer === "boolean" ? (answer ? "yes" : "no") : String(answer);
    row.cells[1].textContent = text;
    row.dataset.answer = text;
  };
}

export const frame = () => new Promise<number>((r) => requestAnimationFrame(r));
export const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
