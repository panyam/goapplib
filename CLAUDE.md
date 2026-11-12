# goapplib

A Go web-app framework (server-rendered pages, mixins, htmx, templates through templar) with three TS packages: `tsappkit`, `tsappkit-solid` and `tsappkit-dock`. SUMMARY.md has the overview, and the docs site (https://panyam.github.io/goapplib/, built from `docsite/`) has the guides. Where each line of work stopped is in NEXTSTEPS.md.

## Where things are

Each of these directories has a CLAUDE.md with its own gotchas, loaded when a session works there:

- `exercise/`: the browser exercises (`make exercise-wasmhost`, `-islands`, `-worker-state`, `-tour`) and their `pending`/XPASS convention.
- `docsite/`: the site, its demos, and `make exercise-docsite`.
- `wasmhost/`: Go handlers in a Web Worker, and which piece fits which problem.
- `tsappkit/`, `tsappkit-solid/`, `tsappkit-dock/`: the npm packages.
- `templates/`: the fields `BasePage.html` expects, and live template actions in comments.
- `scripts/`: releasing and npm publishing.

## Commands

- `make setup` sets up the git hooks. `make test` runs the Go tests, the wasmhost tests as wasm under Node (`make wasm-test`, needs `node`), and each TS package's tests.
- `make exercise-<name>` runs one browser exercise in headless Chromium; CI runs all of them on every PR.
- `~/.claude/skills/retriage/queue.sh` shows what's next.

## Versions and releases

- goapplib and the three TS packages share the version in `VERSION`. A PR that bumps one bumps all four, and the first PR after a release that changes any package bumps them. CI checks it (`scripts/check-versions.sh`).
- Bumps are patches by default, even for new API. Ask before a minor.
- When a capability lands, tag a release. A `v*` tag publishes the TS packages. `scripts/CLAUDE.md` has the steps and the publishing traps.
- Consumers bump to the tag: `panyam/thambura`, `panyam/agni`, `panyam/diffpp`, `turnforge/lilbattle`. File an upgrade ticket in each one a release helps.

## Issues and missions

The conventions are in `~/.claude/skills/retriage/CONVENTIONS.md`.

- A new issue gets a priority (`P0`–`P3`), plus either a `mission_<slug>` label with a blocked-by link from its mission, or `waiting` with its trigger in the body. Never both P and `waiting`.
- Consumers' problems pick the next work (decided 2026-10-06). A lift from an app is `waiting` until a second app needs it, and the body names that app.
- Each mission is worked in its own worktree. `MISSION=mission_<slug> ~/.claude/skills/retriage/queue.sh` filters the queue.

## Repo-wide gotchas

- **pnpm 12.** Each TS package needs `allowBuilds: {esbuild: true}` in its own `pnpm-workspace.yaml`, or `pnpm install --frozen-lockfile` fails on esbuild. A consumer installing a release only hours old gets a `minimumReleaseAgeExclude` entry; that's expected.
- **esbuild is overridden to `^0.28.1`** in each package's `pnpm-workspace.yaml`, since tsup 8.5 still asks for `^0.27`, which has an advisory. Delete the override once tsup moves to 0.28.
- **`go build` fails in a worktree** with `error obtaining VCS status: exit status 128`, since this clone is a bare repo with worktrees. Pass `-buildvcs=false`.
- **PR images** go on the orphan `pr-assets` branch under the PR number and are linked by commit (`https://github.com/panyam/goapplib/raw/<sha>/63/x.png`), so `main` carries no binaries.
- **One PR closing two issues needs "Closes" before each** ("Closes #111, closes #95"). Check `gh pr view N --json closingIssuesReferences` after opening.
- **A PR with merge conflicts runs no `pull_request` workflows**, while CodeQL still reports green. If `test` is missing from a PR's checks, check `mergeable` first.
