---
title: "Docks and drawers"
description: "An IDE-like page from @panyam/tsappkit-dock: panels in a dockview grid, floating, or in drawers on any edge, with a saved layout that picks up new panels."
---

`@panyam/tsappkit-dock` builds an IDE-like page on [dockview-core](https://dockview.dev). You list your panels and a default arrangement. Each panel then lives in the grid, in a floating group over it, or in a drawer on one of the four edges, and the reader moves it with the ⋯ menu on its group or its drawer tab. The layout is saved, drawers included, so a reload brings back what the reader arranged.

The demo below is a dock whose panels are this site's own pages. Open one from the Pages panel, and a link inside it opens its page as another panel. Move any panel with its ⋯ menu, type into Notes in the bottom drawer, and click the Solid counter, then move them around and see that both keep their state. Reload and the layout comes back, open pages included. Panels ▾ lists every panel and where it is, and Reset layout puts everything back.

{{ demo "dock" 640 }}

## Panels, hostFor and release

A panel's content lives outside dockview. `hostFor(id)` returns the element a panel shows, and the dock calls it each time the panel arrives somewhere: in the grid, in a float, or in a drawer. `release(id, el)` hands the element back when the panel leaves. Return the same element from `hostFor` every time, and whatever it holds survives a close, a reopen and every move.

```ts
import { createDock } from "@panyam/tsappkit-dock";

const hosts = new Map<string, HTMLElement>();
const dock = createDock({
  container: document.getElementById("dock")!,
  panels: [
    { id: "files", title: "Files" },
    { id: "diff", title: "Diff" },
    { id: "log", title: "Log", defaultPlacement: { kind: "drawer", edge: "bottom" } },
  ],
  defaultLayout: [
    { id: "diff" },
    { id: "files", position: { direction: "left", referencePanel: "diff" } },
  ],
  hostFor: (id) => hosts.get(id) ?? buildPanel(id),
  storage: localStorage,
  key: "myapp-dock",
});
dock.panelsMenu(document.getElementById("toolbar")!);
```

`log` starts in the bottom drawer, and Reset layout puts it back there. A panel the registry gains in a later release opens on the reader's next load, and a panel the reader closed stays closed, since the saved layout records the registry it was saved with.

A move between the grid and a drawer calls `release` and then `hostFor`. A move between the grid and a float stays inside dockview, which keeps the element, so it calls neither.

## Moving panels from code

```ts
dock.place("log", { kind: "grid" });
dock.place("files", { kind: "drawer", edge: "left" });
dock.place("diff", { kind: "float" });
dock.where("files");          // { kind: "drawer", edge: "left" }
dock.show("log");             // brings it to the front, opening its drawer if it's in one
dock.toggleDrawer("bottom");
```

Drawers lie over the dock with a scrim behind them. A drawer that holds several panels shows them as tabs. Escape or a click on the scrim closes it, and a drag on its inner edge resizes it, clamped to `min` and to `maxFrac` of the dock on that axis. `drawers: { bottom: { size: 220, min: 120 } }` sets one edge's numbers.

## Panels opened at runtime

The demo's page panels aren't in the registry, since there's one per page the reader opens. `resolve(id)` describes them:

```ts
resolve: (id) => id.startsWith("page:") ? { id, title: titleOf(id), position: { direction: "right", referencePanel: "pages" } } : undefined,
```

`dock.show("page:/guide/islands/")` opens one, it can be placed anywhere, and a saved layout brings it back as long as `resolve` still knows it. The Panels menu lists the open ones under the registry's.

## Solid panels

Mount the Solid tree once into the element `hostFor` returns, and return that element every time. Solid's `render` appends, so rendering inside `hostFor` adds a second tree on each move. A closed panel's tree keeps running, since it's only detached. To tear it down, call `render`'s dispose function in `release` and build a fresh host next time. Solid listens for events on the document, so a click on a closed panel's element, which isn't in the document, does nothing.

## Gotchas

- **An iframe reloads when it moves into or out of a drawer.** The browser reloads an iframe whose element gets a new parent, and a drawer is a new parent. dockview keeps panel content in a layer of its own, so moving between groups or into a float doesn't reload it. The demo fetches each page and moves its article into the panel rather than using an iframe for that reason. The live demos inside a page panel are iframes, so they restart when their page moves into a drawer.
- **dockview-core 8.x ships no stylesheet.** Take `dockview/dist/styles/dockview.css` from the `dockview` package. 4.13 to 7.x have it at `dockview-core/dist/styles/dockview.css`.
- **A panel reopened from the Panels menu goes to its `position`, then to its spec in the default layout.** The first panel in a default layout usually has neither, so dockview adds it to the active group. Give it a `position` if it should come back to the same place.
- **The page's own scripts don't run in a fetched page panel.** The demo moves the article's HTML, so code blocks have no copy buttons and Mermaid diagrams show as text.
