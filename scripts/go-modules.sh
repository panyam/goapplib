#!/usr/bin/env bash
# Prints the directory of each nested Go module this repo publishes, one per line, relative to
# the repo root. Each is tagged <dir>/vX.Y.Z beside vX.Y.Z (release.sh) and tested by make test.
# docsite is a module too, but it is the site and isn't published.
#
#   scripts/go-modules.sh
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
git ls-files --cached --others --exclude-standard '*/go.mod' | xargs -n1 dirname | { grep -v '^docsite$' || true; } | sort
