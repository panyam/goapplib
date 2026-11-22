# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. goapplib's storage work for agni's `mission_linked_sources` (panyam/agni#1722) shipped in v0.7.26: Git LFS, S3 and GCS mounts and blob stores, bucket events, SharePoint and OneDrive, and `mounts.Credentials` in the driver API. agni moves to it in panyam/agni#1816, which blocks agni#1809, #1730 and #1729.
- main is at 0.7.27, untagged. Since v0.7.26 it adds #221 (GitHub `ListAt` and `FetchAt` agree on LFS sizes, for agni#1809), #222 (Entra workload identity and `WithIdentityEndpoint`, for agni#1729) and #224 (`blobs.Cached` and `WithViews` for bucket snapshots, for agni#1730 and #1729). Next: tag v0.7.27 and file agni's upgrade ticket. v0.7.26 is the latest tag, and npm has all three packages at 0.7.26. thambura, diffpp and lilbattle get an upgrade ticket when they next bump goapplib. The `Credentials` change breaks `WithCredentials` and direct driver callers.
- Waiting on consumers to port onto tsappkit-dock 0.7.11: panyam/diffpp#172 and turnforge/lilbattle#203. lilbattle#203 also moves the mobile viewer off the deprecated `MobileBottomDrawer`, and needs lilbattle's dockview 1.17 upgraded to 4.13+ first. lilbattle#201 (the v0.7.3 upgrade) is still open. #146 waits on lilbattle's mobile port.
- Waiting: content services (#6–#9, #13–#15, #17), #18, #19, #28, #29, #43, #44, #49, #56, #64, #82, #83, #118, #146, #223 (parallel fetches and live tests for bucket views), #225 (lazy bucket views).
- This run: #224 merged, closing #219 and #220; dropped feat/blobs-cached-bucket-views. No branch threads are open.

## Across threads

- Closed missions (#33, #34, #60, #74, #91, #150) keep their exercises in CI as regression checks. `mission_server_nav` (#40, #38) was considered on 2026-10-06 and not started, since no consumer asked for it.
- goapplib requires oneauth v0.3.16, and storage requires oneauth/oauth2 v0.3.16. Every go.mod keeps `go 1.26.9` as importers' floor and builds on `toolchain go1.27.4`. thambura, diffpp and lilbattle get an upgrade ticket when they next bump goapplib.
- The docsite's `islands-presenter` demo failed in CI on #211 and #221, and passed on a rerun each time.
- CodeQL alert 9 (`go/request-forgery` on the SNS certificate fetch) is dismissed as a false positive, as alert 7 was. The URL is checked against `sns.<region>.amazonaws.com` before any request.
