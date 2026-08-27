# Documentation

These guides describe the supported sing-box-panel workflows. Start with the
task you need to complete and follow links to component-level specifications
when exact schemas or packaging details matter.

## User and operator guides

| Task | Guide |
| --- | --- |
| Build, initialize, and run a local instance | [Getting started](getting-started.md) |
| Use the command hierarchy and shell integrations | [CLI reference](cli.md) |
| Install sing-box versions and manage capability evidence | [Core versions and capabilities](core-versions-and-capabilities.md) |
| Create, validate, activate, and roll back configuration | [Configuration and runtime](configuration-and-runtime.md) |
| Publish subscriptions and inspect operational data | [Subscriptions and observability](subscriptions-and-observability.md) |
| Integrate with the management API securely | [HTTP API and security](http-api-and-security.md) |
| Build, sign, test, and publish Linux artifacts | [Release process](release.md) |

## Authoritative component references

- [OpenAPI contract](../api/openapi.yaml) defines management HTTP operations,
  request and response schemas, and problem details.
- [Capability manifest format](../capabilities/README.md) defines the reviewed,
  exact-version capability package and quarantine rules.
- [systemd packaging](../packaging/systemd/README.md) defines supported service
  layouts, ownership, installation, and hardening.
- [Release build materials](../packaging/release/README.md) define the release
  script inputs and generated files.
- [Web application](../web/README.md) defines frontend ownership, pnpm commands,
  embedding, and API-client boundaries.

The guides summarize these contracts without replacing them. When a guide and
an authoritative component reference disagree, treat the component reference
and executable validation as the source of truth and update the guide.
