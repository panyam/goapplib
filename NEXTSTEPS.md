# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. The next goapplib work comes from agni's `mission_linked_sources` (panyam/agni#1722). Ready off-mission P3s: #114 (a real cache bug), #134, #37, #38, #40, #147. #182 has no priority yet and needs triage.
- v0.7.18 is the latest tag. It added pasted-token Disconnect, githubtest grants and hooks, and mount webhooks (#183, #185). agni's upgrade is panyam/agni#1776.
- `main` is at 0.7.19, unreleased. It has githubtest's GitHub App sign-in (#187, PR #188), for agni#1772. After `/release`, add it to agni#1776.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.
- This run: dropped the #183 and v0.7.16 lines, since #183 closed and v0.7.18 replaced v0.7.16. No branch threads are open.

## Across threads

- Closed missions (#33, #34, #60, #74, #91, #150) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
- goapplib v0.7.18 requires oneauth v0.3.10, and storage's tests require oneauth/oauth2 v0.3.11. thambura, diffpp and lilbattle get an upgrade ticket when they next bump goapplib.
