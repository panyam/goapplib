# @panyam/tsappkit-dock

A dock for IDE-like pages, built on [dockview-core](https://www.npmjs.com/package/dockview-core). You list your panels and a default arrangement. The dock restores the reader's saved layout, opens panels you added since it was saved, and keeps the ones the reader closed closed. Panel content lives outside dockview, so it survives a panel being closed and reopened.

```bash
npm install @panyam/tsappkit-dock dockview-core
```

It needs `dockview-core` 4.13 or later, up to 8.x, and no UI framework. It shares one version with tsappkit and goapplib, so `@panyam/tsappkit-dock@0.7.10` goes with `@panyam/tsappkit@0.7.10` and goapplib `v0.7.10`. Load dockview's stylesheet in your page. dockview-core 4.13 to 7.x ship it as `dockview-core/dist/styles/dockview.css`. 8.x doesn't, and the same file comes in the `dockview` package as `dockview/dist/styles/dockview.css`.

## What's in it

- **`createDock`** builds the dock from a panel registry, a default layout, and `hostFor`, which returns the element each panel shows. `release` gets the element back when its panel leaves where it was. The returned dock has `show`, `place`, `where`, `toggle`, `toggleDrawer`, `reset`, `panelsMenu` (a Panels button with a toggle per panel and Reset layout) and `api`, the dockview api.
- **Placement.** A panel lives in the grid, in a floating group over it, or in the drawer on one of the four edges. `dock.place(id, {kind: "drawer", edge: "bottom"})` moves it, and so does the ⋯ menu on each group and each drawer tab. `PanelDef.defaultPlacement` says where a panel starts, and Reset layout puts it back there. `dock.where(id)` also reports `{kind: "popout"}` for a panel popped out through `dock.api`.
- **`moved`.** Called after a panel ends up somewhere new, with its element in place. Its `documentChanged` flag is true on the way into a popout window and back, which is when a canvas needs rebuilding.
- **Drawers.** One per edge, laid over the dock with a scrim. A drawer holding several panels shows tabs. Escape or a click on the scrim closes it, and a drag on its inner edge resizes it. The saved layout keeps each drawer's panels, active tab, size and open state.
- **Panels added at runtime.** `resolve(id)` describes a panel the registry doesn't list, such as one per open document. `dock.show(id)` opens it, it can be placed anywhere, and a saved layout brings it back.
- **`saveLayout`, `loadLayout`, `panelsToReconcile` and `panelsToPrune`** are the saved-layout logic `createDock` uses, without dockview, for an app that drives dockview itself. A save is `{v: 2, panels, layout, drawers}`: `panels` is the registry when it was saved, which is how a later load tells a new panel from a closed one. A version 1 save, from 0.7.10, loads as grid only.
- **`resolveTheme` and `documentTheme`** map `<html data-theme>` and the OS colour scheme to light or dark. The dock follows both by default.
- **`drawerSize`, `clampSize`, `loadDrawerSize` and `saveDrawerSize`** are the drawers' sizing, for an app that builds its own: a size on any edge from a drag on its inner edge, clamped to a minimum and a share of the viewport.

A three-panel page whose content is built before the dock:

```ts
import { createDock } from "@panyam/tsappkit-dock";

const hosts = new Map(["files", "diff", "log"].map((id) => [id, document.getElementById(`panel-${id}`)!]));
const park = document.getElementById("park")!;

const dock = createDock({
  container: document.getElementById("dock")!,
  panels: [
    { id: "files", title: "Files" },
    { id: "diff", title: "Diff" },
    { id: "log", title: "Log", defaultOpen: false },
  ],
  defaultLayout: [
    { id: "diff" },
    { id: "files", position: { direction: "left", referencePanel: "diff" } },
    { id: "log", position: { direction: "below", referencePanel: "diff" } },
  ],
  hostFor: (id) => hosts.get(id)!,
  release: (_id, el) => park.appendChild(el),
  storage: localStorage,
  key: "myapp-dock",
});
dock.panelsMenu(document.getElementById("toolbar")!);
```

`log` has `defaultOpen: false`, so a reader whose layout was saved before `log` existed finds it in the Panels menu rather than in their grid.

Panel content can be anything that renders into an element: plain DOM, Solid, or another framework. With Solid, mount once into the element `hostFor` returns, and return that same element every time. The dock calls `hostFor` whenever a panel arrives somewhere, and Solid's `render` appends, so rendering inside `hostFor` adds a second tree on each reopen. A closed panel's tree keeps running, since it's only detached. To tear it down, call `render`'s dispose function in `release` and build a fresh host next time.

A panel reopened from the Panels menu goes to its `PanelDef.position`, then to its spec in the default layout. The first panel in a default layout usually has no position, so give it one in its `PanelDef` if it should come back to the same place.

Moving a panel into or out of a drawer moves its element to a new parent, and the browser reloads an iframe when that happens. Moving between the grid and a float, or between tabs in the grid, doesn't, since dockview keeps panel content in a layer of its own. Keep iframes out of panels that go into drawers, or reload what they show.

The [dock guide](https://panyam.github.io/goapplib/guide/dock/) has a live demo whose panels are the docsite's own pages.

## License

Apache-2.0
