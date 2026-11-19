# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Active mission: storage, #150. All 10 of its tickets are closed, and `make exercise-storage` passed on main at 3bf759c. One done-when item is left: agni moving its mounts and blobs onto the library with `make exercise-mounts` printing the same lines. That's panyam/agni#1759 (P1, agni's linked-sources mission #1722). Close #150 once #1759 merges and agni's exercise checks out.
- v0.7.13 is released. It tagged `v0.7.13`, `storage/v0.7.13` and both `storage/backends/*/v0.7.13`, and published the three npm packages. The npm publish needed a by-hand run, because a push of four tags creates no tag events. #171 fixed `release.sh` for the next release, which is the first test of that fix.
- agni#1724 (connections in agni's mounts) is back at P1 after #1759, since its trigger, the storage tag, fired. agni#1728 is blocked on agni#1627, and the library half is `connections.Service.CanRead`.
- No branch threads open in this repo. Off-mission, P3: #37, #38, #40, #114, #134, #147. #147 now also covers a live mount browser in the docsite.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.
- This run: PRs #156, #162, #164–#171 merged. Dropped the "next ready #152, #153" lines, since both are closed.

## Across threads

- Closed missions (#33, #34, #60, #74, #91) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
- Consumers on v0.7.13 move from oneauth v0.1.x or v0.3.2 to v0.3.7. Upgrade tickets other than agni's aren't filed yet. thambura, diffpp and lilbattle get one when they next bump goapplib.
