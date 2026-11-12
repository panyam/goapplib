# @panyam/tsappkit-dock

A dock for IDE-like pages, built on [dockview-core](https://www.npmjs.com/package/dockview-core). You list your panels and a default arrangement. The dock restores the reader's saved layout, opens panels you added since it was saved, and keeps the ones the reader closed closed. Panel content lives outside dockview, so it survives a panel being closed and reopened.

```bash
npm install @panyam/tsappkit-dock dockview-core
```

It needs `dockview-core` 4.13 or later, up to 8.x, and no UI framework. It shares one version with tsappkit and goapplib, so `@panyam/tsappkit-dock@0.7.8` goes with `@panyam/tsappkit@0.7.8` and goapplib `v0.7.8`. Load dockview's stylesheet in your page. dockview-core 4.13 to 7.x ship it as `dockview-core/dist/styles/dockview.css`. 8.x doesn't, and the same file comes in the `dockview` package as `dockview/dist/styles/dockview.css`.

## What's in it

- **`createDock`** builds the dock from a panel registry, a default layout, and `hostFor`, which returns the element each panel shows. `release` gets the element back when its panel leaves the grid. The returned dock has `show`, `toggle`, `reset`, `panelsMenu` (a Panels button with a toggle per panel and Reset layout) and `api`, the dockview api.
- **`saveLayout`, `loadLayout`, `panelsToReconcile` and `panelsToPrune`** are the saved-layout logic `createDock` uses, without dockview, for an app that drives dockview itself. A save is `{v: 1, panels, layout}`: `panels` is the registry when it was saved, which is how a later load tells a new panel from a closed one.
- **`resolveTheme` and `documentTheme`** map `<html data-theme>` and the OS colour scheme to light or dark. The dock follows both by default.
- **`drawerSize`, `clampSize`, `loadDrawerSize` and `saveDrawerSize`** size a drawer on any edge from a drag on its inner edge, clamped to a minimum and a share of the viewport, and save it.

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

Panel content can be anything that renders into an element: plain DOM, Solid, or another framework. With Solid, mount once into the element `hostFor` returns, and return that same element every time. The dock calls `hostFor` whenever a panel enters the grid, and Solid's `render` appends, so rendering inside `hostFor` adds a second tree on each reopen. A closed panel's tree keeps running, since it's only detached. To tear it down, call `render`'s dispose function in `release` and build a fresh host next time.

A panel reopened from the Panels menu goes to its `PanelDef.position`, then to its spec in the default layout. The first panel in a default layout usually has no position, so give it one in its `PanelDef` if it should come back to the same place.

Drawers on four edges, floating panels, and moving a panel between them are planned in [#133](https://github.com/panyam/goapplib/issues/133).

## License

Apache-2.0
