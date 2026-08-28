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
nearby patch, an unsupported architecture, an unreported fingerprint, or a
runtime manifest.

The initial reviewed profiles are the official plain Linux amd64 and arm64
builds of:

- sing-box 1.11.15;
- sing-box 1.12.25; and
- sing-box 1.13.19.

The reviewed source catalog at `internal/singbox/catalog.json` records
each exact tag and commit plus the amd64 and arm64 asset name, URL, size,
SHA-256, feature fingerprint, and behavior family. `go tool singbox-support
generate` turns it into immutable Go data; production never loads or downloads
a support manifest. `go tool singbox-support check` requires the catalog,
generated output, configuration registry, inbound registry, and compiled
family map to agree exactly.

The `Core Compatibility` workflow runs every catalog entry on native Linux
amd64 and arm64 runners. It verifies the reviewed archive size and SHA-256,
executes the real binary to inspect its exact version and feature fingerprint,
resolves the compiled adapter, projects a non-empty canonical configuration,
and requires that binary to accept the result with `sing-box check`. Relevant
pull requests run this contract automatically, and every signed release must
pass both architectures before the signing environment is available. On a
native Linux development host the same evidence is available through `make
core-contract`.

The dual-architecture review changes these adapters from the former
`official-linux-arm64@1` identity to `official-linux-plain@2`. Existing checked
startup artifacts retain their original evidence and therefore fail closed
under the new panel build; recompile and check them from the unchanged
canonical revision before applying them again.

An unmatched artifact may still be installed, inspected, quarantined, revoked,
or removed. Preview, compilation, check, Apply, Start, Restart, and Rollback
fail closed when they would depend on an unavailable adapter.

Adapter provenance records the exact upstream tag and commit used during
review. There is no runtime capabilities manifest, mutable pin, remote adapter
download, compatibility level, or manual-JSON fallback.

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

A patch release in a known release line normally needs one catalog entry
followed by generation. It may reuse the latest reviewed behavior family while
the exact catalog profile still prevents fallback to another patch.
Behavior-family IDs are scoped to the release line: the initial family may be
`1.13`, while a schema, projection, subscription, or feature-fingerprint change
uses a new ID such as `1.13-r2`. Family differences are private functions in
the `singbox` package and are covered by family-driven tests rather than
version-specific forwarding packages.

Adding support is intentionally manual:

1. Review the upstream stable release and both official Linux artifacts.
2. Add the exact catalog entry and run `make support-generate`.
3. Reuse a family only when its schema, projection, subscription contract, and
   feature fingerprints remain identical; otherwise add a new private family
   implementation in `internal/singbox`.
4. Add focused adapter and inbound tests, run `make check`, and require the
   native amd64 and arm64 core contracts to pass before merging.

The daily `Core Version Monitor` compares only the highest supported catalog
entry with the latest upstream stable Release. When upstream is newer it creates
or updates one rolling issue for manual evaluation; it never edits code, opens
a pull request, chooses a family, executes upstream bytes, or attempts to
backfill older versions. The issue closes automatically after the support
catalog catches up.

Unknown, malformed, empty, approximate, or partially matching versions must
continue to fail rather than falling back.

## Artifact trust boundary

The official path relies on GitHub HTTPS, immutable repository identity,
release digest evidence, and local SHA-256 verification. Administrator import
relies on the operator to obtain the expected digest through a trusted channel.
The project does not claim TUF, project-owned core signatures, or an
independent transparency log.
