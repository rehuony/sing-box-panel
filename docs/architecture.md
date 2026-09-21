# Repository architecture

This document records code ownership and dependency direction. It complements
the behavior contracts linked from [the documentation index](README.md); it
does not redefine the HTTP API, CLI, database, or subscription semantics.

## Dependency direction

The backend keeps one main package per domain and introduces another package
only for a concrete dependency or side-effect boundary:

```text
configuration --\
subscription ---+--> singbox --> application --> cli / httpapi / server
coreartifact ---/

catalog / artifactstore / runtime / store --> application / server
release --> selfupdate / release commands
```

`configuration` and `subscription` never import `singbox`. Transport packages
translate input and output but do not own use-case rules. `application`
composes use cases and stable package contracts. Infrastructure packages own
process, network, filesystem, and database effects. The web client depends on
the documented HTTP contract rather than Go implementation details.

## Domain package layout

A cohesive domain stays in one package and uses file prefixes to make ownership
visible. File length alone is not a reason to create another package.

- `internal/settings` owns the shared panel settings file, validation, defaults,
  atomic replacement and writer locking. `application` owns recovery when a
  Web save also updates sing-box protocol identity.
- `internal/configuration` owns strict, lossless sing-box JSON parsing and
  canonical serialization. `store` retains immutable
  snapshots as runtime evidence and initializes the current `schema.sql` directly.
- `internal/subscription` owns documents, normalized nodes, source parsing and
  fetching, rendering, and inbound conversion contracts. Files use
  `document_*`, `node_*`, `source_*`, `render_*`, and `inbound_*` prefixes.
- `internal/singbox` owns the reviewed support catalog, version-scoped native
  Schema assets, inbound conversion, and behavior-family dispatch. Exact
  versions exist as catalog data rather than forwarding packages.
- `internal/runtime` owns managed processes and its restricted Clash API
  monitoring client.
- `internal/application` owns use cases and runtime identity resolution backed
  by persistent state.
- `internal/server` owns server composition, serialized runtime controls, bounded recovery,
  and periodic subscription refresh.
- `internal/panelprocess` owns private local process control; it reuses the
  server's lifetime and lease and does not launch background processes.
- `internal/installation` inventories and cleans one selected instance's
  persistent paths. Database-directory locks in `internal/store` exclude
  cleanup while the panel or another CLI command owns a database connection.
- `internal/release` owns release-version validation and signatures;
  `internal/selfupdate` remains the download and atomic-replacement boundary.

Packages such as `store`, `catalog`, `artifactstore`, `coreartifact`, and
`runtime` remain separate because they represent durable dependency or
side-effect boundaries. The top-level `systemd` resource package remains
separate because Go embedding cannot read files from a parent directory.

## Adding a sing-box version

Support for a stable release is explicit and fails closed:

1. Review the upstream release and both official Linux artifacts, then update
   `internal/singbox/catalog.json`.
2. Decide whether the release can reuse a reviewed inbound-conversion behavior
   family. Runtime eligibility does not depend on that optional capability.
3. When the release provides the native `sing-box schema` command, generate
   and commit that exact version's canonical Schema. Older releases remain
   JSON-only; do not synthesize a replacement Schema.
4. Run the offline `make support-check`; it verifies the catalog and every
   committed native Schema asset without downloading upstream source.
5. Add or update catalog- and family-driven tests. Unknown, malformed, empty,
   or approximate versions must remain unsupported and must never select a
   nearby release.
6. Run the native amd64 and arm64 core contracts before merging.

Do not add exact-version forwarding directories. Version identity belongs in
the catalog; reusable behavior belongs in private `singbox` family functions.

## Test ownership

- Go unit, integration, fuzz, and package contract tests live beside their
  production package. Shared test helpers remain in that package's
  `test_helpers_test.go`.
- sing-box behavior differences use catalog- or family-driven table tests in
  `internal/singbox`. The real-binary contract is
  `internal/singbox/core_contract_test.go` and runs in dedicated native Linux
  CI jobs.
- React tests mirror the `web/src` ownership structure. API client tests are
  split by the same contract domains as the implementation.
- External inputs must skip in ordinary local tests and fail when their
  dedicated job requires them; do not recreate a centralized Docker E2E suite.

When moving a responsibility, move its focused tests and update callers in the
same change. Generated contracts and public compatibility boundaries must be
updated only through their designated workflow.
