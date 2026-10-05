# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #60 `mission_lazy_islands`: 7/7 goapplib tickets closed. `make exercise-islands` last ran at 03a6a3b (2026-10-05, logged on #60): all 11 checks pass, nothing merged since. It stays open for done-when's consumer page, panyam/thambura#204.
- Mission #74 `mission_worker_state` (new, active): 0/6 tickets. Next ready: #75 (build `make exercise-worker-state`, every check pending), then #76 (blob store) and #78 (cancellation). #70 is its epic.
- Off-mission P3: #37, #38, #40, #41, #48. Waiting: content services (#6-#9, #13-#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64.
- This run: retriage made #70 into mission #74 with tickets #75-#80; no branch threads open (#65, #66, #68, #69, #72, #73 merged; v0.6.0 to v0.6.3 released); dropped the lock-step and trusted-publishing notes, now in CLAUDE.md.

## Across threads

- When thambura#204 merges, run `make exercise-islands` once more, post the mission log on #60, and close #60 if the consumer page loads its islands lazily. agni has its own ticket for the same adoption (panyam/agni#903, P3).
- Dependabot PR #25 (Go modules, 2026-09-02) has been open a month with green checks: merge it or close it.
