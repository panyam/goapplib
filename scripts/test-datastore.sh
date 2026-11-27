#!/usr/bin/env bash
# Runs the Datastore tests (storage/backends/gae and dsmigrate) against a local emulator. It starts the
# emulator binary from gcloud's cloud-datastore-emulator component directly, so it needs that
# component and Java, but not gcloud's beta commands. CI skips these tests, since they need the
# emulator; run this before changing the Datastore backend.
#
#   scripts/test-datastore.sh
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
port=${DATASTORE_PORT:-8432}
emulator="$(gcloud info --format='value(installation.sdk_root)')/platform/cloud-datastore-emulator/cloud_datastore_emulator"
[ -x "$emulator" ] || { echo "no Datastore emulator; run: gcloud components install cloud-datastore-emulator" >&2; exit 1; }

# The emulator writes WEB-INF/ into its working folder, so it runs in a temp one.
work=$(mktemp -d)
log="$work/emulator.log"
(cd "$work" && exec "$emulator" start --host=localhost --port="$port" --store_on_disk=false --consistency=1.0 >"$log" 2>&1) &
pid=$!
trap 'kill $pid 2>/dev/null || true; pkill -f "CloudDatastore.*--port=$port" 2>/dev/null || true; rm -rf "$work"' EXIT
for _ in $(seq 1 60); do
  curl -sf "http://localhost:$port/" >/dev/null && break
  kill -0 $pid 2>/dev/null || { cat "$log" >&2; exit 1; }
  sleep 1
done

for dir in storage/backends/gae dsmigrate; do
  (cd "$root/$dir" && DATASTORE_EMULATOR_HOST="localhost:$port" DATASTORE_PROJECT_ID=goapplib-test go test -count=1 ./...)
done
