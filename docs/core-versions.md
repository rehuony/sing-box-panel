# Core versions

sing-box-panel keeps release discovery, installed versions, structured
editing, and inbound subscription conversion as separate decisions. An installed
artifact can run raw JSON even when this panel has no Schema or converter for
its exact version.

## Exact binary identity

An installed artifact is identified by its immutable artifact ID, exact
`MAJOR.MINOR.PATCH` version, operating system, architecture, variant, archive
digest, binary digest, and reported feature fingerprint. The supported build
variant is `musl`, with separate `amd64` and `arm64` CPU architectures. Artifacts
with different source or digest identities can still coexist.

Official installation and administrator import both verify bounded archive
extraction, ELF identity, SHA-256, and the output of `sing-box version`. A
reported version must match the requested exact version. Installation requires
the reported `with_musl` build tag and an ELF without a dynamic interpreter or
shared-library dependencies. Missing tags and non-musl builds are rejected,
including archives renamed to look like musl builds.

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
  --variant musl
```

## Catalog refresh and last-known-good state

The official catalog reads stable GitHub Releases in pages of 50, up to 100
pages. Each page is limited to 32 MiB, the complete refresh to 256 MiB, and the
operation to three minutes. Drafts, prereleases, malformed releases, and
irrelevant assets are filtered before a candidate can enter the stored
catalog. Only `sing-box-VERSION-linux-{amd64,arm64}-musl.tar.gz` archives are
accepted. Each version has at most one asset per architecture; releases without
a matching musl archive are omitted, with no plain/glibc fallback. Missing or
inconsistent digest evidence still blocks installation. Storage uses the current format described in
[Database compatibility](getting-started.md#database-compatibility).

The complete successful catalog and validator remain in SQLite until a newer
successful refresh replaces them. `github.catalog_refresh_interval_hours`
controls when the running server checks GitHub again; it is not a cache expiry.
The server checks the schedule in the background at startup and continues while
the panel runs, without delaying HTTP readiness. An ordinary refresh honors the
interval; use `--force` for an explicit upstream check. The Web “Check GitHub
for updates” action requests a forced refresh, while opening version management
reads the local catalog and only initializes it when missing. Cached rows and
installed versions remain visible while a refresh runs. The HTTP client
honors standard proxy environment variables. Large valid release pages are byte
bounded and checked for duplicate keys and excessive nesting without an asset-count
limit. Authentication and GitHub rate-limit errors have distinct diagnostics. Per-page ETags allow the panel to prove an unchanged catalog without
replacing it. A timeout, rate limit, invalid response, or size failure returns
an error, leaves the last successful catalog and validator intact, and is
retried by the running server after five minutes.

```sh
sing-box-panel core refresh --force
```

`--installable` means that an asset has usable digest evidence. Structured
editing and inbound conversion are reported as separate optional capabilities.

## Browser version management

Installed and available lists share 5/10/50 pagination, independent scrolling
and inline enable/disable/remove/download buttons. Available rows contain
version, source, official link, and actions. The official link displays the
archive name as a badge and opens the matching upstream GitHub Release in a new
tab. Installed assets show a disabled **Installed** button and missing assets
offer **Download**. Installed rows mark the selected
version **Enabled**, independently of whether the process is running. The top
status bar shows that selected version even while stopped. Confirmed stopped
state displays zero uptime and `0 B/s` upload/download rates; unavailable or
uncertain observations still remain unknown. Available assets are filtered using the
deployed panel binary's GOOS/GOARCH, not browser/device detection. Enabling rejects incompatible artifacts
before process control, then validates the saved configuration before replacement.
Selecting while stopped does not launch sing-box. Selecting while running
restarts it only after preflight succeeds. Start, stop and restart are available
in the top status bar; stop retains the selected version. The selected row offers
**Disable**, which stops the process and clears selection only after successful
completion. A disable request is bound to that artifact and cannot disable a
different selection. After disabling, select a version before starting again.

Enabling another version is a single serialized operation: the running old
process is stopped before the new process starts, and the committed selection
and current symlink are replaced atomically. A failed preflight preserves the
old selection and process; the browser does not chain separate disable/enable
requests that could leave a valid version disabled when validation fails.

The committed activation bundle records the selected artifact. After completion,
`<data_dir>/artifacts/current` is atomically updated as a relative symlink to its
immutable `sha256/.../sing-box` binary. The panel reconciles the link from persisted
selection at startup, including after a crash between database commit and link
publication. Runtime execution continues to open and verify the immutable path,
so changing the convenience symlink cannot bypass digest or configuration checks.

The compact import dialog accepts a single `.tar.gz`/`.tgz` archive by drag-and-drop
or file selection, and asks for its exact version. Standard sing-box filenames
fill the version automatically; custom
filenames require manual entry, and the suggested version remains editable.
Source metadata records the filename and the variant is `musl`. Recognized
non-musl or wrong-architecture filenames are rejected by the browser. Both CLI
and HTTP imports reject other variant identities. Custom archive names are
allowed when the operator supplies a musl build for the server architecture.

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

- sing-box 1.13.19; and
- sing-box 1.14.0.

The reviewed source catalog at `internal/singbox/catalog.json` records each
exact tag and commit plus the module sums, amd64 and arm64 asset name, URL,
size, SHA-256, musl feature fingerprint, behavior family, and upstream Go identity.
It is the version and official-artifact lock. The old 1.11.15/1.12.25 profiles
and inbound converters are removed because those releases have no musl assets.

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
verifies static linking without a dynamic interpreter or shared-library
dependencies, and requires that binary to accept representative raw configuration with
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
Import adds a version to the installed list; enable selects the binary while
preserving the running/stopped state. Removal unregisters an unused version and keeps existing reference checks.
Archive format, size, platform, checksum and exact-version checks still detect
invalid files, incompatible binaries and changed bytes.

The source kind `user_verified` identifies an administrator-imported archive.
Archive verification runs before registration; there is no additional approval state.

## Adding a stable version

A patch release in a known release line normally needs one catalog entry.
Inbound-conversion behavior may reuse a reviewed family only when its supported
types remain identical. Schema identity is simpler: each exact version either
has its own native output or is JSON-only.

Adding support is intentionally manual:

1. Review the upstream stable release and both official Linux musl artifacts.
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
