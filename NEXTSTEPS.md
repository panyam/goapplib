# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. The next goapplib work comes from agni's `mission_linked_sources` (panyam/agni#1722). #183 is filed for agni#1772's two storage pushdowns: Disconnect for a pasted-token connection, and per-repo token grants in `githubtest`. It's P2 and off-mission, and it's next. Ready off-mission P3s: #114 (a real cache bug), #134, #37, #38, #40, #147.
- v0.7.16 is released and is the latest tag. It closed #177, #178 and #179 (ListRefs, folder `mtime:` versions, push webhooks), and moved oneauth to v0.3.10 in every module (#181). agni's upgrade is panyam/agni#1775. It blocks agni#1725 and agni#1726, and it flags that a folder's version is now `mtime:...` where agni checks for 40 hex characters.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.
- This run: dropped the closed agni#1724 and agni#1728 lines and the v0.7.15 note. No branch threads are open.

## Across threads

- Closed missions (#33, #34, #60, #74, #91, #150) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
- goapplib v0.7.16 requires oneauth v0.3.10. thambura, diffpp and lilbattle get an upgrade ticket when they next bump goapplib.
