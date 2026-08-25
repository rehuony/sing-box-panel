# Release process

The repository can build static Linux artifacts but does not publish, upload,
sign, install, or retain them. Formal versions are fail-closed and require the
executable GA readiness gate to pass.

## Development builds

Build the Web distribution before invoking the release script:

```sh
corepack enable pnpm
(cd web && pnpm install --frozen-lockfile --ignore-scripts --verify-store-integrity)
(cd web && pnpm run build)
RELEASE_VERSION=dev ./scripts/build-release.sh ./out-dev
```

`RELEASE_VERSION` is mandatory. Only the exact values `dev` and `ci` may
continue while GA readiness is false. They emit the open-gate warning and may
use `unknown` when `RELEASE_COMMIT` is omitted. They resolve the active GOPATH,
module cache, and build cache through `go env`.

The destination must not already contain an expected output and must not be
inside a `go:embed` source tree.

## Formal builds

A formal version uses strict v-prefixed SemVer and requires a non-zero,
lowercase, full 40- or 64-character `RELEASE_COMMIT`:

```sh
RELEASE_VERSION=v0.1.0 \
RELEASE_COMMIT="$(git rev-parse HEAD)" \
./scripts/build-release.sh /absolute/path/to/output
```

`RELEASE_DATE` may supply the whitespace-free build timestamp recorded in the
binary; it defaults to `unknown` when omitted.

The commit must equal both the checked-out Git `HEAD` and the source commit in
the validated embedded evidence ledger. A formal build rejects staged,
modified, or untracked inputs outside the narrow evidence overlay documented
in [Release evidence records](../release/evidence/README.md). Ignore rules and
Git `skip-worktree` or `assume-unchanged` flags cannot hide another source
input.

The script also disables inherited Go workspaces, overlays, persistent Go
environment overrides, experiments, and automatic toolchain selection. It
uses isolated temporary GOPATH, module, and build caches, forces module mode,
downloads the complete module graph, and runs `go mod verify` at both readiness
boundaries. Advertised CPU baselines are fixed to `GOAMD64=v1` and
`GOARM64=v8.0`.

After the first readiness decision, the formal path moves existing
`web/node_modules` and `web/dist` aside. It recreates locked Web dependencies
with the package-pinned pnpm, isolated Corepack and store state, a frozen
lockfile, disabled lifecycle scripts, and store-integrity verification. It
builds a fresh distribution, verifies the index and assets, rechecks source
state, and reruns readiness against the exact evidence bytes that the Go
binaries will embed. The caller's original Web trees are restored on exit.

## GA readiness

Inspect the authoritative gate directly:

```sh
go run ./cmd/release-readiness
```

The argument-free form is diagnostic and emits a JSON status. Readiness is the
conjunction of:

- an embedded SQLite version at or above the required minimum; and
- a complete, digest-pinned, reviewed evidence ledger for the same source
  commit.

The required evidence categories are the core-version matrix, structured
capability matrix, Linux runtime resilience, browser contract and
accessibility, and subscription/observability end-to-end evidence. The exact
record schema and permitted overlay paths are defined by
[Release evidence records](../release/evidence/README.md).

Formal use supplies `--release-version` and `--source-commit` together. Either
one alone is invalid. Upgrading SQLite cannot open the gate while evidence is
missing or invalid, and complete evidence from another source commit cannot
authorize the current checkout.

## Outputs and verification

The build script creates:

- `sing-box-panel-linux-amd64`;
- `sing-box-panel-linux-arm64`; and
- `SHA256SUMS`.

Both binaries use `CGO_ENABLED=0`, `-trimpath`, and the `webdist` tag. The
verification wrapper evaluates readiness, confirms that a formal build is
blocked when the gate is open, then exercises the `ci` development path and
checks both Linux outputs:

```sh
./scripts/verify-release-build.sh /absolute/path/to/output
```

CI additionally validates notices, shell syntax, OpenAPI, Go formatting and
module tidiness, `go vet`, normal and race-enabled tests, deterministic fuzz
smoke tests, Web type checking/tests/build, the embedded host binary, and
checksums for both Linux architectures.

No workflow publishes these files. Adding uploads, release creation, signing,
or a distribution channel is a separate privileged change. See
[Release build materials](../packaging/release/README.md) for the authoritative
script interface.
