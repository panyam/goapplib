# goapplib

A Go web-app framework (server-rendered pages, mixins, htmx, templates through templar) with the TS packages `tsappkit` and `tsappkit-solid`. See SUMMARY.md for an overview, USAGE_GUIDE.md and INTEGRATION_GUIDE.md for how to use it, and CAPABILITIES.md for the stack-facing API and version. Bump CAPABILITIES.md whenever the API changes.

## Commands

- `make setup` sets up the git hooks. `make test` runs all the tests, including `make wasm-test` (the wasmhost tests as wasm under Node, which needs `node` on PATH).
- `make exercise-wasmhost` is mission #33's exercise: it builds `exercise/wasmhost` to wasm and drives it in headless Chromium (playwright-core; locally it uses `~/.cache/ms-playwright`). `make exercise-wasmhost-gen` regenerates its proto code with buf and local plugins.
- `make exercise-islands` is mission #60's exercise: a Go page (`exercise/islands`) whose islands have different load strategies, driven in headless Chromium at 1200 and 400 px. Its `below` island is a real `SolidIsland`, so the page is built by `exercise/islands/build.mjs` (esbuild with `esbuild-plugin-solid`) rather than the esbuild command line; the script points tsappkit-solid's `@panyam/tsappkit` and `solid-js` imports at one copy, and the `one-copy` check fails if a second one gets bundled. Checks still waiting on a ticket sit in `pending` in `exercise/islands/run.mjs`; a pending check that passes fails the run (`XPASS`), so the PR that fixes it must take it off the list. Each run saves screenshots to `exercise/islands/dist/screenshots/`, and CI keeps them as the `exercise-islands-screenshots` artifact.
- Images for a PR description go on the orphan `pr-assets` branch under the PR number (`pr-assets:63/…`) and are linked by commit (`https://github.com/panyam/goapplib/raw/<sha>/63/x.png`), so `main` carries no binaries.
- When a capability lands, tag a release (`v0.x.y`). Consumers (thambura, agni, lilbattle) bump to the tag. A release is a "Release vX.Y.Z" commit on main when the versions need setting, then an annotated tag and a GitHub Release whose body is the notes; there's no CHANGELOG file.
- Version bumps are patches (0.6.x) by default, even for new API or a small break with no outside callers. Ask before a minor.
- **One version for everything.** goapplib, `@panyam/tsappkit` and `@panyam/tsappkit-solid` share the version under `## Version` in CAPABILITIES.md (lock-step since v0.6.0), so `v0.6.0` means the same thing in `go.mod` and `package.json`. A PR that bumps one bumps all three, and the release tag is that version. `scripts/check-versions.sh` checks it in CI, and `publish.yml` runs it with the tag before publishing anything.
- A `v*` tag also publishes the TS packages (`.github/workflows/publish.yml`, through `scripts/npm-publish.sh`): each package whose `package.json` version isn't on npm yet is built, tested, published and waited on until `npm view` shows it. So the first PR after a release that changes either package bumps all three versions to the next one. CI's `DRY_RUN=1 scripts/npm-publish.sh` fails when a package's contents differ from the published version with the same number. Auth is npm trusted publishing, so there's no token; a failed run can be retried from the Actions tab (`workflow_dispatch`). Each package names `panyam/goapplib` and `publish.yml` as its trusted publisher, set with `npx npm@latest trust github <package> --repo panyam/goapplib --file publish.yml --allow-publish` (needs a recent npm, which `npx npm@latest` gets, plus a login and an OTP); `npm trust list <package>` shows it. Both packages have it since v0.6.0, so a tag publishes with no manual step; without it the job fails with `E404 Not Found - PUT` and needs `npm publish --access public --otp=<code>` by hand.

## Issues and missions

Work is queued by the missions it serves. The conventions are in `~/.claude/skills/retriage/CONVENTIONS.md`.

- Active missions (each worked in its own worktree; `MISSION=mission_<slug> ~/.claude/skills/retriage/queue.sh` filters):
  - #74 `mission_worker_state`: a worker-hosted service keeps its big state across a reload and gives memory back after a heavy job. Its exercise is `make exercise-worker-state` (#75 builds it). #70 is the epic with the design notes.
  - #60 `mission_lazy_islands`: a page with many islands downloads and mounts only the islands on screen. Exercise `make exercise-islands`. Every goapplib ticket is closed; it closes when panyam/thambura#204 lands a consumer page.
- #33 `mission_worker_host` and #34 `mission_island_pages` closed on 2026-10-04.
- A new issue gets a priority (`P0`–`P3`) when it's filed, plus either a `mission_<slug>` label and a blocked-by link from its mission, or `waiting` with its trigger named in the body. Never both P and `waiting`.
- Lifts from apps (thambura, agni) are `waiting` until a second app needs them. The issue body names that app.
- To see what's next, run `~/.claude/skills/retriage/queue.sh`.

## Gotchas

- **TS packages and pnpm 12.** Each package (`tsappkit`, `tsappkit-solid`) needs `allowBuilds: {esbuild: true}` in its own `pnpm-workspace.yaml`, or `pnpm install --frozen-lockfile` fails on esbuild's build script. A consumer installing a release that's only hours old gets a `minimumReleaseAgeExclude` entry added to its workspace file; that's expected.
- **0.x peer ranges.** `^0.0.5` means 0.0.5 only, and `^0.1.0` means 0.1.x only, which is why tsappkit-solid's peer on tsappkit is `>=0.0.5 <1.0.0` (since 0.0.4). Keep it a plain range, or every tsappkit minor needs a solid release.
- **npm lag.** A new version can take a few minutes to show in `npm view` and the package's `latest`, because the registry's package document is CDN-cached. The per-version URL (`https://registry.npmjs.org/@panyam/tsappkit/0.3.0`) answers at once, so check that after a manual publish.
- **Spec golden file.** `page/testdata/spec.json` is Go's exact `Spec.JSON()` output, `\u003c` escapes included. Regenerate it from Go's output rather than retyping it, since editors and some tools turn `\u003c` back into `<`.
- **tsappkit tests run in jsdom.** `BasePage` calls `window.matchMedia`, which jsdom lacks, so a test that constructs a page stubs it (see `tsappkit/src/page/IslandPage.test.ts`).
- **wasmhost exports never block.** Every `<ns>.*` export copies its arguments and returns a Promise, doing the work on a goroutine (`promise()` in `wasmhost/js.go`). Work run inside the `js.FuncOf` callback hangs as soon as a handler waits on JS, and it hangs rather than failing, which is why `wasm-test` has `-timeout 60s`.
- **Wasm tests under Node.** Await each JS Promise as soon as it's created. Node ends the process on a rejection with no handler yet, so building several rejected promises and awaiting them later crashes the test binary.
- **Worker and client ship together.** A page from one tsappkit version must load that version's `wasmhost/worker.js`. The v0.2.0 worker treats an unknown request kind as `unmount`, so a newer page's `addFiles` against a cached old worker unmounts the mount.
- **Screenshot the viewport, never the full page, in island tests.** A full-page screenshot resizes the viewport to the whole document, which puts every slot in view and sets off `visible` islands.
- **Debugging islands.** Add `?islands` to a page's URL for the overlay (each slot labelled with its island, load and state). In a test, `page.CheckIslands(assets.Names(), specs...)` catches a spec naming an island the registry doesn't have.
- **Solid's `render` appends to its container** rather than replacing what's there, which is why `SolidIsland.activate` clears its element first (#39). And two copies of solid-js in one bundle don't break a click test, so check for them in the metafile (the exercise's `one-copy` check) rather than in the browser.
- **Late islands start on their own.** An island with a non-eager `load` mounts after the page's `LifecycleController` has finished, so it gets its own; nothing orders its `activate` after the page's. Don't defer an island something else needs at startup. A `lazy` registry entry always mounts late, eager or not.
- **A PR with merge conflicts runs no `pull_request` workflows**, while CodeQL still reports green. If `test` and `exercise-wasmhost` are missing from a PR's checks, check `mergeable` first.
