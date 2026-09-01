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
| [Core versions](core-versions.md) | Core operator or capability maintainer | Catalog caching, artifact identity and trust, version-scoped native Schema support, behavior families, native core contracts, and manual version onboarding | `internal/catalog`, `internal/coreartifact`, `internal/artifactstore`, `internal/singbox`, and `scripts/test/core-contract.sh` |
| [Configuration and runtime](configuration-and-runtime.md) | Configuration or runtime operator | Global JSON history, optional structured editing, check/apply, lifecycle, and rollback | `internal/configuration`, configuration application services, `internal/runtime`, and activation storage |
| [Subscriptions and observability](subscriptions-and-observability.md) | Subscription or operations administrator | Users, grants, source versions, renderers, public delivery, logs, metrics, and traffic | Subscription application/store packages, `internal/subscription`, and `internal/runtime` |
| [HTTP API and security](http-api-and-security.md) | API integrator or security reviewer | Routing, authentication, request boundaries, concurrency, and the Web trust boundary | `api/openapi.yaml`, `internal/httpapi`, and the Web HTTP client |
| [Release process](release.md) | Release maintainer | Isolated packaging, signing, native smoke tests, Draft Release verification, and publication | `Makefile`, `scripts`, and GitHub Actions workflows |
| [Repository architecture](architecture.md) | Contributor or maintainer | Dependency direction, package ownership, version capabilities, and test placement | Current imports, composition roots, directory layout, and colocated tests |

## Authoritative component references

- [OpenAPI contract](../api/openapi.yaml) defines management HTTP operations,
  request and response schemas, and problem details.
- [Core support catalog](../internal/singbox/catalog.json) defines the exact
  upstream release and official artifact identities supported by this build.
- [Configuration Schema manifest](../internal/singbox/schemas/manifest.json)
  records each version-scoped native Schema file and digest. Versions without a
  manifest entry remain fully editable as JSON and do not gain a synthetic
  Schema.
- [systemd packaging](../systemd/README.md) defines supported service
  layouts, ownership, installation, and hardening.
- [Project scripts](../scripts/README.md) defines the installer, release
  script inputs, local script checks, and generated release files.
- [Web application](../web/README.md) defines frontend ownership, pnpm commands,
  embedding, and API-client boundaries.

The guides summarize these contracts without replacing them. When a guide and
an authoritative component reference disagree, treat the component reference
and executable validation as the source of truth and update the guide.
