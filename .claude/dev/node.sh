#!/bin/sh
# Runs a development node, enrolled with the development panel and with its
# console on 127.0.0.1:3001. The node runtime needs Linux, so the node runs
# in a Debian container on the host network; its binary and data stay in
# .claude/dev/state (ignored). Start the panel first. The first start prints
# the console password once.
set -eu
cd "$(dirname "$0")/../.."
state=.claude/dev/state

image=debian:trixie-slim
name=sbp-dev-node
binaries="$PWD/$state/bin/linux:/opt/sbp:ro"
data="$PWD/$state/node:/var/lib/sing-box-node"

[ -f web/apps/node/dist/index.html ] || make build-frontend
mkdir -p "$state/bin/linux" "$state/node"
GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -o "$state/bin/linux/" ./cmd/sing-box-node

if [ ! -f "$state/node/enrollment.json" ]; then
  go build -o "$state/bin/" ./cmd/sing-box-panel
  settings=$state/panel/setting.json
  # A node left registered by an earlier data directory gets a new code.
  issued=$("$state/bin/sing-box-panel" node add dev-node --config "$settings" -o json 2>/dev/null ||
    "$state/bin/sing-box-panel" node enroll dev-node --config "$settings" -o json)
  code=$(printf '%s' "$issued" | sed -n 's/.*"code": *"\([^"]*\)".*/\1/p')
  docker run --rm --network host -v "$binaries" -v "$data" "$image" \
    /opt/sbp/sing-box-node enroll --data-dir /var/lib/sing-box-node --code "$code"
fi
if [ ! -f "$state/node/console.json" ]; then
  docker run --rm -v "$binaries" -v "$data" "$image" \
    /opt/sbp/sing-box-node console enable --data-dir /var/lib/sing-box-node
fi

docker rm -f "$name" >/dev/null 2>&1 || true
exec docker run --rm --name "$name" --network host -v "$binaries" -v "$data" "$image" \
  /opt/sbp/sing-box-node run --data-dir /var/lib/sing-box-node
