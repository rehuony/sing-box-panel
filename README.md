<div align="center">
  <img src="web/packages/shared/public/favicon.svg" width="128" height="128" alt="sing-box-panel icon">
  <h1>sing-box-panel</h1>
  <p>A Linux control plane for a fleet of exact, verifiable sing-box runtimes.</p>
  <div>
    <img src="https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-FCC624?logo=linux&logoColor=black" alt="Linux amd64 and arm64">
    <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" alt="Go 1.26">
    <img src="https://img.shields.io/badge/pnpm-11.21-F69220?logo=pnpm&logoColor=white" alt="pnpm 11.21">
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-GPL--3.0--or--later-00A9BE" alt="GNU General Public License 3.0 or later"></a>
  </div>
  <div>
    <a href="#features">Features</a> ·
    <a href="#installation">Installation</a> ·
    <a href="#quick-start">Quick start</a> ·
    <a href="#documentation">Documentation</a> ·
    <a href="https://github.com/rehuony/sing-box-panel/issues">Issues</a>
  </div>
</div>

sing-box-panel manages sing-box on any number of Linux hosts from one panel.
The panel, `sing-box-panel`, holds every node's configuration, the core version
catalog, subscriptions and aggregated telemetry, and runs no sing-box itself. A
node, `sing-box-node`, runs one exact-version sing-box on its host, reports its
state and carries out the panel's commands. Release builds embed each service's
React interface and SQLite schema in its Go executable, so a target host does
not need Go, Node.js, pnpm, or a separate SQLite CLI.

## Features

- Manage a fleet from one panel: nodes dial out to the panel over gRPC, enroll
  with a one-time code and a node-generated Ed25519 key, and keep running their
  applied configuration while the panel is unreachable.
- Manage official and administrator-imported sing-box artifacts by exact
  version and Linux architecture (amd64 or arm64), with recorded installation hashes.
  The panel caches and verifies each archive and nodes install from it. Each
  platform uses the static musl build.
- Edit one lossless sing-box JSON document per node, validate every candidate
  with the selected exact binary on that node, and add structured editing only
  for versions that have a committed native or project-reviewed JSON Schema.
- Keep canonical revisions, checked startup artifacts, applied bundles, and
  rollback bundles separate and immutable; roll back to the previous bundle
  through the authenticated API.
- Publish live, authorized subscriptions in sing-box, Mihomo, and Loon formats
  from each node's last applied inbounds and current third-party source versions.
- Manage nodes, core versions, configuration and subscriptions through the
  embedded React UI and versioned HTTP API; maintain panel settings, nodes and
  services through a focused CLI and audited systemd integration.
- Return completed operation results, serialize each node's process controls,
  and retain sanitized logs and immutable runtime history.

The supported deployment targets are `linux/amd64` and `linux/arm64`. Windows,
macOS, BSD, more than one sing-box runtime on a node, and external databases are
outside the current product contract.

## Installation

A release has two components: the panel, `sing-box-panel`, and the node agent
that runs sing-box on each proxy host, `sing-box-node`. The installer supports
Linux on amd64 and arm64 and installs one component per run. With no
arguments it selects the latest published stable GitHub Release. An explicit
`--version vX.Y.Z` may select an already published stable release or GitHub
pre-release. It first reports which components are installed for the current
user and the state of their systemd services, then installs `--component panel`
or `--component node`, or asks which one when neither is given; without a
terminal the component is required. The installer authenticates the exact
`SHA256SUMS` bytes and release version with the Ed25519 key retrieved from the
repository's canonical `.github/keypair/release-signing-public-key` file,
verifies the selected binary's SHA-256 digest, and installs only that binary.
It does not read, initialize, migrate, or rewrite settings or data.

Install the panel for the current user, or a node as root:

```sh
curl https://raw.githubusercontent.com/rehuony/sing-box-panel/main/scripts/installer.sh | bash -s -- --component panel
curl https://raw.githubusercontent.com/rehuony/sing-box-panel/main/scripts/installer.sh | sudo bash -s -- --component node
```

Pin an exact published release, including a pre-release, when reproducibility
matters:

```sh
curl https://raw.githubusercontent.com/rehuony/sing-box-panel/main/scripts/installer.sh | bash -s -- --component panel --version v0.1.0
```

The default layouts are:

| Component | Effective user | Binary | Settings | Data |
| --- | --- | --- | --- | --- |
| Panel | Regular user | `~/.local/bin/sing-box-panel` | `${XDG_CONFIG_HOME:-$HOME/.config}/sing-box-panel/setting.json` | `${XDG_DATA_HOME:-$HOME/.local/share}/sing-box-panel` |
| Panel | root | `/usr/local/bin/sing-box-panel` | `/etc/sing-box-panel/setting.json` | `/var/lib/sing-box-panel` |
| Node | Regular user | `~/.local/bin/sing-box-node` | — | `${XDG_DATA_HOME:-$HOME/.local/share}/sing-box-node` |
| Node | root | `/usr/local/bin/sing-box-node` | — | `/var/lib/sing-box-node` |

An existing binary requires overwrite confirmation; pass `--yes` for an unattended
replacement. The formatted completion summary lists the installed binary, default
settings and data paths, and shell-completion commands, for example:

```sh
eval "$(sing-box-panel completion zsh)"
```

The installer leaves shell startup files and systemd unchanged. Add `~/.local/bin`
to `PATH` when it is not already present. To install and start an audited service
explicitly:

```sh
# Regular user
~/.local/bin/sing-box-panel systemd install --scope=user --now

# Root/systemd installation
/usr/local/bin/sing-box-panel systemd install --scope=system --now
```

A node installs its service the same way. It starts at once, prints its
console password and waits to be enrolled: run the command the panel shows when
the node is added, or enroll it from its console at `http://127.0.0.1:8000/`:

```sh
sudo /usr/local/bin/sing-box-node systemd install --scope=system --now
sudo /usr/local/bin/sing-box-node enroll --code ...
```

A user service requires administrator-enabled lingering if it must continue
after logout. An already running service keeps its existing process after a
binary upgrade; restart it explicitly when ready. See the authoritative
[systemd packaging guide](docs/systemd.md) for ownership and hardening.

The no-argument latest-version lookup requires at least one published,
non-prerelease GitHub Release. Before the first stable release is published,
the installer exits without changing the host and reports that no latest
release can be resolved. Use `--version` to install an already published
pre-release such as `v0.1.0` explicitly.

## Build requirements

Building from source requires:

- Go 1.26, as declared by `go.mod`.
- Node.js 22.12 or newer.
- Corepack with the package-pinned pnpm 11.21.0.

A production build targets Linux and embeds its web assets. The managed
sing-box binary is installed or imported separately through the panel. The
supported build workflow generates `web/apps/panel/dist` and `web/apps/node/dist` before compiling the binaries; Go
has no alternate UI input.

## Quick start

Install the locked frontend dependencies, run the repository checks, and build
both binaries with their Web applications:

```sh
make deps
make check build
```

For daily work, `make build` builds `bin/sing-box-panel` and `bin/sing-box-node`,
`make check` runs the ordinary checks, and `make fmt` formats Go sources; plain
`make` lists every target. See [Contributing](.github/CONTRIBUTING.md#validation)
for focused checks and generated-file maintenance.

Initialize an isolated development instance and start the server:

```sh
./bin/sing-box-panel init --config ./setting.json
./bin/sing-box-panel config verify --config ./setting.json
./bin/sing-box-panel server start --config ./setting.json
```

`server start` also creates default settings automatically when the selected
file is missing, so `init` is optional. On creation, it prints a concise summary
of the paths, default URL, and generated administrator email and password. Every successful start also
prints the bound panel URL and streams formatted panel logs. Existing settings
are never overwritten by startup.

The default listener is `127.0.0.1:3000`. The settings file contains a random
password hash and must not be committed. The panel runs no sing-box: add a node
in the Web UI, then enroll and start `sing-box-node` on a Linux host with the
command the panel shows. Use the Web UI for nodes, catalog refresh, installation,
import, configuration and subscription management.

See [Getting started](docs/getting-started.md) for adding the first node, the
first configuration, settings precedence, and systemd deployment paths.

## Documentation

| Workflow | Guide |
| --- | --- |
| Build, initialize, and run the panel | [Getting started](docs/getting-started.md) |
| Use commands, automation output, and shell completion | [CLI reference](docs/cli.md) |
| Install exact versions and inspect configuration capabilities | [Core versions](docs/core-versions.md) |
| Edit, check, apply, and restart configuration; roll back through the API | [Configuration and runtime](docs/configuration-and-runtime.md) |
| Publish subscriptions and inspect operational data | [Subscriptions and observability](docs/subscriptions-and-observability.md) |
| Understand the panel and node services, their protocol and state ownership | [Panel and node architecture](docs/panel-node-architecture.md) |
| Integrate with the API and operate its security boundary | [HTTP API and security](docs/architecture.md#http-api-and-security) |
| Build, sign, test, and publish release artifacts | [Release process](docs/release.md) |

The [documentation index](docs/README.md) also links the component-level
sources of truth for OpenAPI, version capabilities, systemd packaging, release
builds, and the web application.

## Project map

```text
.github/             Workflows, community templates, code ownership, and signing keys
api/                 HTTP (OpenAPI) and panel–node (protobuf) contracts
cmd/                 The panel and node binaries' main.go files
docs/                Guides, architecture, testing, packaging, and release process
internal/            Go packages: sing-box-panel/ and sing-box-node/ per service, shared ones beside them
scripts/             End-user installation, release, and local test scripts
tools/               Maintainer commands run through go tool
web/                 pnpm workspace: the panel and node console React applications and their shared package
DESIGN.md            Visual identity, design tokens, and interface rules
```

## Contributing

Read the [contribution guide](.github/CONTRIBUTING.md) before opening an issue or pull
request. Participation is governed by the [Code of Conduct](.github/CODE_OF_CONDUCT.md),
and vulnerabilities must be reported privately according to the
[Security Policy](.github/SECURITY.md).

## License

sing-box-panel is licensed under GPL-3.0-or-later. See the concise
[license summary](LICENSE), the complete [GPLv3 text](COPYING), and the
[third-party notices](THIRD_PARTY_NOTICES).
