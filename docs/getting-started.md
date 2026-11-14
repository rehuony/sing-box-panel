# Getting started

This guide is for contributors building sing-box-panel and operators starting
one local development instance. The supported production targets are
`linux/amd64` and `linux/arm64`.

## Build the application

Source builds require Go 1.26, Node.js 22.12 or newer, Corepack, and the
package-pinned pnpm 11.21.0. Go and Web dependencies are locked by `go.mod`,
`go.sum`, and `web/pnpm-lock.yaml`.

From the repository root:

```sh
make deps
make check build
```

`make check` builds the Web assets (including TypeScript checking), verifies Go
formatting and modules, and runs `go vet`, Go tests, Web linting and tests,
third-party notice checks, offline configuration-schema validation, OpenAPI
checks, shell syntax checks, and network-independent installer contract tests.
`make check-backend`, `make check-frontend`, and `make check-contracts` run the individual
groups described in [Contributing](../.github/CONTRIBUTING.md#validation).
`make build` uses the same Web-first boundary and writes `bin/sing-box-panel` and
`bin/sing-box-node`; plain `make` lists the available targets. Go compilation requires the generated `web/apps/panel/dist` and `web/apps/node/dist` trees;
that generated tree is the only UI source accepted by the Go build.

## Initialize settings and storage

To prepare and inspect settings before startup, initialize them explicitly:

```sh
./bin/sing-box-panel init --config ./setting.json
./bin/sing-box-panel config verify --config ./setting.json
```

To generate only the default configuration file, use
`./bin/sing-box-panel config init --config ./setting.json`. It prints the generated
administrator email and random password and leaves the data directory and database untouched. Existing files
are preserved unless `--force` is explicitly requested.

`init` performs the following operations:

- creates `admin@example.com` and a random password; stores only its Argon2id hash;
- writes the settings atomically with mode `0600`;
- creates the data directory with mode `0700`; and
- creates `panel.db` with the current schema in that data directory.

Do not commit the settings file, administrator password, database, exported
configuration, or subscription data.

### Database compatibility

The panel database uses SQLite `application_id = 0x53425034` and storage schema
version 22, defined in `internal/sing-box-panel/store/schema.sql`; each node
keeps its own `node.db` with `application_id = 0x5342504E` and storage schema
version 2, defined in
`internal/sing-box-node/store/schema.sql`. Empty databases are initialized
directly. Each schema is a storage baseline with no upgrade path:
databases written by earlier versions, unidentified databases, other application
identities and newer schemas are rejected without changing their data. Use a
fresh data directory when the stored schema is older. The panel never deletes an
unsupported database.
Panel settings are read from the selected `setting.json` only.

Configuration revisions contain a sing-box JSON object directly. SQLite's
storage schema version describes panel tables and is unrelated to sing-box
configuration or JSON Schema versions.

### Settings selection

The persistent `-c, --config PATH` flag always selects the settings file for a
command. Without that flag, the default path is:

- `/etc/sing-box-panel/setting.json` for root;
- `$XDG_CONFIG_HOME/sing-box-panel/setting.json` for another user; or
- `~/.config/sing-box-panel/setting.json` when `XDG_CONFIG_HOME` is unset.

The default data directory is `/var/lib/sing-box-panel` for root. For another
user it is `$XDG_DATA_HOME/sing-box-panel`, or
`~/.local/share/sing-box-panel` when `XDG_DATA_HOME` is unset. A relative
`data_dir` in an explicit settings file is resolved relative to that file.

The settings file is the single source for all panel settings. The Web UI,
`config init/show/get/set/verify`, and manual edits use this same file. Shared fields retain
their existing sections; `panel` holds the language and appearance. The Web form exposes service paths, version check interval, metric retention and sing-box log retention; each node's published host and traffic quota belong to that node's settings. Panel events are retained indefinitely; subscription source policy remains a file/API setting. Inbound credentials are edited directly in the native sing-box configuration. Changing `data_dir` moves
existing storage on the next explicit start, with interruption recovery.
Sing-box documents, subscriptions and runtime evidence remain in SQLite.
See the [complete field mapping](configuration-and-runtime.md#shared-settings-file).
To edit one field without replacing the document, use
`./bin/sing-box-panel --config ./setting.json config set server.port 9090`.
Read its stored value with `config get server.port`. Field keys use dots as
separators. String values are literal text; numbers, arrays and objects use JSON.
Clear an optional string with `config set github.token ''`. Use `config set auth.password --stdin`
to hash and save a custom administrator password without echoing it. See
[Panel settings](cli.md#panel-settings) for field types and password handling.
Only active settings are generated. Removed subscription author/provider and
panel event retention fields are rejected; see the field mapping above for the
breaking development change.

`traffic.sample_retention_days` defaults to 90. It is required for startup and
`config verify` and must be between 1 and 366;
older settings without it are rejected instead of receiving a compatibility
default. Commands that only locate instance files or data validate `data_dir`
without validating unrelated runtime fields; see [CLI configuration dependencies](cli.md#global-flags-and-output).

When the panel is served through a reverse proxy, set `server.external_origin`
to the single public HTTP origin, for example `https://panel.example.com`.
Session cookies automatically use `Secure` for an HTTPS external origin. Without
an external origin, they use the request's actual TLS state; forwarded headers
are not trusted. The origin contains no path—continue to use `server.base_path`
for a public path prefix. Origin changes take effect after a server restart.
The removed `auth.secure_cookie` field is rejected: delete it from existing
settings files and backups before using them with this version.

Login allows five attempts per minute per direct peer and, when available,
per browser fingerprint, with a process-wide budget of 30 and two concurrent
logins. Successful attempts also count. A 429 response includes `Retry-After`;
wait before trying again. Behind a reverse proxy or NAT, users share the direct
peer's budget; forwarded IP headers do not change it. Fingerprint collection is
local and optional, with no third-party telemetry. See the
[authentication contract](architecture.md#management-authentication).

The generated listener is `127.0.0.1:3000`. Change `data_dir` to an absolute,
empty directory when a test must also isolate the database.

## Start the server

```sh
./bin/sing-box-panel server start --config ./setting.json
```

If the selected file is absent, `server start` creates default settings and an
administrator email and random password automatically, then initializes storage and starts the
panel. The explicit `init` step is optional. Existing files are validated without
replacement; broken or unreadable settings still fail. The same behavior applies
without `--config`, using the default path for the current user.

First-run guidance lists `Panel URL`, `Email`, `Initial Password`,
`Settings`, and `Data Dir`. Open the URL and sign in with the
printed email and password. The password is displayed only when settings are
created; `auth.password_hash` stores its hash. Existing token-based settings using
`auth.token` are rejected: initialize current-format settings and migrate the
other options. There is no automatic conversion.
That initial summary confirms settings creation. After binding the listener, every
start prints the actual panel URL, settings and data paths, Web logs guidance and stop
hint, then streams sanitized panel events. Redirected output omits ANSI color;
`NO_COLOR` disables it in terminals and `--output json` emits structured events.

If the password is lost, run the following on the server using the same configuration
path and an OS account permitted to update it:

```sh
./bin/sing-box-panel --config ./setting.json config set auth.password --stdin < ./password.txt
```

Supply a new password containing 12–128 Unicode characters in `password.txt`.
The command reads it without echoing it in output, preserves the email and
other configuration, and stores only its Argon2id hash. Existing sessions become invalid
at their next authentication check. It works while the server is stopped or running
and requires no restart. See the [CLI reference](cli.md) for input handling.

This runs in the foreground. Stop it with `Ctrl+C`, or run
`./bin/sing-box-panel server stop --config ./setting.json` in another terminal.
Use `server status` with the same settings to inspect the process. For
background operation, install and start the systemd service instead.

The server exposes the embedded UI, the management API and the node service on
one port, and runs no sing-box. Configuration checks, enabling or disabling a
core, start, stop, restart, and rollback through the authenticated API are
commands the panel sends to a node, so that node must be online; saving a
node's configuration works while it is offline. Manage nodes, catalogs,
installed cores, subscriptions, logs, and metrics in the Web UI. The CLI provides panel settings, node registration, service and
installation maintenance; its panel status/stop controls use the owner-only Unix socket.

Source refresh, catalog refresh and retention keep their own bounded schedules
on the panel; each node recovers its own runtime. There is no generic operation
queue or detached polling API.

## Add a node

The panel manages sing-box only on nodes. Add one in the Web UI's Nodes view,
or with `./bin/sing-box-panel node add NAME --config ./setting.json`. Either
shows a one-time enrollment command that is valid for one hour. Run it on the
node's Linux host, then start the node:

```sh
./bin/sing-box-node enroll --code <code>
./bin/sing-box-node run
```

The node dials the panel, so the code must name an address the node can reach:
the generated loopback listener serves a node on the same host, and a node on
another host needs `nodes.endpoint` or `server.external_origin`. See
[Nodes](cli.md#nodes) for the node's data directory and states, and
[Panel and node architecture](panel-node-architecture.md) for enrollment and
trust.

## Edit the sing-box configuration

Log in to the Web UI, open the node and select its Configuration tab. The panel
keeps one sing-box document per node, logically named `config.json`, as text in
the `node_configurations` table of `panel.db`. The Web editor manages it; there
is no separately editable file on disk or CLI for its content. Adding a node
saves this minimal document as its first configuration:

```json
{}
```

You can install and enable a core on the node immediately without first saving
a configuration manually. Edit and save the configuration in the Web editor,
then use Validate configuration to check the saved text and Start or Restart
to load it. The saved text, including invalid or unfinished text, is kept
across panel and node restarts.
Concurrent saves use the current file revision and reject stale edits instead
of merging implicitly. Invalid JSON can be saved as text but blocks validation,
Enable, Start, and Restart until corrected. Core lifecycle and artifact management
use the Web UI or authenticated HTTP API. The `config init/show/get/set/verify` commands manage
only the panel's `setting.json`; see [Panel settings](cli.md#panel-settings).

Continue with [Core versions](core-versions.md), then
[Configuration and runtime](configuration-and-runtime.md).

## Install a systemd service

The CLI can install an audited per-user or system service on Linux. User scope
uses the current executable and the current user's XDG paths. System scope
requires root and the fixed release layout under `/usr/local`, `/etc`, and
`/var/lib`.

```sh
sing-box-panel systemd install --scope=user --now
sing-box-panel systemd status --scope=user
```

`systemd status` reports systemd's unit state together with the settings path
written in the unit file on disk, the CLI's own `--config` path, and the data
directory, database, and configuration storage declared by that settings file
as it exists now. Each value names its on-disk source; the settings of the
running process are not inspected and are reported as unknown, and a unit file
edited since systemd loaded it is flagged as stale.
`--scope=auto` selects `system` for root and `user` otherwise. Each node host
installs its own service with `sing-box-node systemd install`. The node's system
unit grants `CAP_DAC_READ_SEARCH` so sing-box can read root-owned certificates
without changing their ownership or permissions; the panel's units and the
node's user unit grant no capabilities. The node's user unit does not confine
the file system: a user-scope node can write wherever its user can. TUN,
transparent proxying, raw sockets, and privileged ports on a node require a
reviewed local override. See the authoritative
[systemd packaging guide](systemd.md) before deploying a
system service.

## Update the panel and nodes

A release build updates itself to a newer signed release. The panel settings'
maintenance category shows the panel's version and the newest stable release
and updates the panel; a node's settings tab updates that node to the panel's
own release, and nodes never contact GitHub. Restarting a node restarts its
core, so its proxy is briefly interrupted. From a shell, `sing-box-panel update`
and `sing-box-node update` do the same; see the [CLI reference](cli.md). A
system service hands each update to its root update helper, described in the
[systemd packaging guide](systemd.md#update-helper).
