# tsappkit-dock

`@panyam/tsappkit-dock`: `createDock` over dockview-core, the saved layout with reconcile and prune, theme, and drawer sizing (#133). The pure halves (`layout.ts`, `drawer.ts`) need no DOM; `dock.ts` is the dockview glue.

- Tests run against a real dockview-core in jsdom, with `ResizeObserver` stubbed in `vitest.setup.ts`, so nothing has a size. Check geometry in a browser, not here.
- dockview reports layout changes a microtask late (`AsapEvent`). A test that reads storage after a change awaits first (`settle()` in `dock.test.ts`). `createDock` saves once after restore and again in `dispose` for the same reason.
- The peer range (4.13 to 8.x) was checked by running the suite on 4.13.1, 5.2, 6.6, 7.0 and 8.9. Widen it the same way. dockview 4 throws on a second `dispose`, so `Dock.dispose` guards against one.
- `dock.solid.test.tsx` mounts Solid trees in panels (solid-js and vite-plugin-solid are devDependencies only). Solid delegates events to `document`, so a click on a detached panel's element does nothing; drive a closed panel's state through a signal instead.
- dockview-core 8.x ships no stylesheet. Apps take `dockview/dist/styles/dockview.css` from the `dockview` package; 4.13 to 7.x have it at `dockview-core/dist/styles/dockview.css`.
