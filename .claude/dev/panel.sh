#!/bin/sh
# Runs a development panel on 127.0.0.1:3000. Its binary is shared in
# .claude/dev/state/bin; its settings and data stay in
# .claude/dev/state/panel-<generation> (ignored), where the generation names
# the panel and node storage baselines, such as p1a2b3cn4d5e6f. A new baseline
# starts a new generation and leaves earlier ones untouched on disk, so nothing
# has to be deleted. The first start of a generation saves the administrator email
# and password to administrator.json (mode 0600) in its directory.
set -eu
cd "$(dirname "$0")/../.."
state=.claude/dev/state

# The generation names the two storage baselines by a short digest of each
# baseline migration: later migrations upgrade a generation's data in place.
baseline() { shasum -a 256 "$1" | cut -c1-6; }
panel_schema=$(baseline internal/sing-box-panel/store/migrations/panel/0001_baseline.sql)
node_schema=$(baseline internal/sing-box-node/store/migrations/0001_baseline.sql)
instance=$state/panel-p${panel_schema}n${node_schema}
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
