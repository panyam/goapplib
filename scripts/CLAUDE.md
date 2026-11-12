# Releasing and publishing

goapplib, `@panyam/tsappkit`, `@panyam/tsappkit-solid` and `@panyam/tsappkit-dock` share the version in `VERSION`. A PR that bumps one bumps all four, and the release tag is that version. `check-versions.sh` checks it in CI, and `publish.yml` runs it with the tag before publishing anything.

## Cutting a release

1. If the versions need setting, commit "Release vX.Y.Z" on main. Bumps are patches by default; ask before a minor.
2. `git tag -a vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z`. Push the tag before `gh release create`, or gh makes a lightweight tag at `main`'s head.
3. `gh release create vX.Y.Z --notes-file <notes> --verify-tag`. The release body is the notes; there's no CHANGELOG.
4. The tag runs `.github/workflows/publish.yml`, which runs `npm-publish.sh`. Watch it with `gh run watch`.
5. File an upgrade ticket in each consumer that needs the release: `panyam/thambura`, `panyam/agni`, `panyam/diffpp`, `turnforge/lilbattle`.

## How publishing works

- `npm-publish.sh` builds, tests and publishes each package whose version isn't on npm yet, tsappkit first. A version already on npm with the same contents is skipped. One with different contents fails, so the first PR after a release that changes a package bumps all four. CI runs `DRY_RUN=1 npm-publish.sh` on every PR.
- npm accepting the publish counts as success. The script waits 2 minutes for `npm view` and otherwise warns; npm can take up to half an hour (v0.6.8's tsappkit-solid took 29 minutes). Don't re-run a publish npm accepted, since it refuses the same version.
- Auth is npm trusted publishing, so there's no token. A failed run can be retried from the Actions tab. Each package names `panyam/goapplib` and `publish.yml` as its publisher, set with `npx npm@latest trust github <package> --repo panyam/goapplib --file publish.yml --allow-publish` (login and OTP). A new package can't get trust before it exists, so its first version fails with `E404 Not Found - PUT` and is published by hand (`npm publish --access public --otp=<code>`), then given the trust. tsappkit-dock went through this at 0.7.8.
- `check-readmes.mjs` (CI) fails when a package README, its npm page, doesn't name an entry point in `exports` or links a docsite page, heading or repo file that doesn't exist.
- The repo is Apache-2.0 since v0.6.9, and each npm package carries its own `LICENSE`.
