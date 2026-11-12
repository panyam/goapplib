# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No branch threads open, no active mission, nothing at P1 or P2. The queue is P3 only: #37, #38, #40, #114, #134, and #138–#140 (three tsappkit-dock options #141 didn't take: pinned drawers, popouts, `MobileBottomDrawer` as the bottom drawer).
- Waiting on consumers to take tsappkit-dock 0.7.9 (#133 closed): panyam/diffpp#172, panyam/agni#1549 (agni#1542's drawers are blocked by it), turnforge/lilbattle#203. lilbattle#201 (the v0.7.3 upgrade) is still open. Their porting is the most likely source of the next goapplib ticket.
- This run: shipped v0.7.8 (tsappkit-dock, grid only, PR #136) and v0.7.9 (drawers, `place`, `resolve`, docsite dock demo, PR #141). tsappkit-dock now has npm trust, so tags publish it with no manual step. Root CLAUDE.md went from 17.7K to 3.6K characters, with subsystem detail moved into a CLAUDE.md per directory. Dropped the old notes on agni#996 and thambura#209, both closed.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #125.

## Across threads

- Closed missions (#33, #34, #60, #74, #91) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
