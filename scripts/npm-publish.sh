#!/usr/bin/env bash
# Publishes each TS package whose version isn't on npm yet, in order (tsappkit
# before tsappkit-solid, which depends on it). Run by .github/workflows/publish.yml
# on a v* tag; CI runs it with DRY_RUN=1 on every PR.
#
# A version that's already on npm is compared with what this checkout would
# pack. Same contents: skipped. Different contents: fails, because the package
# changed without a version bump and a publish would silently skip it.
#
# A publish isn't done until `npm view` shows it, which can take a few minutes.
#
#   scripts/npm-publish.sh [package-dir...]   (default: tsappkit tsappkit-solid)
#   DRY_RUN=1 scripts/npm-publish.sh          builds, tests and compares, publishes nothing
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
[ $# -gt 0 ] || set -- tsappkit tsappkit-solid
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

field() { node -p "require('$1/package.json').$2"; }

for dir in "$@"; do
  pkg="$root/$dir"
  name=$(field "$pkg" name)
  version=$(field "$pkg" version)
  echo "== $name@$version"

  (cd "$pkg" && pnpm install --frozen-lockfile >/dev/null && pnpm run build >/dev/null)
  mkdir -p "$work/$dir/local" "$work/$dir/published"
  tarball=$(cd "$pkg" && npm pack --ignore-scripts --pack-destination "$work/$dir" 2>/dev/null | tail -1)
  tar xzf "$work/$dir/$tarball" -C "$work/$dir/local"

  if [ "$(npm view "$name@$version" version 2>/dev/null)" = "$version" ]; then
    published=$(npm pack "$name@$version" --pack-destination "$work/$dir/published" 2>/dev/null | tail -1)
    tar xzf "$work/$dir/published/$published" -C "$work/$dir/published"
    # package.json is left out: pnpm publish rewrites it, and the version already matches.
    if diff -rq -x package.json "$work/$dir/local/package" "$work/$dir/published/package" >"$work/$dir/diff"; then
      echo "   already on npm with the same contents, skipping"
      continue
    fi
    echo "   $name changed since $version was published, so bump its version:" >&2
    sed -e "s#^Files $work/$dir/local/package/\([^ ]*\) and .* differ#     \1#" -e "s#^Only in $work/$dir/\([a-z]*\)/package\(.*\): #     only in \1 \2/#" "$work/$dir/diff" >&2
    exit 1
  fi

  (cd "$pkg" && pnpm test >/dev/null)
  if [ "${DRY_RUN:-}" = 1 ]; then
    echo "   would publish $tarball"
    continue
  fi
  npm publish "$work/$dir/$tarball" --provenance --access public
  for _ in $(seq 1 40); do
    if [ "$(npm view "$name@$version" version --prefer-online 2>/dev/null)" = "$version" ]; then
      echo "   published, and npm shows it"
      continue 2
    fi
    sleep 15
  done
  echo "   published, but npm still doesn't show $name@$version after 10 minutes" >&2
  exit 1
done
