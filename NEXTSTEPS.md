# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. Storage (#150) closed on 2026-11-19. agni moved onto storage v0.7.15 in panyam/agni#1762, and #174 put storage on the docsite's landing page, overview, README and Go API page. `queue.sh` wants a mission picked. The ready work is all off-mission and P3: #37, #38, #40, #114, #134, #147.
- v0.7.15 is released and is the latest tag. #174 merged after it, but it changed only the docsite and the README.
- agni's storage follow-ups are in agni's `mission_linked_sources`. agni#1724 (connections) is P1. agni#1728 is blocked on agni#1627, and its library half is `connections.Service.CanRead`.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.
- This run: closed #150. Dropped the storage mission lines and the v0.7.13 release note.

## Across threads

- Closed missions (#33, #34, #60, #74, #91, #150) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
- Consumers on v0.7.13 or later use oneauth v0.3.7. agni is on it. thambura, diffpp and lilbattle get an upgrade ticket when they next bump goapplib.
