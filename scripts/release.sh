#!/usr/bin/env bash
# Cuts a release from a clean, up-to-date main. It sets VERSION and the three package.json
# versions, pins the docs to the new version (pin-docs.sh), and commits "Release vX.Y.Z" if
# anything changed. Then it tags, pushes main and the tag, and creates the GitHub release when
# given a notes file. The tag runs publish.yml, which publishes the npm packages.
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
! git rev-parse -q --verify "refs/tags/v$v" >/dev/null || { echo "tag v$v exists" >&2; exit 1; }

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
git tag -a "v$v" -m "v$v"
git push -q origin main "v$v"
echo "pushed main and v$v; publish.yml runs on the tag"

if [ -n "$notes" ]; then
  gh release create "v$v" --notes-file "$notes" --verify-tag
else
  echo "next: gh release create v$v --notes-file <notes> --verify-tag"
fi
