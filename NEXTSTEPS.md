# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #33 `mission_worker_host`: 1/2 tickets closed (#32 landed in PR #50 with `make exercise-wasmhost`). Next ready: #46, publish `@panyam/tsappkit` from the release tag. tsappkit's `wasmhost` export from #50 isn't on npm yet, so agni#863 can't take it until there's a publish.
- Mission #34 `mission_island_pages` closed 2026-10-04. goapplib v0.3.0 shipped tsappkit's `IslandPage` (#27, `@panyam/tsappkit` 0.1.0, then 0.1.1 without test files), and thambura adopted it (panyam/thambura#192, #195). Leftovers: #39 (slot fallbacks, P2, needs this branch's thesis on main), #19 (`waiting` on excaliframe), #44 (embed helpers, `waiting` on a second embedding app).

## Across threads

- npm publishes are manual and have needed retries (the registry took minutes to show 0.1.0, and 0.0.2 needed a second push). Check `npm view @panyam/<pkg> version` before bumping a consumer. #46 automates it.
- pnpm 12: each TS package needs `allowBuilds: {esbuild: true}` in its `pnpm-workspace.yaml` for `--frozen-lockfile` to install, and a consumer gets a `minimumReleaseAgeExclude` entry added for a release only hours old.
- Peer ranges on 0.x packages: `^0.0.5` means 0.0.5 only. Widen tsappkit-solid's range whenever tsappkit's minor moves.
