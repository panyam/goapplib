#!/usr/bin/env bash
# goapplib, @panyam/tsappkit and @panyam/tsappkit-solid share one version, the
# one under "## Version" in CAPABILITIES.md. Fails when either package.json
# says otherwise, and, given a tag (v0.6.0), when the tag says otherwise too.
# CI runs it on every PR; publish.yml runs it with the tag before publishing.
#
#   scripts/check-versions.sh [tag]
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
want=$(awk '/^## Version/ { getline; print; exit }' "$root/CAPABILITIES.md" | tr -d '[:space:]')
[ -n "$want" ] || { echo "no version under '## Version' in CAPABILITIES.md" >&2; exit 1; }

bad=0
for dir in tsappkit tsappkit-solid; do
  got=$(node -p "require('$root/$dir/package.json').version")
  if [ "$got" != "$want" ]; then
    echo "$dir/package.json is $got; CAPABILITIES.md is $want" >&2
    bad=1
  fi
done
if [ $# -gt 0 ] && [ "${1#v}" != "$want" ]; then
  echo "tag $1 doesn't match CAPABILITIES.md's $want" >&2
  bad=1
fi
[ $bad = 0 ] && echo "all at $want"
exit $bad
