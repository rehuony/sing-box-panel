# Core versions

sing-box-panel keeps release discovery, installed versions, structured
editing, and inbound subscription conversion as separate decisions. An installed
artifact can run raw JSON even when this panel has no Schema or converter for
its exact version.

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
sing-box-panel core refresh
sing-box-panel core catalog --installable
sing-box-panel core install ASSET_ID
sing-box-panel core list
sing-box-panel core show ARTIFACT_ID
```

A local archive can be imported with its expected checksum:

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
sing-box-panel core refresh --force
```

`--installable` means that an asset has usable digest evidence. Structured
editing and inbound conversion are reported as separate optional capabilities.

## Browser version management

Installed and available lists share 5/10/50 pagination, independent scrolling
and inline enable/disable/remove/download buttons. Version and source cells
contain plain text; state reflects the running binary. The top status bar shows
the numeric core version as a badge. Available assets are filtered using the
deployed panel binary's GOOS/GOARCH, not browser/device detection. Enabling rejects incompatible artifacts
before queueing work, then validates the saved configuration before replacement.

The compact import dialog accepts a single `.tar.gz`/`.tgz` archive by drag-and-drop
or file selection, and asks for its exact version. Standard sing-box filenames
fill the version automatically; custom
filenames require manual entry, and the suggested version remains editable.
Source metadata records the filename and the variant defaults to `plain`.

An optional GitHub Token in panel service/security settings is used server-side
for version discovery. An omitted token retains the configured value; explicit
removal returns to anonymous requests. Authentication can raise GitHub's normal
API allowance, but a token does not bypass rate limits or replace cache/error
handling.

## Runtime and Schema boundaries

Runtime eligibility depends on platform compatibility and a successful
configuration check, independently of Schema or subscription-conversion availability. Compile snapshots the current
strict JSON object, and the selected artifact must accept those exact bytes
with `sing-box check` before it can be activated. Start and Restart repeat the
identity, digest, and binary check gates.

The catalog currently contains the exact releases:

- sing-box 1.11.15;
- sing-box 1.12.25;
- sing-box 1.13.19; and
- sing-box 1.14.0.

The reviewed source catalog at `internal/singbox/catalog.json` records each
exact tag and commit plus the module sums, amd64 and arm64 asset name, URL,
size, SHA-256, feature fingerprint, behavior family, and upstream Go identity.
It is the version and official-artifact lock.

Schema support is independently keyed by exact version. Starting with 1.14,
the networked `go tool singbox-support generate` command executes the locked
official Linux release binary's native `sing-box schema` command and
deterministically rebuilds one canonical Schema asset. It does not apply source
patches or maintain an upstream source copy. Versions before the native command
remain JSON-only. Production never downloads a Schema or upstream source;
offline `go tool singbox-support check` verifies every committed asset and
digest.

The `Core Compatibility` workflow runs every catalog entry on native Linux
amd64 and arm64 runners. It verifies the reviewed archive size and SHA-256,
executes the real binary to inspect its exact version and feature fingerprint,
and requires that binary to accept representative raw configuration with
`sing-box check`. Native Schema reproducibility is checked separately by the
networked regeneration job and offline asset validation. Relevant pull
requests run these contracts automatically, and every signed release must pass
both architectures before the signing environment is available. On a native
Linux development host the binary evidence is available through
`make core-contract`.

A version without a committed Schema remains fully available through the raw
JSON editor and runtime evidence chain. A missing inbound converter disables
only subscription extraction for that version. Neither capability is inferred
from a nearby patch version.

## Installed version lifecycle

Installed versions have no separate trust, quarantine, or revocation state.
Import adds a version to the installed list; enable and disable control runtime
use. Removal unregisters an unused version and keeps existing reference checks.
Archive format, size, platform, checksum and exact-version checks still detect
invalid files, incompatible binaries and changed bytes.

Migration 0008 removes the former verification state while preserving artifact
IDs, creation times, digests, and startup/runtime references. Previously restricted
artifacts are selectable after migration; migration does not start a process or
change desired runtime state. The quarantine/revoke HTTP and CLI operations and
the verification-state filter are removed. The persisted source code
`user_verified` continues to mean manual import, preserving artifact identities;
it is not an approval state.

## Adding a stable version

A patch release in a known release line normally needs one catalog entry.
Inbound-conversion behavior may reuse a reviewed family only when its supported
types remain identical. Schema identity is simpler: each exact version either
has its own native output or is JSON-only.

Adding support is intentionally manual:

1. Review the upstream stable release and both official Linux artifacts.
2. Add the exact catalog entry. If the release exposes `sing-box schema`, run
   `make support-generate` and require a clean offline `make support-check`; if
   it does not, leave the version JSON-only.
3. Add or reuse an inbound-conversion family only after its input types are
   reviewed. Its absence must not block runtime use.
4. Add focused catalog, optional Schema, and inbound tests, run the offline
   `make check`, and require the native amd64 and arm64 core contracts to pass
   before merging.

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
release digest evidence, and local SHA-256 verification. Manual import accepts
operator-supplied archives. Browser upload computes a transfer checksum locally;
this detects changed bytes, not publisher authenticity. CLI imports compare the
provided expected checksum.
The project does not claim TUF, project-owned core signatures, or an
independent transparency log.
