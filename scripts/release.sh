#!/usr/bin/env bash
# Cuts a release from a clean, up-to-date main. It sets VERSION and the three package.json
# versions, pins the docs to the new version (pin-docs.sh), and commits "Release vX.Y.Z" if
# anything changed. Then it tags vX.Y.Z, plus <dir>/vX.Y.Z for each nested Go module
# (go-modules.sh), and pushes main and the tags. Given a notes file, it creates the GitHub release.
# The v tag runs publish.yml, which publishes the npm packages.
#
#   scripts/release.sh 0.7.12 [notes.md]
set -euo pipefail

[ $# -ge 1 ] || { echo "usage: $0 X.Y.Z [notes-file]" >&2; exit 2; }
v=${1#v}
notes=${2:-}
[[ $v =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "not a version: $1" >&2; exit 2; }
[ -z "$notes" ] || [ -f "$notes" ] || { echo "no notes file: $notes" >&2; exit 2; }

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

[ "$(git branch --show-current)" = main ] || { echo "not on main" >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo "working tree isn't clean" >&2; exit 1; }
git fetch -q origin main --tags
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || { echo "main isn't at origin/main" >&2; exit 1; }
tags=("v$v")
for m in $(scripts/go-modules.sh); do tags+=("$m/v$v"); done
for t in "${tags[@]}"; do
  ! git rev-parse -q --verify "refs/tags/$t" >/dev/null || { echo "tag $t exists" >&2; exit 1; }
done

echo "$v" > VERSION
for dir in tsappkit tsappkit-solid tsappkit-dock; do
  node -e '
    const fs = require("fs"), f = process.argv[1];
    const p = JSON.parse(fs.readFileSync(f, "utf8"));
    p.version = process.argv[2];
    fs.writeFileSync(f, JSON.stringify(p, null, 2) + "\n");
  ' "$dir/package.json" "$v"
done
scripts/pin-docs.sh "$v"
scripts/check-versions.sh "v$v"

if [ -n "$(git status --porcelain)" ]; then
  git diff --stat
  git commit -qam "Release v$v"
fi
for t in "${tags[@]}"; do git tag -a "$t" -m "$t"; done
git push -q origin main "${tags[@]}"
echo "pushed main and ${tags[*]}; publish.yml runs on v$v"

if [ -n "$notes" ]; then
  gh release create "v$v" --notes-file "$notes" --verify-tag
else
  echo "next: gh release create v$v --notes-file <notes> --verify-tag"
fi
