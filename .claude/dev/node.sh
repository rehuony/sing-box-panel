#!/bin/sh
# Runs a development node, enrolled with the development panel and with its
# console on 127.0.0.1:8000. The node runtime needs Linux, so the node runs
# in a Debian container on the host network. Its binary is built into
# .claude/dev/state/bin/linux; its data stays in .claude/dev/state/node
# (ignored). Start the panel first. The first start creates the node's
# settings, printing the console password once, and adds or re-enrolls
# dev-node with the panel. Remove that directory to start over.
set -eu
cd "$(dirname "$0")/../.."
state=.claude/dev/state
instance=$state/node

image=debian:trixie-slim
name=sbp-dev-node
binaries="$PWD/$state/bin/linux:/opt/sbp:ro"
data="$PWD/$instance:/var/lib/sing-box-node"
config=/var/lib/sing-box-node/setting.json

[ -f web/apps/node/dist/index.html ] || make build-frontend
mkdir -p "$state/bin/linux" "$instance"
GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -o "$state/bin/linux/" ./cmd/sing-box-node

if [ ! -f "$instance/setting.json" ]; then
  docker run --rm -v "$binaries" -v "$data" "$image" \
    /opt/sbp/sing-box-node config init --config "$config"
fi
if [ ! -f "$instance/enrollment.json" ]; then
  go build -o "$state/bin/" ./cmd/sing-box-panel
  settings=$state/panel/setting.json
  # A node the panel still lists from an earlier node directory gets a new code.
  issued=$("$state/bin/sing-box-panel" node add dev-node --config "$settings" -f json 2>/dev/null ||
    "$state/bin/sing-box-panel" node enroll dev-node --config "$settings" -f json)
  code=$(printf '%s' "$issued" | sed -n 's/.*"code": *"\([^"]*\)".*/\1/p')
  docker run --rm --network host -v "$binaries" -v "$data" "$image" \
    /opt/sbp/sing-box-node enroll --config "$config" --code "$code"
fi

docker rm -f "$name" >/dev/null 2>&1 || true
exec docker run --rm --name "$name" --network host -v "$binaries" -v "$data" "$image" \
  /opt/sbp/sing-box-node server start --config "$config"
