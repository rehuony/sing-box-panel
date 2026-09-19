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

`make check` validates the offline configuration-schema artifacts,
third-party notices, and OpenAPI; runs Web linting, type-checking, tests, and a
production Web build; then verifies Go sources and modules and runs `go vet`,
Go tests, Shell syntax checks, and the network-independent installer contract
tests. `make build` uses the same Web-first boundary and writes
`bin/sing-box-panel`. Go compilation requires the generated `web/dist` tree;
that generated tree is the only UI source accepted by the Go build.

## Initialize settings and storage

Use an explicit settings path for an isolated repository-local instance:

```sh
./bin/sing-box-panel init --config ./setting.json
./bin/sing-box-panel verify --config ./setting.json
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
schema version 1, represented by the single consolidated `0001_initial.sql`.
Opening a new empty database applies that schema; opening an unidentified
non-empty database, a database with a previous application ID, or a database
newer than this binary fails closed. There is no in-place migration or
backfill from previous database identities in the current contract. Use a
fresh `data_dir` and retain any earlier database separately when testing this
architecture.

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

The settings file contains process-bootstrap values only: listener, base path,
external browser origin, authentication, data directory, GitHub catalog access,
traffic-period and raw-sample retention policy, subscription publication
metadata, and log retention. Mutable product state belongs in SQLite. New
settings initialize `traffic.sample_retention_days` to 90. The field is
required in every settings file and must be between 1 and 366; older settings
without it are rejected instead of receiving a compatibility default.

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

This runs in the foreground. Stop it with `Ctrl+C`, or run
`./bin/sing-box-panel server stop --config ./setting.json` in another terminal.
Use `server status` with the same settings to inspect the process. For
background operation, install and start the systemd service instead.

The server exposes the embedded UI and management API and is the only durable
task executor. Keep it active while commands install cores, refresh the
catalog or a subscription source, check or apply configuration, enable a
core, or control the child process.

Core, catalog, configuration, and runtime commands normally wait for their
task. Add `--detach` where supported to return immediately. Subscription source
refresh always returns its queued task immediately. Inspect either kind of
task separately with:

```sh
./bin/sing-box-panel task show TASK_ID --config ./setting.json
./bin/sing-box-panel task wait TASK_ID --config ./setting.json
```

## Save the first configuration

The panel keeps one saved sing-box configuration, logically named
`config.json`, that the Web editor and the CLI share. It is stored as text in
the `configuration_file` table of `panel.db`, not as a separate file on disk.
Save this minimal document as `config.json`:

```json
{}
```

Import it with the numeric file revision `0`, which means "no save yet":

```sh
./bin/sing-box-panel config import \
  --config ./setting.json \
  --file ./config.json \
  --revision 0
```

Later imports must pass the current file revision shown by
`config show --output json`; a stale revision is rejected instead of being
merged implicitly. Text that is not valid JSON is stored as a draft and blocks
check, apply, start, and restart until it is corrected. After installing a
core, validate and start with it:

```sh
./bin/sing-box-panel config check --config ./setting.json --core CORE_ARTIFACT_ID
./bin/sing-box-panel config apply --config ./setting.json --core CORE_ARTIFACT_ID
```

Continue with [Core versions](core-versions.md), then
[Configuration and runtime](configuration-and-runtime.md).

## Install a systemd service

The CLI can install an audited per-user or system service on Linux. User scope
uses the current executable and the current user's XDG paths. System scope
requires root and the fixed release layout under `/usr/local`, `/etc`, and
`/var/lib`.

```sh
sing-box-panel system install --scope=user --now
sing-box-panel system status --scope=user
```

`system status` reports systemd's unit state together with the settings path
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
