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
./bin/sing-box-panel config check --config ./setting.json
```

`init` performs the following operations:

- creates a random management token;
- writes the settings atomically with mode `0600`;
- creates the data directory with mode `0700`; and
- creates and migrates `panel.db` in that data directory.

Do not commit the settings file, management token, database, exported
configuration, or subscription data.

### Database compatibility

The current application uses SQLite `application_id = 0x53425034` and storage
schema version 7. Opening a new database applies the embedded migrations;
existing databases with this application identity migrate forward automatically.
Unidentified non-empty databases, previous application identities, and schemas
newer than the binary fail closed. Startup also transfers legacy panel settings
from SQLite to the selected settings file once; subsequent edits use that file.

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
`config show/set/check`, and manual edits use this same file. Shared fields retain
their existing sections; `panel` adds the public node host, protocol identity,
language and appearance. Web saves preserve fields not exposed by its form. Changing `data_dir` moves
existing storage on the next explicit start, with interruption recovery.
Sing-box documents, subscriptions, tasks and runtime evidence remain in SQLite.
See the [complete field mapping](configuration-and-runtime.md#shared-settings-file).
New settings initialize `subscription.provider` to `"default"`; existing files
retain their configured value.

`traffic.sample_retention_days` defaults to 90. It is required for startup and
`config check` and must be between 1 and 366;
older settings without it are rejected instead of receiving a compatibility
default. Commands that only locate instance files or data validate `data_dir`
without validating unrelated runtime fields; see [CLI configuration dependencies](cli.md#global-flags-and-output).

When the panel is served through a reverse proxy, set `server.external_origin`
to the single public HTTP origin, for example `https://panel.example.com`.
HTTPS origins require `auth.secure_cookie: true`; a secure cookie in turn
requires an HTTPS external origin. The origin contains no path—continue to use
`server.base_path` for a public path prefix.

The generated listener is `127.0.0.1:3000`. Change `data_dir` to an absolute,
empty directory when a test must also isolate the database.

## Start the server

```sh
./bin/sing-box-panel server start --config ./setting.json
```

If the selected file is absent, `server start` creates default settings and a
random management token automatically, then initializes storage and starts the
panel. The explicit `init` step is optional. Existing files are validated without
replacement; broken or unreadable settings still fail. The same behavior applies
without `--config`, using the default path for the current user.

First-run guidance lists the settings file, data directory, default URL, generated
`Login token`, and stop shortcut. Open the default URL and use the printed token
to log in to a new instance; the same value is saved as `auth.token` in settings.
The summary confirms settings creation, not that the HTTP listener is ready.
If an existing database contains legacy panel preferences or credentials, startup
imports them into the file once, retaining the previously effective values. In
that migration case, the imported management token replaces the generated token.

This runs in the foreground. Stop it with `Ctrl+C`, or run
`./bin/sing-box-panel server stop --config ./setting.json` in another terminal.
Use `server status` with the same settings to inspect the process. For
background operation, install and start the systemd service instead.

The server exposes the embedded UI and management API and is the only durable
task executor. Keep it active while commands install cores, refresh the
catalog or a subscription source, check or apply configuration, enable a
core, or control the child process.

Core, catalog, and runtime commands normally wait for their
task. Add `--detach` where supported to return immediately. Subscription source
refresh always returns its queued task immediately. Inspect either kind of
task separately with:

```sh
./bin/sing-box-panel task show TASK_ID --config ./setting.json
./bin/sing-box-panel task wait TASK_ID --config ./setting.json
```

## Save the first sing-box configuration

Log in to the Web UI and open Configuration. The panel keeps one sing-box
document, logically named `config.json`, as text in the `configuration_file`
table of `panel.db`. The Web editor manages it; there is no separately editable
file on disk or CLI for its content. A minimal document is:

```json
{}
```

Save in the Web editor, install and select a core, then use Check and Apply.
Concurrent saves use the current file revision and reject stale edits instead
of merging implicitly. Invalid JSON remains a draft and blocks Check, Apply,
Start, and Restart until corrected. Core lifecycle and artifact commands remain
available through the CLI. The separate `config show/set/check` commands manage
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
`--scope=auto` selects `system` for root and `user` otherwise. The default unit
grants no Linux capabilities; TUN, transparent proxying, raw sockets, and
privileged ports require a reviewed local override. See the authoritative
[systemd packaging guide](../systemd/README.md) before deploying a
system service.
