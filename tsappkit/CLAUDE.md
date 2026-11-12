# tsappkit/

`@panyam/tsappkit`: `BasePage` and the component lifecycle, `IslandPage`, `Port`, `Tour`, the wasmhost client, and the docs kit. `README.md` is its npm page.

- Tests run in jsdom. `BasePage` calls `window.matchMedia`, which jsdom lacks, so a test that constructs a page stubs it (see `src/page/IslandPage.test.ts`). `pnpm test` runs `tsc --noEmit` first, so `@ts-expect-error` checks count.
- Late islands start on their own. An island with a non-eager `load`, or any `lazy` registry entry, mounts after the page's `LifecycleController` has finished, so it gets its own and nothing orders its `activate` after the page's. Don't defer an island something else needs at startup.
- Debugging islands: add `?islands` to a page's URL for the overlay. In a Go test, `page.CheckIslands(assets.Names(), specs...)` catches a spec naming an island the registry doesn't have.
- `../page/testdata/spec.json` is Go's exact `Spec.JSON()` output, `<` escapes included, and both Go and TS tests read it. Regenerate it from Go's output; editors turn `<` back into `<`.
