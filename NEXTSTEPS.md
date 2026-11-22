# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. Storage work for agni's `mission_linked_sources` (panyam/agni#1722) is in flight: agni issues 1809 (LFS), 1730 (buckets) and 1729 (SharePoint and OneDrive).
- main is at VERSION 0.7.26, untagged on purpose. It holds #207 (Git LFS in github mounts, oneauth v0.3.16), #209 (S3 and GCS mount drivers, `mounts.Credentials` in place of `oauth2.TokenSource` in every `Driver` method, #208) and #211 (S3 and GCS blob stores). Tag v0.7.26 once #212 and the #206 PR merge, then file agni upgrade tickets for 1809, 1730 and 1729. #209's `Credentials` change breaks `WithCredentials` and direct driver callers, so the agni ticket names `mounts.Bearer(ts)`.
- storage/bucket-events-210: PR #212, ready for review. Next: merge it.
- storage/msgraph-206: #206 in progress, no PR yet. Next: open its PR.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146.
- This run: merged #207, #209 and #211; dropped storage/lfs-203, storage/s3-gcs-204 and storage/bucket-blobs-205.

## Across threads

- Closed missions (#33, #34, #60, #74, #91, #150) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
- goapplib requires oneauth v0.3.16, and storage requires oneauth/oauth2 v0.3.16. Every go.mod keeps `go 1.26.9` as importers' floor and builds on `toolchain go1.27.4`. thambura, diffpp and lilbattle get an upgrade ticket when they next bump goapplib.
- The docsite's `islands-presenter` demo failed once in CI on #211 and passed on a rerun.

## storage/bucket-events-210

- **Last touched**: 2026-11-21 (worktree /Users/sri/newstack/goapplib/events-210)
- **Ticket**: #210 (PR #212)
- **Why**: turn S3 (SQS or SNS) and GCS (Pub/Sub) object events into `mounts.Change`, so agni sees a moved object without polling.
- **Where it stopped**: PR #212 is open with green tests. It adds `mounts.SourceContainer`, which `Registry.Following` uses. Google's two issuer forms are both accepted (commit 430dbe4).
- **Next action**: review and merge #212. If the #206 PR merges first, merge main in, since both edit `mounts/change.go`.
- **Open questions**: no real SNS or Pub/Sub delivery has gone through the parsers yet. The live tests wait on the trigger in #210.

## storage/msgraph-206

- **Last touched**: 2026-11-21 (worktree /Users/sri/newstack/goapplib/msgraph-206)
- **Ticket**: #206
- **Why**: mount SharePoint and OneDrive through Microsoft Graph, with `entra` connections (secret, certificate, managed identity, and a delegated `entra-user`), for agni 1729.
- **Where it stopped**: a background session is building it on the branch. No PR yet.
- **Next action**: open the PR, then check the certificate assertion and the managed-identity call, since nothing tests them against real Azure.
