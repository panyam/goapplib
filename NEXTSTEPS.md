# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No branch threads open, no active mission, nothing at P1 or P2. The queue is P3 only: #37, #38, #40, #114, #134.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172, panyam/agni#1549 (agni#1542's drawers are blocked by it), turnforge/lilbattle#203. lilbattle#203 also carries the mobile viewer's move off the deprecated `MobileBottomDrawer`, and it needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. These ports are the most likely source of the next goapplib ticket.
- This run: PR #145 closed #140 (the dock's bottom drawer works as a phone sheet, and `MobileBottomDrawer` is deprecated). It shipped as v0.7.11, and all three npm packages published. #146 waits on lilbattle's mobile port for a side-by-side on a phone.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.

## Across threads

- Closed missions (#33, #34, #60, #74, #91) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
