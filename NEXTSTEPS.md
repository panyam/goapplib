# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #60 `mission_lazy_islands`: 2/5 tickets (#61 exercise, #36 load strategies). `make exercise-islands` last ran at be75166 (2026-10-04, logged on #60): 4 pass, 3 pending on #35. Next ready: #35 (lazy registry entries, per-island chunks, Go writing `modulepreload` from the esbuild metafile), then #39 (slot fallbacks) and #42 (dev aids).
- Off-mission P3: #37, #38, #40, #41, #48. Waiting: content services (#6-#9, #13-#15, #17), #18, #19, #28, #29, #43, #44, #49, #56.
- This run: no branch threads open (#62 and #63 merged); moved the islands exercise, pending-list, screenshot and `pr-assets` conventions into CLAUDE.md.

## Across threads

- tsappkit 0.4.0 (load strategies) is merged but unreleased; tag when a consumer page adopts it for #60's done-when, likely after #35.
- npm trusted publishing still isn't set up. The v0.5.0 tag's publish run failed with E404 and both packages went up by hand. Before the next `v*` tag, add the trusted publisher for `@panyam/tsappkit` and `@panyam/tsappkit-solid` on npmjs.com (`panyam` / `goapplib` / `publish.yml`), or plan on `--otp` publishes.
