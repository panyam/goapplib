#!/usr/bin/env bash
# Publishes each TS package whose version isn't on npm yet, in order (tsappkit
# before tsappkit-solid, which depends on it). Run by .github/workflows/publish.yml
# on a v* tag; CI runs it with DRY_RUN=1 on every PR.
#
# A version that's already on npm is compared with what this checkout would
# pack. Same contents: skipped. Different contents: fails, because the package
# changed without a version bump and a publish would silently skip it.
#
# A publish is done once npm accepts it (`npm publish` exits 0). npm can take minutes to show a new
# version (v0.6.8's tsappkit-solid took 29), so the script waits up to 2 minutes to say it's visible
# and otherwise warns and carries on, rather than failing a release npm has already taken.
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
  for _ in $(seq 1 8); do
    if [ "$(npm view "$name@$version" version --prefer-online 2>/dev/null)" = "$version" ]; then
      echo "   published, and npm shows it"
      continue 2
    fi
    sleep 15
  done
  echo "   published (npm accepted it), but npm doesn't show $name@$version yet; it can take half an hour" >&2
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::warning::npm accepted $name@$version but doesn't show it yet; check https://registry.npmjs.org/$name/$version later"
  fi
done
