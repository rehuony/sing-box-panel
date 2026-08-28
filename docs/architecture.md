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

- `internal/configuration` owns canonical documents and the core projection
  contract. `document_*` and `adapter_*` files distinguish those concerns.
- `internal/subscription` owns documents, normalized nodes, source parsing and
  fetching, rendering, and inbound conversion contracts. Files use
  `document_*`, `node_*`, `source_*`, `render_*`, and `inbound_*` prefixes.
- `internal/singbox` owns the reviewed support catalog, generated profiles,
  configuration projection, inbound conversion, and behavior-family dispatch.
  Exact versions exist as catalog data rather than forwarding packages.
- `internal/runtime` owns managed processes and its restricted Clash API
  monitoring client.
- `internal/application` owns use cases and runtime identity resolution backed
  by persistent state.
- `internal/server` owns server composition and its private task runner.
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
2. Decide whether the release can reuse a reviewed behavior family. Add private
   same-package projection or inbound functions when behavior has changed.
3. Run `make support-generate` and `make support-check`.
4. Add or update catalog- and family-driven tests. Unknown, malformed, empty,
   or approximate versions must remain unsupported; there is no nearest-version
   fallback.
5. Run the native amd64 and arm64 core contracts before merging.

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
