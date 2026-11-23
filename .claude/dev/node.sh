#!/bin/sh
# Runs a development node, enrolled with the development panel and with its
# console on 127.0.0.1:3001. The node runtime needs Linux, so the node runs
# in a Debian container on the host network. Its binary is shared in
# .claude/dev/state/bin/linux; its data stays in
# .claude/dev/state/node-<generation> (ignored), the generation of the panel
# it enrolls with, such as p1a2b3cn4d5e6f; earlier generations stay untouched
# on disk.
# Start the panel first. The first start of a generation adds or re-enrolls
# dev-node with that panel and prints the console password once.
set -eu
cd "$(dirname "$0")/../.."
state=.claude/dev/state

# The generation names the two storage baselines by a short digest of each
# baseline migration: later migrations upgrade a generation's data in place.
baseline() { shasum -a 256 "$1" | cut -c1-6; }
panel_schema=$(baseline internal/sing-box-panel/store/migrations/0001_baseline.sql)
node_schema=$(baseline internal/sing-box-node/store/migrations/0001_baseline.sql)
generation=p${panel_schema}n${node_schema}
instance=$state/node-$generation

image=debian:trixie-slim
name=sbp-dev-node
binaries="$PWD/$state/bin/linux:/opt/sbp:ro"
data="$PWD/$instance:/var/lib/sing-box-node"

[ -f web/apps/node/dist/index.html ] || make build-frontend
mkdir -p "$state/bin/linux" "$instance"
GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -o "$state/bin/linux/" ./cmd/sing-box-node

if [ ! -f "$instance/enrollment.json" ]; then
  go build -o "$state/bin/" ./cmd/sing-box-panel
  settings=$state/panel-$generation/setting.json
  # A node left registered by an earlier data directory gets a new code.
  issued=$("$state/bin/sing-box-panel" node add dev-node --config "$settings" -o json 2>/dev/null ||
    "$state/bin/sing-box-panel" node enroll dev-node --config "$settings" -o json)
  code=$(printf '%s' "$issued" | sed -n 's/.*"code": *"\([^"]*\)".*/\1/p')
  docker run --rm --network host -v "$binaries" -v "$data" "$image" \
    /opt/sbp/sing-box-node enroll --data-dir /var/lib/sing-box-node --code "$code"
fi
if [ ! -f "$instance/console.json" ]; then
  docker run --rm -v "$binaries" -v "$data" "$image" \
    /opt/sbp/sing-box-node console enable --data-dir /var/lib/sing-box-node
fi

docker rm -f "$name" >/dev/null 2>&1 || true
exec docker run --rm --name "$name" --network host -v "$binaries" -v "$data" "$image" \
  /opt/sbp/sing-box-node run --data-dir /var/lib/sing-box-node
