# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #60 `mission_lazy_islands`: 2/5 tickets (#61 exercise, #36 load strategies). `make exercise-islands` last ran at be75166 (2026-10-04, logged on #60): 4 pass, 3 pending on #35. Next ready: #35 (lazy registry entries, per-island chunks, Go writing `modulepreload` from the esbuild metafile), then #39 (slot fallbacks) and #42 (dev aids).
- Off-mission P3: #37, #38, #40, #41, #48. Waiting: content services (#6-#9, #13-#15, #17), #18, #19, #28, #29, #43, #44, #49, #56.
- This run: no branch threads open (#62 and #63 merged); moved the islands exercise, pending-list, screenshot and `pr-assets` conventions into CLAUDE.md.

## Across threads

- v0.6.0 is the first lock-step release (goapplib, tsappkit and tsappkit-solid all 0.6.0; the never-released tsappkit 0.4.0 is folded into it). It carries #36's load strategies and #35's lazy chunks, for #60's done-when consumer page.
- npm trusted publishing needs `npm trust github` run once per package (command in CLAUDE.md) before the v0.6.0 tag, or the publish job fails with E404 as v0.5.0's did.
