# Getting started

This guide is for contributors building sing-box-panel and operators starting
one local development instance. The supported production targets are
`linux/amd64` and `linux/arm64`.

## Build the application

Source builds require Go 1.25, Node.js 22.12 or newer, Corepack, and the
package-pinned pnpm 11.21.0. Go and Web dependencies are locked by `go.mod`,
`go.sum`, and `web/pnpm-lock.yaml`.

From the repository root:

```sh
corepack enable pnpm
(cd web && pnpm install --frozen-lockfile --ignore-scripts --verify-store-integrity)
make check
make build
```

`make check` validates third-party notices and OpenAPI, runs Web tests and a
production Web build, verifies that Go sources are already formatted, and runs
`go vet` and Go tests. It is read-only and does not rewrite the working tree.
`make build` writes `bin/sing-box-panel` with the `webdist` build tag.

A plain `go build` embeds only a small development fallback page. It is not the
production Web build.

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
traffic-period policy, subscription publication metadata, and log retention.
Mutable product state belongs in SQLite.

When the panel is served through a reverse proxy, set `server.external_origin`
to the single public HTTP origin, for example `https://panel.example.com`.
HTTPS origins require `auth.secure_cookie: true`; a secure cookie in turn
requires an HTTPS external origin. The origin contains no path—continue to use
`server.base_path` for a public path prefix.

The generated listener is `127.0.0.1:3000`. Change `data_dir` to an absolute,
empty directory when a test must also isolate the database.

## Start the server

```sh
./bin/sing-box-panel server run --config ./setting.json
```

The server exposes the embedded UI and management API and is the only durable
task executor. Keep it active while commands install or check cores, refresh
the catalog, prepare or apply configuration, or control the child process.

Long-running commands wait for their task by default. Add `--detach` where the
command supports it to return immediately, then inspect the task separately:

```sh
./bin/sing-box-panel task show TASK_ID --config ./setting.json
./bin/sing-box-panel task wait TASK_ID --config ./setting.json
```

## Create the first canonical revision

Save this minimal document as `canonical.json`:

```json
{"schema_version":1,"global":{},"nodes":[],"rules":[],"subscription":{}}
```

Import it with an explicit empty compare-and-swap base:

```sh
./bin/sing-box-panel config import \
  --config ./setting.json \
  --file ./canonical.json \
  --base-revision none
```

Later writes must provide the current revision ID as `--base-revision`. A
stale base is rejected instead of being merged implicitly. Continue with
[Core versions and capabilities](core-versions-and-capabilities.md), then
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

`--scope=auto` selects `system` for root and `user` otherwise. The default unit
grants no Linux capabilities; TUN, transparent proxying, raw sockets, and
privileged ports require a reviewed local override. See the authoritative
[systemd packaging guide](../packaging/systemd/README.md) before deploying a
system service.
