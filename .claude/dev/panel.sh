#!/bin/sh
# Runs a development panel on 127.0.0.1:3000. Its binary is built into
# .claude/dev/state/bin; its settings and data stay in .claude/dev/state/panel
# (ignored). The first start saves the administrator email and password to
# administrator.json (mode 0600) there. Remove that directory to start over.
set -eu
cd "$(dirname "$0")/../.."
state=.claude/dev/state
instance=$state/panel
settings=$instance/setting.json

# The panel embeds its Web build; any build will do, Vite serves the pages.
[ -f web/apps/panel/dist/index.html ] || make build-frontend
go build -o "$state/bin/" ./cmd/sing-box-panel

if [ ! -f "$settings" ]; then
  mkdir -p "$instance"
  (umask 077 && "$state/bin/sing-box-panel" config init --config "$settings" -o json >"$instance/administrator.json")
  # Relative to the settings file: $instance/data.
  "$state/bin/sing-box-panel" config set data_dir data --config "$settings" >/dev/null
  echo "Administrator credentials saved to $instance/administrator.json"
fi
exec "$state/bin/sing-box-panel" server start --config "$settings"
