# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. #33 `mission_worker_host` closed 2026-10-04: agni#884 moved agni onto wasmhost (goapplib v0.5.0, tsappkit 0.3.0), agni's `wasm.spec.ts` passes, and dropping a design carries on in agni#851. Next: `retriage` to choose a mission.
- Ready without a mission: #48 (wasmhost guide page, P2), #39 (slot fallbacks, P2). Waiting: #49, #56. Open dependabot PRs: #59, #25.
- This run: closed #33.

## Across threads

- npm trusted publishing still isn't set up. The v0.5.0 tag's publish run failed with E404 and both packages went up by hand. Before the next `v*` tag, add the trusted publisher for `@panyam/tsappkit` and `@panyam/tsappkit-solid` on npmjs.com (`panyam` / `goapplib` / `publish.yml`), or plan on `--otp` publishes.
