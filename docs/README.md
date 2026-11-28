# Documentation

Start with [Getting started](getting-started.md) to build, initialize and run the
panel. The guides below describe the current supported behavior; exact API,
Schema and packaging contracts remain beside their owning components.

## Using the panel

| Task | Guide |
| --- | --- |
| Panel settings, services, installation maintenance and shell completion | [CLI reference](cli.md) |
| Install, import and select a version; inspect release-line support and migration rules | [Core versions](core-versions.md) |
| Edit and validate configuration; operate, recover and observe the runtime | [Configuration and runtime](configuration-and-runtime.md) |
| Publish subscriptions, manage access, and inspect logs, metrics and traffic | [Subscriptions and observability](subscriptions-and-observability.md) |

## Developing and releasing

| Task | Reference |
| --- | --- |
| Understand package ownership and dependency direction | [Architecture](architecture.md) |
| Design the panel and node services, their protocol and state ownership | [Panel and node architecture](panel-node-architecture.md) |
| Review product behavior and its verification coverage | [Product contract](architecture.md#product-contract) |
| Run test layers and maintain deterministic coverage | [Testing](testing.md) |
| Review data refresh, streaming and interaction performance evidence | [Telemetry performance](telemetry-performance.md) |
| Integrate HTTP clients and understand authentication and trust boundaries | [HTTP API and security](architecture.md#http-api-and-security) |
| Build, sign, verify and publish a panel release | [Release process](release.md) |

Release-line support and the [reviewed 1.13 schema](core-versions.md#reviewed-113-schema)
are described in the core guide.

## Sources of truth

| Component | Authoritative source |
| --- | --- |
| Toolchain and verification commands | [`go.mod`](../go.mod), [`web/package.json`](../web/package.json), [`Makefile`](../Makefile) |
| CLI behavior | Cobra commands under [`internal/sing-box-panel/cli`](../internal/sing-box-panel/cli) and [`internal/sing-box-node/cli`](../internal/sing-box-node/cli) and live `--help` |
| HTTP operations, payloads and problem details | Panel: [`api/panel-openapi.yaml`](../api/panel-openapi.yaml); node console: [`api/node-openapi.yaml`](../api/node-openapi.yaml) |
| Panel–node protocol messages | [`api/proto/sbp/node/v1/node.proto`](../api/proto/sbp/node/v1/node.proto) |
| Supported sing-box release lines | [`internal/singbox/line.go`](../internal/singbox/line.go) |
| Reviewed 1.13 configuration schema and its provenance | [Source definition](../internal/sing-box-panel/schema/1.13.json) |
| Service layouts, ownership and hardening | [systemd packaging](systemd.md) |
| Installer, cleanup, packaging scripts and generated release files | [Project scripts](../scripts/README.md) |
| Frontend ownership, embedding and HTTP client boundaries | [Web application](../web/README.md) |
| Visual identity, design tokens and interface rules | [`DESIGN.md`](../DESIGN.md) |

Keep each behavior in its owning guide and link to it from other guides.
When a summary conflicts with an authoritative contract or executable validation,
resolve the discrepancy there and update the summary. Regenerate outputs through
their documented workflow.
