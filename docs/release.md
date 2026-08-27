# Release process

The repository builds static Linux artifacts but does not publish, upload,
sign, install, or retain them. Local builds, reproducible snapshots, and formal
releases are intentionally separate workflows.

## Local development build

Build the current working tree, including uncommitted source changes:

```sh
make bootstrap
make build
```

This writes `bin/sing-box-panel`. It is the appropriate command while
developing, but it is not the release packaging path.

## HEAD snapshot

Build both supported Linux architectures from committed `HEAD` without
requiring GA authorization:

```sh
make snapshot OUT=/absolute/path/to/new-output-directory
```

The output directory must not already exist and its parent must already be a
directory. The snapshot version is `dev`; its commit and date come from
`HEAD`. Uncommitted application changes, untracked files, ignored files, and
existing `web/node_modules` or `web/dist` do not enter or block the build.

## Formal release

A formal version uses strict v-prefixed SemVer and must pass the executable GA
readiness gate:

```sh
make release VERSION=v0.1.0 OUT=/absolute/path/to/new-output-directory
```

The recorded date defaults to the `HEAD` commit timestamp. Supply an explicit
RFC 3339 value only when the release process requires one:

```sh
make release VERSION=v0.1.0 DATE=2026-08-27T00:00:00Z OUT=/absolute/path/to/new-output-directory
```

The commit is always derived from `HEAD`; the release interface does not
accept caller-provided commit metadata or the former `RELEASE_*` environment
variables.

## Source isolation and evidence overlay

Snapshot and release builds share one implementation in
`packaging/release/build-release.sh`. It exports committed `HEAD` into a
temporary source tree, builds the Web application from a separate temporary
copy with locked dependencies, and copies only the verified `web/dist` into
the source snapshot. Go module, build, and package-manager state is isolated
from the caller's normal caches.

Formal builds may overlay only `release/evidence.json` and the five evidence
records listed in [Release evidence records](../release/evidence/README.md).
This narrow exception solves the ledger's commit self-reference; no
application source or additional evidence filename can enter through it.

Evidence authorizes a formal build. It is embedded in the private
`release-readiness` repository tool, not in the shipped `sing-box-panel`
binary.

## GA readiness

Inspect the authoritative gate directly:

```sh
go tool release-readiness
```

The command always emits its JSON status to stdout. It exits `0` when ready,
`3` when valid evidence reports that release requirements are not yet met,
`2` for invalid arguments, and `1` for an internal failure. Formal checks pass
`--release-version` and `--source-commit` together.

Readiness requires both the minimum embedded SQLite version and a complete,
digest-pinned, reviewed evidence ledger for the same source commit. The
checked-in gate is intentionally closed until all requirements are satisfied.

## Outputs and verification

Both snapshot and formal release create exactly:

- `sing-box-panel-linux-amd64`
- `sing-box-panel-linux-arm64`
- `SHA256SUMS`

The binaries use `CGO_ENABLED=0`, fixed CPU baselines, `-trimpath`,
`-buildvcs=false`, and the `webdist` tag. Files are assembled and verified in
a staging directory, then the whole directory is renamed into place so a
failed build cannot leave partial output.

Exercise the complete snapshot path and the current formal-gate outcome with:

```sh
make release-verify
```

For the exact script interface and environment isolation rules, see
[Release build materials](../packaging/release/README.md). No workflow
publishes these files; adding uploads, release creation, signing, or a
distribution channel remains a separate privileged change.
