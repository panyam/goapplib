# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Active mission: storage, #150, at 1/5. Next ready: #152 (mounts) and #153 (connections), planned together. #154 (blobs) and #155 (example app, guide, `make exercise-storage`) come after. The exercise can't run until #155 writes it.
- #153 builds on oneauth's `outbound` package, which shipped in oneauth v0.3.5 (oneauth#467, PR oneauth#469). The storage module can require v0.3.5 directly. goapplib's root module is still on oneauth v0.1.13.
- The next release is the first whose `release.sh` also tags `storage/vX.Y.Z` (from `scripts/go-modules.sh`). Check that both tags reach GitHub.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- This run: PR #156 merged and closed #151 (storage is its own Go module, and the pre-push hook runs tests again). Dropped agni#1549 and agni#1542 from the consumer list, since both closed.
- Off-mission, P3: #37, #38, #40, #114, #134, #147.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.

## Across threads

- Closed missions (#33, #34, #60, #74, #91) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
