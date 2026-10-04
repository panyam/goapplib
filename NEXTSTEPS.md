# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #60 `mission_lazy_islands`: 0/5 tickets. Next ready: #61 (build `make exercise-islands`, failing until #36 and #35 land), then #39 (slot fallbacks). Exercise never run yet.
- Off-mission P3: #37, #38, #40, #41, #48. Content services (#6-#9, #13-#15, #17) and #18 are now `waiting` with triggers.
- This run: retriage after closing #33. Created #60 and #61, moved #35, #36 and #39 to P1, #42 to P2 and #48 to P3, and parked 10 tickets.

## Across threads

- npm trusted publishing still isn't set up. The v0.5.0 tag's publish run failed with E404 and both packages went up by hand. Before the next `v*` tag, add the trusted publisher for `@panyam/tsappkit` and `@panyam/tsappkit-solid` on npmjs.com (`panyam` / `goapplib` / `publish.yml`), or plan on `--otp` publishes.
