#!/usr/bin/env bash
# Points the docs' version pins at a release: the README's and getting-started's `go get` and
# templar `ref:`, and the "X@0.7.10 goes with goapplib v0.7.10" lines in each package README.
# release.sh runs it before the release commit. Run it alone to fix pins by hand.
#
#   scripts/pin-docs.sh 0.7.12
set -euo pipefail

[ $# = 1 ] || { echo "usage: $0 X.Y.Z" >&2; exit 2; }
v=${1#v}
[[ $v =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "not a version: $1" >&2; exit 2; }

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
semver='[0-9]+\.[0-9]+\.[0-9]+'

# go get github.com/panyam/goapplib@vX.Y.Z
perl -pi -e "s{goapplib\@v$semver}{goapplib\@v$v}g" README.md docsite/content/guide/getting-started.md
# templar.yaml's `ref: vX.Y.Z`
perl -pi -e "s{^(\s*ref: )v$semver}{\${1}v$v}" docsite/content/guide/getting-started.md
# `@panyam/tsappkit@X.Y.Z` and goapplib `vX.Y.Z` in the npm READMEs
perl -pi -e "s{(\@panyam/tsappkit(?:-solid|-dock)?\@)$semver}{\${1}$v}g; s{goapplib \`v$semver\`}{goapplib \`v$v\`}g" \
  tsappkit/README.md tsappkit-solid/README.md tsappkit-dock/README.md

echo "docs pinned to v$v"
