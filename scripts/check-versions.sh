#!/usr/bin/env bash
# goapplib, @panyam/tsappkit, @panyam/tsappkit-solid and @panyam/tsappkit-dock share one version, the
# one in VERSION, and so do the nested Go modules. Fails when a package.json says otherwise, when a
# nested module requires another of this repo's modules at a different version, and, given a tag
# (v0.6.0), when the tag says otherwise too.
# CI runs it on every PR; publish.yml runs it with the tag before publishing.
#
#   scripts/check-versions.sh [tag]
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
want=$(tr -d '[:space:]' < "$root/VERSION")
[ -n "$want" ] || { echo "VERSION is empty" >&2; exit 1; }

bad=0
for dir in tsappkit tsappkit-solid tsappkit-dock; do
  got=$(node -p "require('$root/$dir/package.json').version")
  if [ "$got" != "$want" ]; then
    echo "$dir/package.json is $got; VERSION is $want" >&2
    bad=1
  fi
done
# A nested module that requires another of this repo's modules requires it at vVERSION, since a
# release tags them all on one commit. Until then a replace in its go.mod points at the copy here.
for m in $("$root/scripts/go-modules.sh"); do
  reqs=$(cd "$root/$m" && go mod edit -json | node -e '
    const j = JSON.parse(require("fs").readFileSync(0, "utf8"));
    for (const r of j.Require || []) if (r.Path === "github.com/panyam/goapplib" || r.Path.startsWith("github.com/panyam/goapplib/")) console.log(r.Path + " " + r.Version);
  ')
  while read -r dep ver; do
    [ -z "$dep" ] && continue
    if [ "$ver" != "v$want" ]; then
      echo "$m/go.mod requires $dep $ver; VERSION is $want" >&2
      bad=1
    fi
  done <<< "$reqs"
done
if [ $# -gt 0 ] && [ "${1#v}" != "$want" ]; then
  echo "tag $1 doesn't match VERSION's $want" >&2
  bad=1
fi
[ $bad = 0 ] && echo "all at $want"
exit $bad
