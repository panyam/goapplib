# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #33 `mission_worker_host`: 4/4 goapplib tickets closed (#32, #46, #54, #55), released as v0.5.0 with tsappkit 0.3.0 and tsappkit-solid 0.0.4. Exercise `make exercise-wasmhost` passed at d408616 (2026-10-04, logged on #33). The mission closes when agni#863 swaps agni onto wasmhost and its demo works; agni has the rename list and the v0.5.0 notes on that issue.
- Nothing ready in the goapplib queue. Off-mission and ready: #39 (slot fallbacks, P2), #48 (wasmhost guide page, P2). Waiting: #49 (zip drops), #56 (atomic `Host.Add` for fixed handlers).
- This run: no branch threads open (feature/wasmhost-32 and feature/wasmhost-add-stats-54-55 merged); moved the wasmhost, Node-test, worker-pairing and conflicting-PR gotchas into CLAUDE.md.

## Across threads

- npm trusted publishing still isn't set up. The v0.5.0 tag's publish run failed with E404 and both packages went up by hand. Before the next `v*` tag, add the trusted publisher for `@panyam/tsappkit` and `@panyam/tsappkit-solid` on npmjs.com (`panyam` / `goapplib` / `publish.yml`), or plan on `--otp` publishes.
