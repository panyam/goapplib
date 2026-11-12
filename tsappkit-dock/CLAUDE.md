# tsappkit-dock

`@panyam/tsappkit-dock`: `createDock` over dockview-core, the saved layout with reconcile and prune, theme, and drawer sizing (#133). The pure halves (`layout.ts`, `drawer.ts`) need no DOM; `dock.ts` is the dockview glue.

- Tests run against a real dockview-core in jsdom, with `ResizeObserver` stubbed in `vitest.setup.ts`, so nothing has a size. Check geometry in a browser, not here.
- dockview reports layout changes a microtask late (`AsapEvent`). A test that reads storage after a change awaits first (`settle()` in `dock.test.ts`). `createDock` saves once after restore and again in `dispose` for the same reason.
- The peer range (4.13 to 8.x) was checked by running the suite on 4.13.1, 5.2, 6.6, 7.0 and 8.9. Widen it the same way. dockview 4 throws on a second `dispose`, so `Dock.dispose` guards against one.
