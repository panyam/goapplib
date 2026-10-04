# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #33 `mission_worker_host`: 2/4 tickets closed (#32, #46). Exercise `make exercise-wasmhost` passed at 4ed5f19 (2026-10-04, logged on #33). Next ready: #54 (being worked in another session), then #55. agni#863 can now import `@panyam/tsappkit/wasmhost` (on npm since 0.2.0).
- No mission is waiting behind #33. Off-mission and ready: #39 (slot fallbacks, P2, docs only).
- This run: #46 merged (PR #53); dropped the threads for #34 (closed) and chore/issues-missions (PR #51 merged); moved the pnpm, peer-range and npm-lag notes into CLAUDE.md "Gotchas".

## Across threads

- The tag-triggered npm publish (PR #53) hasn't run on a real tag yet. Before the next `v*` tag, add the trusted publisher for `@panyam/tsappkit` and `@panyam/tsappkit-solid` on npmjs.com (`panyam` / `goapplib` / `publish.yml`), then watch that run.
