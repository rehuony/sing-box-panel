# Core versions and adapters

sing-box-panel keeps release discovery, installed binary trust, and executable
configuration support as separate decisions. Seeing a stable release in the
catalog does not mean that the current panel build can run it.

## Exact binary identity

An installed artifact is identified by its immutable artifact ID, exact
`MAJOR.MINOR.PATCH` version, operating system, architecture, variant, archive
digest, binary digest, and reported feature fingerprint. Multiple artifacts
for one version may coexist.

Official installation and administrator import both verify bounded archive
extraction, ELF identity, SHA-256, and the output of `sing-box version`. A
reported version must match the requested exact version. Missing build tags
are recorded as `not_reported`; they are never treated as an empty feature set.

```sh
sing-box-panel core catalog refresh
sing-box-panel core catalog list --installable
sing-box-panel core install ASSET_ID
sing-box-panel core list
sing-box-panel core show ARTIFACT_ID
```

An administrator-verified archive can be imported with independent digest
evidence:

```sh
sing-box-panel core import \
  --file ./sing-box.tar.gz \
  --sha256 ARCHIVE_SHA256 \
  --version 1.13.19 \
  --arch arm64 \
  --variant plain
```

## Catalog refresh and last-known-good state

The official catalog reads stable GitHub Releases in pages of 20, up to 100
pages. Each page is limited to 8 MiB, the complete refresh to 128 MiB, and the
operation to three minutes. Drafts, prereleases, malformed releases, and
irrelevant assets are filtered before a candidate can enter the stored
catalog.

An ordinary refresh honors `github.catalog_ttl_hours`; use `--force` for an
explicit upstream refresh. The Web refresh action also requests a forced
refresh. Per-page ETags allow the panel to prove an unchanged catalog without
replacing it. A timeout, rate limit, invalid response, or size failure returns
an error and leaves the last successful catalog and validator intact.

```sh
sing-box-panel core catalog refresh --force
```

`--installable` means that an asset has usable digest evidence. It does not
mean that this panel build has a compiled configuration adapter for that
binary profile.

## Compiled adapter boundary

Configuration support is compiled into the panel. A registry matches the
complete installed profile—exact version, Linux operating system,
architecture, variant, and the sorted feature fingerprint. It never selects a
nearby patch, another architecture, an unreported fingerprint, or a runtime
manifest.

The initial reviewed profiles are the official plain Linux arm64 builds of:

- sing-box 1.11.15;
- sing-box 1.12.25; and
- sing-box 1.13.19.

An unmatched artifact may still be installed, inspected, quarantined, revoked,
or removed. Preview, compilation, check, Apply, Start, Restart, and Rollback
fail closed when they would depend on an unavailable adapter.

Adapter provenance records the exact upstream tag and commit used during
review. There is no `capabilities/` manifest directory, mutable pin, remote
adapter download, compatibility level, or manual-JSON fallback.

## Trust reduction

These commands permanently reduce trust in immutable bytes:

```sh
sing-box-panel core quarantine ARTIFACT_ID
sing-box-panel core revoke ARTIFACT_ID
```

Revocation is terminal. Reinstalling identical bytes does not clear a prior
restriction. A restriction blocks new checks and runtime work, requests
cancellation where safe, and fences desired runtime state; it does not
silently kill an already-running child.

## Adding a stable version

A new release needs two independent reviewed packages:

1. Add `internal/configuration/adapter/singbox/vX_Y_Z`, record upstream
   provenance and the official build fingerprint, implement projection from
   canonical schema v2, and register it explicitly.
2. Add `internal/subscription/inbound/singbox/vX_Y_Z`, implement the exact
   inbound-to-node contract, and register it explicitly.
3. Add exact-dispatch, projection, ignored-field, credential expansion, and
   secret-sanitization tests. Version packages must not import one another.
4. Update the Web/API documentation only after the complete profile and
   behavior are verified.

Unknown, malformed, empty, approximate, or partially matching versions must
continue to fail rather than falling back.

## Artifact trust boundary

The official path relies on GitHub HTTPS, immutable repository identity,
release digest evidence, and local SHA-256 verification. Administrator import
relies on the operator to obtain the expected digest through a trusted channel.
The project does not claim TUF, project-owned core signatures, or an
independent transparency log.
