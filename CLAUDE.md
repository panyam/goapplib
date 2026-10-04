# goapplib

A Go web-app framework (server-rendered pages, mixins, htmx, templates through templar) with the TS packages `tsappkit` and `tsappkit-solid`. See SUMMARY.md for an overview, USAGE_GUIDE.md and INTEGRATION_GUIDE.md for how to use it, and CAPABILITIES.md for the stack-facing API and version. Bump CAPABILITIES.md whenever the API changes.

## Commands

- `make setup` sets up the git hooks. `make test` runs all the tests.
- When a capability lands, tag a release (`v0.x.y`). Consumers (thambura, agni, lilbattle) bump to the tag.
- A `v*` tag also publishes the TS packages (`.github/workflows/publish.yml`, through `scripts/npm-publish.sh`): each package whose `package.json` version isn't on npm yet is built, tested, published and waited on until `npm view` shows it. So bump `tsappkit`'s or `tsappkit-solid`'s version in any PR that changes it. CI's `DRY_RUN=1 scripts/npm-publish.sh` fails when a package's contents differ from the published version with the same number. Auth is npm trusted publishing, so there's no token; a failed run can be retried from the Actions tab (`workflow_dispatch`).

## Issues and missions

Work is queued by the missions it serves. The conventions are in `~/.claude/skills/retriage/CONVENTIONS.md`.

- Active mission: #33 `mission_worker_host` (the Web Worker wasm host). #32 and #46 landed; #54 and #55 remain. #34 `mission_island_pages` closed on 2026-10-04 with thambura on tsappkit's `IslandPage`.
- A new issue gets a priority (`P0`–`P3`) when it's filed, plus either a `mission_<slug>` label and a blocked-by link from its mission, or `waiting` with its trigger named in the body. Never both P and `waiting`.
- Lifts from apps (thambura, agni) are `waiting` until a second app needs them. The issue body names that app.
- To see what's next, run `~/.claude/skills/retriage/queue.sh`.

## Gotchas

- **TS packages and pnpm 12.** Each package (`tsappkit`, `tsappkit-solid`) needs `allowBuilds: {esbuild: true}` in its own `pnpm-workspace.yaml`, or `pnpm install --frozen-lockfile` fails on esbuild's build script. A consumer installing a release that's only hours old gets a `minimumReleaseAgeExclude` entry added to its workspace file; that's expected.
- **0.x peer ranges.** `^0.0.5` means 0.0.5 only, and `^0.1.0` means 0.1.x only. When tsappkit's minor moves, widen tsappkit-solid's peer range (`^0.0.5 || ^0.1.0 || ^0.2.0`) and bump solid too, or consumers get unmet-peer warnings.
- **npm lag.** A new version can take a few minutes to show in `npm view`. The publish workflow waits for it; after a manual publish, check before bumping a consumer.
- **Spec golden file.** `page/testdata/spec.json` is Go's exact `Spec.JSON()` output, `\u003c` escapes included. Regenerate it from Go's output rather than retyping it, since editors and some tools turn `\u003c` back into `<`.
- **tsappkit tests run in jsdom.** `BasePage` calls `window.matchMedia`, which jsdom lacks, so a test that constructs a page stubs it (see `tsappkit/src/page/IslandPage.test.ts`).
