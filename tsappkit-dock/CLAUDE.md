# tsappkit-dock

`@panyam/tsappkit-dock`: `createDock` over dockview-core, the saved layout with reconcile and prune, theme, drawers on four edges, `place`, and the `moved` hook (#133, #142). The pure halves (`layout.ts`, `drawer.ts`) need no DOM. `drawerView.ts` is the drawer element, and `dock.ts` is the dockview glue that moves panels between the grid, floats and drawers.

- Tests run against a real dockview-core in jsdom, with `ResizeObserver` stubbed in `vitest.setup.ts`, so nothing has a size. Check geometry in a browser, not here.
- dockview reports layout changes a microtask late (`AsapEvent`). A test that reads storage after a change awaits first (`settle()` in `dock.test.ts`). `createDock` saves once after restore and again in `dispose` for the same reason.
- The peer range (4.13 to 8.x) was checked by running the suite on 4.13.1, 5.2, 6.6, 7.0 and 8.9. Widen it the same way. dockview 4 throws on a second `dispose`, so `Dock.dispose` guards against one.
- `dock.solid.test.tsx` mounts Solid trees in panels (solid-js and vite-plugin-solid are devDependencies only). Solid delegates events to `document`, so a click on a detached panel's element does nothing; drive a closed panel's state through a signal instead.
- dockview-core 8.x ships no stylesheet. Apps take `dockview/dist/styles/dockview.css` from the `dockview` package; 4.13 to 7.x have it at `dockview-core/dist/styles/dockview.css`.
- A move between the grid and a drawer calls `release` and then `hostFor`, and so re-parents the element. The browser reloads an iframe when it's re-parented. Grid to float stays inside dockview and doesn't. `drawers.test.ts` checks the exact `release`/`hostFor` sequence for each move.
- The drawers' change hook saves the layout. `quiet` suppresses it while `restore` and `reset` rebuild, so a half-built state is never saved.
- `moved` comes from `sync()`, which compares each panel's place and document with the last ones seen, after every layout change and every drawer change. `place`, `show` and `toggle` run under `batch()` (the `quiet` counter), so a move that passes through closed reports once. jsdom can't make a popout; the docsite lab's `dock-popout` demo checks `moved` and `where()` on real popouts.
- Grid placement picks only visible grid groups (`addToGrid`). While a group is popped out, dockview keeps a hidden placeholder for it in the grid, and `api.groups` lists it with location `grid`. A panel tabbed into it gets a 0 px column (#142). The lab's `back-size` row catches it.
