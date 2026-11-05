#!/bin/sh
# Runs a development panel on 127.0.0.1:3000. Its binary, settings and data
# stay in .claude/dev/state (ignored). The first start prints the
# administrator email and password once.
set -eu
cd "$(dirname "$0")/../.."
state=.claude/dev/state
settings=$state/panel/setting.json

# The panel embeds its Web build; any build will do, Vite serves the pages.
[ -f web/apps/panel/dist/index.html ] || make build-frontend
go build -o "$state/bin/" ./cmd/sing-box-panel

if [ ! -f "$settings" ]; then
  mkdir -p "$state/panel"
  "$state/bin/sing-box-panel" config init --config "$settings"
  # Relative to the settings file: .claude/dev/state/panel/data.
  "$state/bin/sing-box-panel" config set data_dir data --config "$settings"
fi
exec "$state/bin/sing-box-panel" server start --config "$settings"
