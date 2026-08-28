# Repository architecture

This document records code ownership and dependency direction. It complements
the behavior contracts linked from [the documentation index](README.md); it
does not redefine the HTTP API, CLI, database, or subscription semantics.

## Dependency direction

The repository follows this general dependency direction:

```text
cmd / web
    -> cli / httpapi
        -> application
            -> domain packages and store contracts
                -> runtime, catalog, artifact, subscription, and persistence
```

Transport packages translate input and output. They do not own use-case rules.
`application` composes use cases and stable package contracts. Infrastructure
packages own process, network, filesystem, and database effects. The web client
depends on the documented HTTP contract rather than Go implementation details.

Large packages are split by responsibility within the package when doing so
makes ownership clearer. A cohesive algorithm or transaction remains together;
file length alone is not a reason to introduce another abstraction.

## Subscription packages

Subscription code has explicit boundaries under `internal/subscription`:

- `document` performs bounded, strict document parsing.
- `node` owns normalized nodes, stable identity, diagnostics, and persistence
  encoding.
- `source` dispatches and parses third-party sing-box, Mihomo, and URI sources.
- `render` emits sing-box, Mihomo, and Loon subscription documents.
- `inbound` defines the converter contract and exact-version registry.
- `inbound/singbox/v1_11_15`, `v1_12_25`, and `v1_13_19` contain the initial
  verified exact-version adapters.

The dependency direction inside this area is:

```text
application -> inbound registry -> version adapter
source/render/version adapter -> document, node
version adapter -> inbound contract
```

Version adapter packages must not import one another. They may share only the
stable `document`, `node`, and `inbound` contracts. Source parsing and output
rendering are independent of local inbound conversion.

Configuration projection has a parallel boundary under
`internal/configuration/adapter`. It owns the stable profile, diagnostic,
projection, provenance, and exact registry contracts. Version packages live at
`internal/configuration/adapter/singbox/vX_Y_Z`; they depend on canonical schema
v2 and the adapter contract, never on another version package.

## Adding a sing-box version

Support for a stable release is explicit and fails closed:

1. Add `internal/configuration/adapter/singbox/vX_Y_Z`, implement exact profile
   matching and projection, and register it in the application composition.
2. Add `internal/subscription/inbound/singbox/vX_Y_Z` with package name
   `singboxXYZ` and an implementation of `inbound.Converter`.
3. Keep version-specific field extraction, credential expansion, and
   sanitization inside that version directory. Do not reuse another version's
   implementation by importing its package.
4. Add contract tests covering projection, ignored fields, every supported and
   explicitly unsupported inbound, multi-user credential selection, required
   fields, and removal of server-only secrets.
5. Register the converter in the read-only default registry assembled by
   `internal/application/subscription_inbound.go`.
6. Add exact-dispatch coverage. Unknown, malformed, empty, or approximate
   versions must continue to fail with `inbound.ErrUnsupportedCoreVersion` or
   registry construction errors; no nearest-version fallback is permitted.
7. Update user-facing adapter documentation only after both adapters are
   verified against the exact stable core release and official binary profile.

## Test ownership

- Go unit, integration, fuzz, and package contract tests live beside their
  production package. Shared test helpers remain in that package's
  `test_helpers_test.go`.
- Version-specific inbound fixtures and contracts live in the corresponding
  version directory.
- React tests mirror the `web/src` ownership structure. API client tests are
  split by the same contract domains as the implementation.
- Any future test that needs an exact sing-box binary must remain beside the
  owning package, use an explicit environment gate, and run in a dedicated CI
  job. Missing external inputs must skip locally and fail when that job requires
  them; do not recreate a centralized Docker E2E suite.

When moving a responsibility, move its focused tests and update callers in the
same change. Generated contracts and public compatibility boundaries must be
updated only through their designated workflow.
