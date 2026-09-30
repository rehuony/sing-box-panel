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
make bootstrap
make check build
```

`make check` builds the Web assets (including TypeScript checking), verifies Go
formatting and modules, and runs `go vet`, Go tests, Web linting and tests,
third-party notice checks, offline configuration-schema validation, OpenAPI
checks, shell syntax checks, and network-independent installer contract tests.
`make check-go`, `make check-web`, and `make check-contracts` run the individual
groups described in [Contributing](../CONTRIBUTING.md#validation).
`make build` (also the default for plain `make`) uses the same Web-first boundary
and writes `bin/sing-box-panel`. Go compilation requires the generated `web/dist` tree;
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

The current application uses SQLite `application_id = 0x53425034` and storage
schema version 17, defined in `internal/store/schema.sql`,
`internal/store/traffic_months.sql`, `internal/store/traffic_checkpoint.sql` and
`internal/store/subscription_node_orders.sql`. Empty databases are initialized directly.
Versions 11–15 of the same application identity upgrade transactionally to
version 17. Version 11 first adds durable monthly traffic totals; version 13
removes obsolete `export_token_ids` from channel configuration while preserving
all other channel data. Version 14 removes the retired channel organizer and
independent group defaults and retains card order. Rule indices default to zero.
Version 15 preserves recorded traffic while removing monthly completeness state
and redundant checkpoint totals. Historical usage is not recalculated.
Version 16 adds shared subscription node order; browser-local orders are not imported.
Version 17 adds persistent administrator sessions while retaining business data.
API clients must stop submitting retired fields and read recorded period usage
directly instead of using the removed traffic availability/coverage fields. Unidentified
databases, other application identities, and unsupported older or newer schemas
are rejected without changing their data. The panel never deletes an unsupported database.
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
`config init/show/set/unset/verify`, and manual edits use this same file. Shared fields retain
their existing sections; `panel` adds the public node host, language and appearance. The Web form exposes service paths, version check interval, metric retention and sing-box log retention. Panel events are retained indefinitely; subscription source policy remains a file/API setting. Inbound credentials are edited directly in the native sing-box configuration. Changing `data_dir` moves
existing storage on the next explicit start, with interruption recovery.
Sing-box documents, subscriptions and runtime evidence remain in SQLite.
See the [complete field mapping](guides/configuration-and-runtime.md#shared-settings-file).
Only active settings are generated. Removed subscription author/provider and
panel event retention fields are rejected; see the field mapping above for the
breaking development change.

`traffic.sample_retention_days` defaults to 90. It is required for startup and
`config verify` and must be between 1 and 366;
older settings without it are rejected instead of receiving a compatibility
default. Commands that only locate instance files or data validate `data_dir`
without validating unrelated runtime fields; see [CLI configuration dependencies](guides/cli.md#global-flags-and-output).

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
[authentication contract](development/architecture.md#management-authentication).

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
created; `auth.password_hash` stores its hash. Existing token-based settings are
rejected: replace `auth.token` with `auth.email` and `auth.password_hash` manually,
using `config hash-password` to generate the hash. There is no automatic conversion.
That initial summary confirms settings creation. After binding the listener, every
start prints the actual panel URL, settings and data paths, stored-log command and stop
hint, then streams sanitized panel events. Redirected output omits ANSI color;
`NO_COLOR` disables it in terminals and `--output json` emits structured events.

If the password is lost, run the following on the server using the same configuration
path and an OS account permitted to update it:

```sh
./bin/sing-box-panel --config ./setting.json config reset-password
```

The command generates and displays a new password once, preserves the email and
other configuration, and stores only its hash. Existing sessions become invalid
at their next authentication check. It works while the server is stopped or running
and requires no restart. To supply a custom password without echoing it, use
`config reset-password --stdin`; see the [CLI reference](guides/cli.md).

This runs in the foreground. Stop it with `Ctrl+C`, or run
`./bin/sing-box-panel server stop --config ./setting.json` in another terminal.
Use `server status` with the same settings to inspect the process. For
background operation, install and start the systemd service instead.

The server exposes the embedded UI and management API and exclusively owns the
core process. Keep it active for configuration checks, enabling/disabling a core,
and start/stop/restart/rollback. Local CLI controls use its owner-only Unix socket.
Catalog refresh, core installation/import, and subscription refresh execute in the
calling CLI process and do not require a running server. Commands return their
completed result; interrupting the caller cancels active work at a safe boundary.

Runtime recovery and configured source refresh keep their own bounded schedules.
There is no generic operation queue or detached polling API.

## Edit the sing-box configuration

Log in to the Web UI and open Configuration. The panel keeps one sing-box
document, logically named `config.json`, as text in the `configuration_file`
table of `panel.db`. The Web editor manages it; there is no separately editable
file on disk or CLI for its content. Before accepting requests, startup creates
and saves this minimal document if no configuration exists:

```json
{}
```

You can install and enable a core immediately without first saving an empty
configuration manually. Edit and save the configuration in the Web editor,
then use Validate configuration to check the saved text and Start or Restart
to load it. Repeated starts preserve existing configuration, including invalid
or unfinished text.
Concurrent saves use the current file revision and reject stale edits instead
of merging implicitly. Invalid JSON can be saved as text but blocks validation,
Enable, Start, and Restart until corrected. Core lifecycle and artifact commands remain
available through the CLI. The separate `config init/show/set/unset/verify` commands manage
only the panel's `setting.json`; see [Panel settings](guides/cli.md#panel-settings).

Continue with [Core versions](guides/core-versions.md), then
[Configuration and runtime](guides/configuration-and-runtime.md).

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
`--scope=auto` selects `system` for root and `user` otherwise. The system unit
grants `CAP_DAC_READ_SEARCH` for reading root-owned certificates without changing
their ownership or permissions; the user unit grants no capabilities.
TUN, transparent proxying, raw sockets, and
privileged ports require a reviewed local override. See the authoritative
[systemd packaging guide](../systemd/README.md) before deploying a
system service.
