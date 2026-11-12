// A dock whose panels are this site's own pages. The Pages panel lists every page from the site's
// navigation, and opening one fetches it and moves its article into a new panel. A link inside a page
// panel opens its page as another panel rather than navigating away. A Solid counter and a plain
// notes box sit beside them, so you can watch state survive moves between the grid, a float and the
// four drawers. The layout, the open pages included, is saved, so a reload brings it back.
//
// Under automation (run.mjs) the demo moves panels around itself and checks what survived, then
// calls ready() or failed().
import "dockview/dist/styles/dockview.css";
import { createSignal } from "solid-js";
import { createDock, type Dock, type PanelDef } from "../../../tsappkit-dock/src";
import { failed, ready } from "../_lib/frame";
import { mountCounter } from "./counter";

const KEY = "goapplib-docsite-dock";
const prefix = location.pathname.split("/static/demos/")[0];
let light = false;
try {
  light = localStorage.getItem("theme") === "light";
} catch {
  // stay dark, like the site
}
document.documentElement.classList.toggle("dark", !light);
if (!window.frameElement) document.body.classList.add("standalone");

// ---- the site's pages ----

interface Page {
  path: string; // "/guide/islands/", relative to the site's prefix
  title: string;
  section: string;
}

const titles = new Map<string, string>();
const pageId = (path: string) => `page:${path}`;
const sitePath = (href: string): string | null => {
  const url = new URL(href, location.href);
  if (url.origin !== location.origin || !url.pathname.startsWith(prefix + "/")) return null;
  const path = url.pathname.slice(prefix.length);
  if (path.startsWith("/static/")) return null;
  return path.endsWith("/") ? path : path + "/";
};

async function fetchDoc(path: string): Promise<Document> {
  const res = await fetch(prefix + path);
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return new DOMParser().parseFromString(await res.text(), "text/html");
}

// Every page the site's navigation names: the header's sections, then each section's sidebar.
async function sitePages(): Promise<Page[]> {
  const home = await fetchDoc("/");
  const sections = [...home.querySelectorAll<HTMLAnchorElement>(".main-nav .nav-link")]
    .map((a) => ({ name: a.textContent!.trim(), path: sitePath(a.getAttribute("href")!) }))
    .filter((s): s is { name: string; path: string } => !!s.path);
  const pages: Page[] = [];
  for (const s of sections) {
    const doc = await fetchDoc(s.path);
    for (const a of doc.querySelectorAll<HTMLAnchorElement>(".section-nav a")) {
      const path = sitePath(a.getAttribute("href")!);
      if (!path || pages.some((p) => p.path === path)) continue;
      pages.push({ path, title: a.textContent!.trim(), section: s.name });
      titles.set(path, a.textContent!.trim());
    }
  }
  return pages;
}

// ---- panel content ----

const hosts = new Map<string, HTMLElement>();
const loaded = new Map<string, Promise<void>>();
let dock: Dock;

function pageHost(path: string): HTMLElement {
  const el = document.createElement("div");
  el.className = "page-panel";
  el.dataset.page = path;
  el.textContent = "Loading…";
  loaded.set(
    path,
    fetchDoc(path).then(
      (doc) => {
        const article = doc.querySelector("article.content");
        if (!article) throw new Error(`${path}: no article`);
        // This demo on its own page would load itself inside itself, and again inside that.
        for (const fig of article.querySelectorAll('figure.demo[data-demo="dock"]')) {
          const p = document.createElement("p");
          p.className = "demo-elided";
          p.textContent = "(The dock demo, left out here so it doesn't open inside itself.)";
          fig.replaceWith(p);
        }
        // The article's relative links and sources are relative to its own page, not to this frame.
        const base = new URL(prefix + path, location.href);
        for (const node of article.querySelectorAll<HTMLElement>("[href], [src]")) {
          for (const attr of ["href", "src"]) {
            const v = node.getAttribute(attr);
            if (v && !v.startsWith("#") && !/^[a-z]+:/i.test(v)) node.setAttribute(attr, new URL(v, base).href);
          }
        }
        el.replaceChildren(document.importNode(article, true));
        const h1 = article.querySelector("h1")?.textContent?.trim();
        if (h1 && !titles.has(path)) titles.set(path, h1);
      },
      (err) => {
        el.textContent = `Couldn't load ${path}: ${err.message}`;
        throw err;
      },
    ),
  );
  // A link to another page of the site opens it as a panel. Anchors within the page still scroll.
  el.addEventListener("click", (e) => {
    const a = (e.target as Element).closest?.("a");
    if (!a || a.target === "_blank") return;
    const path = sitePath(a.getAttribute("href") ?? "");
    if (!path || new URL(a.href).hash) return;
    e.preventDefault();
    dock.show(pageId(path));
  });
  return el;
}

const [count, setCount] = createSignal(0);

function hostFor(id: string): HTMLElement {
  let el = hosts.get(id);
  if (el) return el;
  if (id.startsWith("page:")) el = pageHost(id.slice(5));
  else {
    el = document.createElement("div");
    el.className = "pad";
    if (id === "counter") {
      mountCounter(el, count, setCount);
    } else if (id === "notes") {
      el.innerHTML = `<p class="tag">plain DOM</p><textarea class="notes" rows="6" placeholder="Notes survive every move"></textarea>`;
    } else if (id === "pages") {
      el.classList.add("pages");
      el.innerHTML = `<p class="tag">This site</p><p>Loading the site's pages…</p>`;
    }
  }
  hosts.set(id, el);
  return el;
}

// ---- the dock ----

const panels: PanelDef[] = [
  { id: "pages", title: "Pages" },
  { id: "counter", title: "Counter", position: { direction: "below", referencePanel: "pages" } },
  { id: "notes", title: "Notes", defaultPlacement: { kind: "drawer", edge: "bottom" } },
];

let fresh = true;
try {
  fresh = localStorage.getItem(KEY) === null;
} catch {
  // no storage: every load is a first one
}

dock = createDock({
  container: document.getElementById("dock")!,
  panels,
  defaultLayout: [{ id: "pages" }, { id: "counter", position: { direction: "below", referencePanel: "pages" } }],
  resolve: (id) =>
    id.startsWith("page:")
      ? { id, title: titles.get(id.slice(5)) ?? id.slice(5), position: { direction: "right", referencePanel: "pages" } }
      : undefined,
  hostFor,
  storage: localStorage,
  key: KEY,
  theme: () => (light ? "light" : "dark"),
  drawers: { bottom: { size: 220, min: 120 }, top: { size: 200, min: 120 }, left: { min: 220 }, right: { min: 260 } },
});
dock.panelsMenu(document.getElementById("menu")!);
if (fresh) dock.show(pageId("/guide/dock/"));
(window as unknown as { dock: Dock }).dock = dock;

const pagesReady = sitePages().then((pages) => {
  const el = hostFor("pages");
  const groups = new Map<string, Page[]>();
  for (const p of pages) groups.set(p.section, [...(groups.get(p.section) ?? []), p]);
  el.replaceChildren(Object.assign(document.createElement("p"), { className: "tag", textContent: "This site" }));
  for (const [section, list] of groups) {
    const h = document.createElement("h4");
    h.textContent = section;
    const ul = document.createElement("ul");
    for (const p of list) {
      const li = document.createElement("li");
      const a = document.createElement("a");
      a.href = prefix + p.path;
      a.textContent = p.title;
      a.addEventListener("click", (e) => {
        e.preventDefault();
        dock.show(pageId(p.path));
      });
      li.appendChild(a);
      ul.appendChild(li);
    }
    el.append(h, ul);
  }
  // A page restored before the titles were known shows its path as its tab; fix it now.
  for (const panel of dock.api.panels) {
    if (panel.id.startsWith("page:")) panel.api.setTitle(titles.get(panel.id.slice(5)) ?? panel.id.slice(5));
  }
  return pages;
});

// ---- the self-check, under automation only ----

function check(cond: unknown, what: string): void {
  if (!cond) throw new Error(what);
}

async function selfCheck(): Promise<void> {
  const pages = await pagesReady;
  check(pages.length >= 5, `found ${pages.length} pages in the site's nav`);
  const first = pageId("/guide/dock/");
  await loaded.get("/guide/dock/");
  const el = hosts.get(first)!;
  check(el.querySelector("article.content h1"), "the dock guide loaded into its panel");
  check(!el.querySelector('figure.demo[data-demo="dock"]'), "the page's own dock demo was left out");

  for (const target of [
    { kind: "drawer", edge: "bottom" },
    { kind: "float" },
    { kind: "drawer", edge: "left" },
    { kind: "grid" },
  ] as const) {
    dock.place(first, target);
    check(JSON.stringify(dock.where(first)) === JSON.stringify(target), `moved to ${JSON.stringify(target)}`);
    check(hosts.get(first) === el && el.querySelector("article.content h1"), `page kept its content in ${target.kind}`);
  }

  hosts.get("counter")!.querySelector<HTMLButtonElement>(".count")!.click();
  dock.place("counter", { kind: "drawer", edge: "right" });
  dock.place("counter", { kind: "grid" });
  check(count() === 1, "Solid counter kept its count");

  // A link inside a page panel opens its page as a panel instead of navigating the frame.
  dock.show(pageId("/guide/"));
  await loaded.get("/guide/");
  const link = hosts.get(pageId("/guide/"))!.querySelector<HTMLAnchorElement>('article a[href$="tour/"]');
  check(link, "the guide index links to the tour page");
  link!.click();
  await loaded.get("/guide/tour/");
  check(dock.where(pageId("/guide/tour/"))?.kind === "grid", "a link in a page opened its page as a panel");
  check(location.pathname.endsWith("/static/demos/dock/") || location.pathname.endsWith("/index.html"), "the frame didn't navigate");

  // What jsdom can't see: the drawer sits on its edge at its size, above everything, with the scrim
  // over the grid, and a floating group stays under it.
  const box = document.getElementById("dock")!.getBoundingClientRect();
  dock.place("counter", { kind: "float" });
  dock.show("notes");
  const bottom = document.querySelector<HTMLElement>(".dock-drawer-bottom")!.getBoundingClientRect();
  check(Math.abs(bottom.bottom - box.bottom) <= 2 && Math.abs(bottom.height - 220) <= 1, `bottom drawer is 220px on the bottom edge (got ${bottom.height}px at ${bottom.bottom} vs ${box.bottom})`);
  const over = document.elementFromPoint(box.left + box.width / 2, box.top + 20);
  check(over?.classList.contains("dock-drawer-scrim"), `the scrim covers the grid (found ${over?.className})`);
  const inDrawer = document.elementFromPoint(bottom.left + bottom.width / 2, bottom.top + bottom.height / 2);
  check(inDrawer?.closest(".dock-drawer-bottom"), `the drawer is on top (found ${inDrawer?.className})`);
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
  check(!dock.where("notes") || document.querySelector<HTMLElement>(".dock-drawer-bottom")!.style.display === "none", "Escape closed the drawer");

  dock.place(pageId("/guide/tour/"), { kind: "drawer", edge: "right" });
  const right = document.querySelector<HTMLElement>(".dock-drawer-right")!.getBoundingClientRect();
  check(Math.abs(right.right - box.right) <= 2, "right drawer sits on the right edge");
  const floatEl = dock.api.getPanel("counter")!.group.element.getBoundingClientRect();
  const probe = document.elementFromPoint(floatEl.left + floatEl.width / 2, floatEl.top + floatEl.height / 2);
  check(probe?.classList.contains("dock-drawer-scrim") || probe?.closest(".dock-drawer"), "the floating group is under the drawer and its scrim");
  dock.toggleDrawer("right", false);

  const moveBtn = dock.api.getPanel("pages")!.group.element.querySelector<HTMLButtonElement>(".dock-move-btn")!;
  moveBtn.click();
  const menu = document.querySelector<HTMLElement>(".dock-menu")!.getBoundingClientRect();
  const top = document.elementFromPoint(menu.left + 10, menu.top + menu.height / 2);
  check(top?.closest(".dock-menu"), "the move menu is on top");
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
  check(!document.querySelector(".dock-menu"), "Escape closed the move menu");
  dock.reset();
  dock.show(first);
}

if (navigator.webdriver) {
  selfCheck().then(ready, (err: Error) => failed(err.message));
} else {
  pagesReady.then(() => ready(), (err: Error) => failed(err.message));
}
