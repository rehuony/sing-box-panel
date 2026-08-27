<div align="center">
  <img src=".github/assets/sing-box-panel-icon.svg" width="128" height="128" alt="sing-box-panel icon">
  <h1>sing-box-panel</h1>
  <p>A Linux control plane for one exact, verifiable sing-box runtime.</p>
  <div>
    <img src="https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-FCC624?logo=linux&logoColor=black" alt="Linux amd64 and arm64">
    <img src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white" alt="Go 1.25">
    <img src="https://img.shields.io/badge/pnpm-11.21-F69220?logo=pnpm&logoColor=white" alt="pnpm 11.21">
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-GPL--3.0--or--later-00A9BE" alt="GNU General Public License 3.0 or later"></a>
  </div>
  <div>
    <a href="#features">Features</a> ·
    <a href="#quick-start">Quick start</a> ·
    <a href="#documentation">Documentation</a> ·
    <a href="https://github.com/rehuony/sing-box-panel/issues">Issues</a>
  </div>
</div>

sing-box-panel manages exact-version sing-box binaries, immutable
configuration and activation artifacts, subscriptions, durable tasks, and
sanitized operational metadata. Release builds embed the React interface and
SQLite migrations in one Go executable, so a target host does not need Go,
Node.js, pnpm, or a separate SQLite CLI.

> [!WARNING]
> sing-box-panel is available for development and integration testing, but is
> not GA-ready. Formal releases remain blocked until the executable readiness
> gate confirms both the required SQLite version and all reviewed release
> evidence. The manual release workflow builds, signs, and retains artifacts,
> but no workflow publishes a GitHub Release.

## Features

- Manage official and administrator-verified sing-box artifacts by exact
  version, architecture, variant, and immutable digest.
- Use structured configuration only with a reviewed exact-version capability;
  otherwise retain a lossless manual JSON workflow for stable sing-box
  versions without a proven manifest.
- Keep canonical revisions, checked startup artifacts, applied bundles, and
  rollback bundles separate and immutable.
- Publish applied subscription snapshots in sing-box, Mihomo, and Loon formats.
- Operate through a Docker-style CLI, versioned HTTP API, embedded React UI, or
  audited systemd integration.
- Persist runtime and maintenance work as observable, cancelable tasks instead
  of hiding long operations inside request handlers.

The supported deployment targets are `linux/amd64` and `linux/arm64`. Windows,
macOS, BSD, multiple simultaneous sing-box runtimes, and external databases are
outside the current product contract.

## Requirements

Building from source requires:

- Go 1.25, as declared by `go.mod`.
- Node.js 22.12 or newer.
- Corepack with the package-pinned pnpm 11.21.0.

A production build targets Linux and embeds its web assets. The managed
sing-box binary is installed or imported separately through the panel.

## Quick start

Install the locked frontend dependencies, run the repository checks, and build
the web-enabled binary:

```sh
make bootstrap
make check build
```

Initialize an isolated development instance and start the server:

```sh
./bin/sing-box-panel init --config ./setting.json
./bin/sing-box-panel verify --config ./setting.json
./bin/sing-box-panel server run --config ./setting.json
```

The default listener is `127.0.0.1:3000`. The settings file contains a random
management token and must not be committed. Keep `server run` active while
using commands that queue core, configuration, or runtime tasks.

See [Getting started](docs/getting-started.md) for the first canonical
configuration, settings precedence, and systemd deployment paths.

## Documentation

| Task | Guide |
| --- | --- |
| Build, initialize, and run the panel | [Getting started](docs/getting-started.md) |
| Use commands, automation output, and shell completion | [CLI reference](docs/cli.md) |
| Install exact versions and manage capability evidence | [Core versions and capabilities](docs/core-versions-and-capabilities.md) |
| Edit, check, apply, restart, and roll back configuration | [Configuration and runtime](docs/configuration-and-runtime.md) |
| Publish subscriptions and inspect operational data | [Subscriptions and observability](docs/subscriptions-and-observability.md) |
| Integrate with the API and operate its security boundary | [HTTP API and security](docs/http-api-and-security.md) |
| Build artifacts and understand the GA gate | [Release process](docs/release.md) |

The [documentation index](docs/README.md) also links the component-level
sources of truth for OpenAPI, capability manifests, systemd packaging, release
evidence, and the web application.

## Project map

```text
api/                 OpenAPI source contract
capabilities/        Reviewed exact-version capability manifests
cmd/                 Published sing-box-panel entry point
internal/cmd/        Repository-only Go tools
internal/            Go implementation packages
packaging/           Release scripts and systemd packaging materials
release/             Release-authorization evidence
web/                 React/Vite application managed with pnpm
```

## Contributing

Use the existing architecture and keep public behavior synchronized with its
authoritative documentation. Before submitting a change, run:

```sh
make check
```

Open an [issue](https://github.com/rehuony/sing-box-panel/issues) before a large
behavioral or architectural change so its contract and evidence requirements
can be reviewed first.

## License

sing-box-panel is licensed under GPL-3.0-or-later. See the concise
[license summary](LICENSE), the complete [GPLv3 text](COPYING), and the
[third-party notices](THIRD_PARTY_NOTICES).
