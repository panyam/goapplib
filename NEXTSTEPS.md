# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. Storage work for agni's `mission_linked_sources` (panyam/agni#1722) is merged: Git LFS (#207), S3 and GCS mounts with `mounts.Credentials` (#209), bucket blob stores (#211), bucket events (#212), SharePoint and OneDrive with `entra` connections (#213), plus fixes #215 and #216.
- main is at VERSION 0.7.26, untagged on purpose. Next: merge the docs PR #217, then tag v0.7.26 and file agni upgrade tickets for 1809, 1730 and 1729. #209's `Credentials` change breaks `WithCredentials` and direct driver callers, so the agni ticket names `mounts.Bearer(ts)`.
- docs/storage-guide-0.7.26: PR #217, the storage guide and module layout for those features. Next: merge it.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.
- This run: merged #212, #213, #215 and #216; dropped storage/bucket-events-210 and storage/msgraph-206.

## Across threads

- Closed missions (#33, #34, #60, #74, #91, #150) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
- goapplib requires oneauth v0.3.16, and storage requires oneauth/oauth2 v0.3.16. Every go.mod keeps `go 1.26.9` as importers' floor and builds on `toolchain go1.27.4`. thambura, diffpp and lilbattle get an upgrade ticket when they next bump goapplib.
- The docsite's `islands-presenter` demo failed once in CI on #211 and passed on a rerun.
- CodeQL alert 9 (`go/request-forgery` on the SNS certificate fetch) is dismissed as a false positive, as alert 7 was. The URL is checked against `sns.<region>.amazonaws.com` before any request.

## docs/storage-guide-0.7.26

- **Last touched**: 2026-11-21 (worktree /Users/sri/newstack/goapplib/docs-storage)
- **Ticket**: no issue (PR #217)
- **Why**: the guide and `reference/storage-module.md` were stale after the storage PRs; the reference pages were already updated.
- **Where it stopped**: PR open; docsite `make check` passes.
- **Next action**: merge #217, then tag v0.7.26.
