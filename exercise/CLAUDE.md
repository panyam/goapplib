# exercise/

Each directory is a browser exercise driven in headless Chromium by its `run.mjs` (playwright-core; locally it uses `~/Library/Caches/ms-playwright` or `~/.cache/ms-playwright`). CI runs all of them on every PR (`.github/workflows/test.yml`).

## The exercises

- `make exercise-wasmhost` (mission #33): builds `wasmhost/` to wasm and drives it in a Web Worker. `make exercise-wasmhost-gen` regenerates its proto code with buf and local plugins.
- `make exercise-islands` (mission #60): a Go page whose islands have different load strategies, at 1200 and 400 px. Its `below` island is a real `SolidIsland`, so `islands/build.mjs` bundles it with `esbuild-plugin-solid` and points tsappkit-solid's `@panyam/tsappkit` and `solid-js` imports at one copy. The `one-copy` check fails if a second copy is bundled.
- `make exercise-worker-state` (mission #74): a toy stateful service (`workerstate/`, plain JSON over `http.Handler`) as wasm in a Web Worker. It uses every wasmhost piece: `oneShotFetch` ingests 32 MB through a 256 MB peak, the serving worker restores from the cache, three long jobs run on a `startLane` lane (one beside a quick query, one streamed, one aborted), and the serving lane has a restore-only warm, a 96 MB `maxMemoryBytes` and a `restartIf` on the live heap, which a 100 MB churn request trips. After a reload the page only reads the state back.
- `make exercise-tour` (#128): a fixture app (`tour/web`, an IIFE opened from `file://`) whose tsappkit `Tour` runs at 1200 and 400 px. It checks what jsdom can't: clicks in and outside holes, blocked wheel input, merged holes, a hole following a moving target, the panel clear of the holes.

## Conventions

- A check waiting on a ticket sits in `pending` in `run.mjs`. A pending check that passes fails the run (`XPASS`), so the PR that fixes it takes it off the list.
- Each run saves screenshots to `<name>/dist/screenshots/`, and CI keeps them as the `exercise-<name>-screenshots` artifact.
- Screenshot the viewport, never the full page, in island tests. A full-page screenshot resizes the viewport to the whole document, which puts every slot in view and sets off `visible` islands.
- Two copies of solid-js in one bundle don't break a click test, so check for them in the metafile (the `one-copy` check) rather than in the browser.
