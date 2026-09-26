# Documentation

Start with [Getting started](getting-started.md) to build, initialize and run the
panel. The guides below describe the current supported behavior; exact API,
Schema and packaging contracts remain beside their owning components.

## Using the panel

| Task | Guide |
| --- | --- |
| Commands, settings, output formats and shell completion | [CLI reference](guides/cli.md) |
| Install, import and select an exact version; inspect support and migration rules | [Core versions](guides/core-versions.md) |
| Edit and validate configuration; operate, recover and observe the runtime | [Configuration and runtime](guides/configuration-and-runtime.md) |
| Publish subscriptions, manage access, and inspect logs, metrics and traffic | [Subscriptions and observability](guides/subscriptions-and-observability.md) |

## Developing and releasing

| Task | Reference |
| --- | --- |
| Understand package ownership and dependency direction | [Architecture](development/architecture.md) |
| Review product behavior and its verification coverage | [Product contract](development/architecture.md#product-contract) |
| Run test layers and maintain deterministic coverage | [Testing](development/testing.md) |
| Integrate HTTP clients and understand authentication and trust boundaries | [HTTP API and security](development/architecture.md#http-api-and-security) |
| Review a new sing-box release and regenerate support assets | [Maintaining support](guides/core-versions.md#maintaining-support) |
| Build, sign, verify and publish a panel release | [Release process](development/release.md) |

Version-specific review evidence belongs in the core guide, including the
[1.13 review](guides/core-versions.md#113-configuration-review). Generated field
inventories live beside their Schema source rather than as separate guides.

## Sources of truth

| Component | Authoritative source |
| --- | --- |
| Toolchain and verification commands | [`go.mod`](../go.mod), [`web/package.json`](../web/package.json), [`Makefile`](../Makefile) |
| CLI behavior | Cobra commands under [`internal/cli`](../internal/cli) and live `--help` |
| HTTP operations, payloads and problem details | [`api/openapi.yaml`](../api/openapi.yaml) |
| Exact core releases and official artifact pins | [`internal/singbox/catalog.json`](../internal/singbox/catalog.json) |
| Exact-version Schema sources and digests | [Schema manifest](../internal/singbox/schemas/manifest.json) |
| Reviewed 1.13 fields and upstream provenance | [Source definition](../cmd/singbox-support/schema-sources/1.13.json), [generated field table](../cmd/singbox-support/schema-sources/1.13-fields.csv) |
| Service layouts, ownership and hardening | [systemd packaging](../systemd/README.md) |
| Installer, packaging scripts and generated release files | [Project scripts](../scripts/README.md) |
| Frontend ownership, embedding and HTTP client boundaries | [Web application](../web/README.md) |

Keep each behavior in its owning guide and link to it from other guides.
When a summary conflicts with an authoritative contract or executable validation,
resolve the discrepancy there and update the summary. Keep generated outputs in
their component directories and regenerate them through the documented workflow.
