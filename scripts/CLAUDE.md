# Releasing and publishing

goapplib, `@panyam/tsappkit`, `@panyam/tsappkit-solid` and `@panyam/tsappkit-dock` share the version in `VERSION`. A PR that bumps one bumps all four, and the release tag is that version. `check-versions.sh` checks it in CI, and `publish.yml` runs it with the tag before publishing anything.

## Cutting a release

1. Run `scripts/release.sh X.Y.Z [notes.md]` on a clean main. Bumps are patches by default; ask before a minor. The script sets `VERSION` and the package.json versions, and runs `pin-docs.sh`. It commits "Release vX.Y.Z" if anything changed, then tags, pushes main and the tag, and runs `gh release create --verify-tag` when given notes. The release body is the notes; there's no CHANGELOG.
2. `pin-docs.sh` moves the docs' version pins: `go get ...@vX.Y.Z` in README.md and getting-started, templar's `ref:`, and the version example in each package README. A new pin in the docs needs a pattern there. Don't pin the package READMEs between releases. They're npm pages, so a README change at a version already on npm fails `npm-publish.sh`'s same-contents check.
3. The tag runs `.github/workflows/publish.yml`, which runs `npm-publish.sh`. Watch it with `gh run watch`.
4. File an upgrade ticket in each consumer that needs the release: `panyam/thambura`, `panyam/agni`, `panyam/diffpp`, `turnforge/lilbattle`.

## How publishing works

- `npm-publish.sh` builds, tests and publishes each package whose version isn't on npm yet, tsappkit first. A version already on npm with the same contents is skipped. One with different contents fails, so the first PR after a release that changes a package bumps all four. CI runs `DRY_RUN=1 npm-publish.sh` on every PR.
- npm accepting the publish counts as success. The script waits 2 minutes for `npm view` and otherwise warns; npm can take up to half an hour (v0.6.8's tsappkit-solid took 29 minutes). Don't re-run a publish npm accepted, since it refuses the same version.
- Auth is npm trusted publishing, so there's no token. A failed run can be retried from the Actions tab. Each package names `panyam/goapplib` and `publish.yml` as its publisher, set with `npx npm@latest trust github <package> --repo panyam/goapplib --file publish.yml --allow-publish` (login and OTP). A new package can't get trust before it exists, so its first version fails with `E404 Not Found - PUT` and is published by hand (`npm publish --access public --otp=<code>`), then given the trust. tsappkit-dock went through this at 0.7.8.
- `check-readmes.mjs` (CI) fails when a package README, its npm page, doesn't name an entry point in `exports` or links a docsite page, heading or repo file that doesn't exist.
- The repo is Apache-2.0 since v0.6.9, and each npm package carries its own `LICENSE`.
