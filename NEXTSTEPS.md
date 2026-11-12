# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No branch threads open, no active mission, nothing at P1 or P2. The queue is P3 only: #37, #38, #40, #114, #134, #140 (`MobileBottomDrawer` as the dock's bottom drawer).
- Waiting on consumers to port onto tsappkit-dock 0.7.10: panyam/diffpp#172, panyam/agni#1549 (agni#1542's drawers are blocked by it), turnforge/lilbattle#203. Both diffpp and agni have a comment saying 0.7.10 is the version to port to. lilbattle#201 (the v0.7.3 upgrade) is still open. These ports are the most likely source of the next goapplib ticket.
- This run: the dock lab (PR #143) closed #138 (no pinned mode, pin means dock) and #139 (defer popouts, leave them to `dock.api`). PR #144 closed #142 (grid placement stays in visible grid groups, Dock from a drawer lands on its edge, `where()` knows popouts, new `moved` hook) and shipped as v0.7.10, published to npm with no manual step.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118.

## Across threads

- Closed missions (#33, #34, #60, #74, #91) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
