# Documentation

These guides describe the supported sing-box-panel workflows. Start with the
task you need to complete and follow links to component-level specifications
when exact schemas or packaging details matter.

## Guide responsibilities

Each guide has one primary responsibility. Keeping behavior in the owning
guide avoids repeating details that can drift independently.

| Guide | Primary reader | Describes | Primary source of truth |
| --- | --- | --- | --- |
| This index | All readers | Documentation ownership, navigation, and conflict resolution | Current `docs/` tree |
| [Getting started](getting-started.md) | Contributor or local operator | Toolchain, initialization, settings paths, first revision, and systemd entry point | `go.mod`, `web/package.json`, `Makefile`, `internal/settings`, and CLI initialization |
| [CLI reference](cli.md) | CLI user or automation author | Command hierarchy, I/O, task waiting, exit codes, completion, and self-update | Cobra command tree under `internal/cli` and live `--help` output |
| [Core versions and adapters](core-versions-and-adapters.md) | Core operator or adapter maintainer | Catalog caching, artifact identity and trust, exact compiled profiles, and version onboarding | `internal/catalog`, `internal/coreartifact`, `internal/artifactstore`, and `internal/configuration/adapter` |
| [Configuration and runtime](configuration-and-runtime.md) | Configuration or runtime operator | Global schema-v2 history, projection, check/apply, lifecycle, and rollback | `internal/canonical`, configuration application services, `internal/runtime`, and activation storage |
| [Subscriptions and observability](subscriptions-and-observability.md) | Subscription or operations administrator | Users, grants, source versions, renderers, public delivery, logs, metrics, and traffic | Subscription application/store packages, `internal/subscription`, `internal/subscriptionfetch`, and `internal/clashapi` |
| [HTTP API and security](http-api-and-security.md) | API integrator or security reviewer | Routing, authentication, request boundaries, concurrency, and the Web trust boundary | `api/openapi.yaml`, `internal/httpapi`, and the Web HTTP client |
| [Release process](release.md) | Release maintainer | Isolated packaging, signing, native smoke tests, Draft Release verification, and publication | `Makefile`, `scripts`, and GitHub Actions workflows |
| [Repository architecture](architecture.md) | Contributor or maintainer | Dependency direction, package ownership, version adapters, and test placement | Current imports, composition roots, directory layout, and colocated tests |

## Authoritative component references

- [OpenAPI contract](../api/openapi.yaml) defines management HTTP operations,
  request and response schemas, and problem details.
- [Configuration adapter contract](../internal/configuration/adapter/contract.go)
  defines exact binary-profile matching, projection diagnostics, and
  fail-closed behavior.
- [systemd packaging](../systemd/README.md) defines supported service
  layouts, ownership, installation, and hardening.
- [Project scripts](../scripts/README.md) defines the installer, release
  script inputs, local script checks, and generated release files.
- [Web application](../web/README.md) defines frontend ownership, pnpm commands,
  embedding, and API-client boundaries.

The guides summarize these contracts without replacing them. When a guide and
an authoritative component reference disagree, treat the component reference
and executable validation as the source of truth and update the guide.
