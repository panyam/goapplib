# storage/

The storage module, `github.com/panyam/goapplib/storage`: mounts, connections and blobs. The design is on the docsite's reference pages (`storage-module`, `mounts`, `connections`, `blobs`), and the guide page `guide/storage` walks through the notes example in `docsite/examples/notes/`. Mission #150 built it.

## Layout and dependencies

- `mounts/` has the registry, the folder driver (`folder/`), the GitHub driver (`github/`), the bucket logic shared by S3 and GCS (`bucket/`), the folder record store (`fsstore/`) and the contract suites for record stores and drivers (`mountstest/`). `connections/` and `blobs/` follow the same shape. The S3 and GCS drivers are modules under `drivers/`.
- The root module never imports `storage`. A backend or driver that needs a third-party SDK gets its own module under `storage/backends/` or `storage/drivers/`, as GORM and Datastore did (#161).
- Each nested module requires `storage` at `VERSION` with a `replace` to the local folder. `scripts/check-versions.sh` fails when a require says otherwise, and `release.sh` moves the requires at release. `scripts/CLAUDE.md` has the steps.
- The S3 and GCS modules under `drivers/` hold both the mount driver and the blob store (`s3blobs`, `gcsblobs`), so each cloud's SDK is in one module. `blobs/blobstest` and `mounts/mountstest` are the contract suites a new store or driver runs.
- `make go-test` tests every nested module (`scripts/go-modules.sh` lists them).

## Gotchas

- Two fake GitHubs exist. `mounts/github/githubtest` serves repos (commits, tarballs, contents, hooks) and an App's sign-in, and checks tokens. oneauth's `outbound/githubtest` mints installation and user tokens for a connection. `AcceptToken` makes the first trust the second's tokens.
- The Datastore tests skip unless `DATASTORE_PROJECT_ID` is set. `make test-datastore` runs them against the local emulator, in a temp folder. Run from the repo root, the emulator writes `WEB-INF/appengine-generated/`, which then blocks `release.sh`'s clean-tree check.
- A connection's key is its owner and ID, and every call and mount names both (#197). A store change has to keep reading the records it wrote when the key was the ID alone, as each store's old-layout test checks.
- GORM on SQLite needs `_txlock=immediate` in the DSN, or concurrent creates fail with "database is locked". The tests set it.
- The folder driver's `IfMatch` check holds within one process only, and `mounts/fsstore` needs hard links to create a record. Both are on the mounts page.
- `TestLiveS3` and `TestLiveGCS` read a real bucket when `STORAGE_LIVE_S3` or `STORAGE_LIVE_GCS` names a source. gofakes3 ignores `If-Match` on a GET, so the S3 test wraps it, and only the live test checks S3's own 412.
- `TestLiveGitHub` (mounts/github) runs against api.github.com with `STORAGE_LIVE_GITHUB=1`. It's the only check of the tarball redirect, of files over the Contents API's 1 MB inline limit, and, with `STORAGE_LIVE_GITHUB_TOKEN` too (`gh auth token` works), of GitHub's tag order. Run it after changing what `githubtest` serves, since the fake can drift from GitHub (#177 found the tag order that way). `TestLiveGitHubLFS` needs `STORAGE_LIVE_GITHUB_LFS=owner/repo:path`, since no small public LFS fixture is known to stay put (#203).
- CodeQL follows request input through `Registry.Create` into the folder driver's `Resolve` (`go/path-injection`). It doesn't see that `Create` refuses `KindFolder` first. Alert 7 is dismissed as a false positive. A new path into a driver should get the same check before a dismissal.
