# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. The next goapplib work comes from agni's `mission_linked_sources` (panyam/agni#1722). Ready off-mission P3s: #114 (a real cache bug), #134, #37, #38, #40, #147. 
- v0.7.24 is the latest tag. v0.7.23 keys a connection by owner and ID (#197), which breaks `connections.Store`, `Service` and `mounts.WithCredentials` callers. v0.7.24 lists a public repo's tags with no connection (#182). agni's move is panyam/agni#1788, which blocks agni#1778.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.
- This run: PR #201 added `make vulncheck` and a weekly scan (panyam/ci-actions#11), and moved x/net to v0.60.0, unreleased. No branch threads are open.

## Across threads

- Closed missions (#33, #34, #60, #74, #91, #150) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
- goapplib requires oneauth v0.3.13, and storage requires oneauth/oauth2 v0.3.13. Every go.mod keeps `go 1.26.9` as importers' floor and builds on `toolchain go1.27.4`. thambura, diffpp and lilbattle get an upgrade ticket when they next bump goapplib.
