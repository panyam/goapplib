# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #74 `mission_worker_state`: 6/6 goapplib tickets closed (#75, #76, #77, #78, #79, #80). `make exercise-worker-state` last ran at c2fbc93 (2026-10-05, logged on #74): all 9 checks pass. It stays open for done-when's agni measurement, panyam/agni#911.
- Mission #60 `mission_lazy_islands`: 7/7 goapplib tickets closed; `make exercise-islands` passes 11/11 (last at 03a6a3b). It stays open for panyam/thambura#204.
- Nothing is ready in goapplib's queue. Off-mission P3: #37, #38, #40, #41, #48. Waiting: content services (#6-#9, #13-#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83.
- This run: no branch threads open (#81, #84, #85, #86, #87, #88 merged); v0.6.8 tagged, the first release since v0.6.3.

## Across threads

- When panyam/agni#911 records its Jetson numbers, post them on #74 and close it; when panyam/thambura#204 merges, run `make exercise-islands` and close #60. With both closed, run `/retriage` to pick the next mission (candidates: #48 wasmhost guide page, #41, #38).
- Dependabot PR #25 (Go modules, 2026-09-02) has been open a month with green checks: merge it or close it.
