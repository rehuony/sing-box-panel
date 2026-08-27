# Release process

The repository builds static Linux artifacts and can sign and retain a formal
release through a protected, manually dispatched GitHub Actions workflow. It
does not create or publish a GitHub Release. Local builds, reproducible
snapshots, and privileged formal releases are intentionally separate
workflows.

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
make release VERSION=v0.1.0 \
  UPDATE_PUBLIC_KEY_FILE=/secure/path/to/release-signing-public-key \
  OUT=/absolute/path/to/new-output-directory
```

The recorded date defaults to the `HEAD` commit timestamp. Supply an explicit
RFC 3339 value only when the release process requires one:

```sh
make release VERSION=v0.1.0 \
  DATE=2026-08-27T00:00:00Z \
  UPDATE_PUBLIC_KEY_FILE=/secure/path/to/release-signing-public-key \
  OUT=/absolute/path/to/new-output-directory
```

The public-key file contains one standard-Base64 Ed25519 public key. It is
embedded in the formal binaries and is not secret. The commit is always
derived from `HEAD`; the release interface does not accept caller-provided
commit metadata or the former `RELEASE_*` environment variables.

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

## Signing-key setup

Create the Ed25519 key pair once on a trusted machine:

```sh
openssl genpkey -algorithm ED25519 -out release-signing-private-key.pem
go tool sign-release public-key \
  --private-key release-signing-private-key.pem \
  >release-signing-public-key
```

Configure a protected GitHub environment named `release`. Restrict it to the
default branch and require an approving reviewer. In that environment, store:

- the complete PEM file as the `RELEASE_SIGNING_PRIVATE_KEY` secret;
- the single-line public-key value as the `RELEASE_SIGNING_PUBLIC_KEY`
  variable.

Keep the private key outside the repository and backups under equivalent
access controls. The workflow checks that the secret and configured public key
are a pair before creating a signature. Keep this key pair stable: replacing
it makes existing installations unable to authenticate later releases. A
future rotation therefore requires an explicit transition design with both
keys trusted before the old key is retired.

The first release containing an embedded public key is a trust bootstrap.
Older unsigned builds cannot authenticate it through `update` and must be
installed through an independently verified manual path once.

For an authorized local release, create and immediately verify the detached
signature after packaging:

```sh
go tool sign-release sign \
  --private-key release-signing-private-key.pem \
  --public-key release-signing-public-key \
  --version v0.1.0 \
  --checksums /absolute/path/to/output/SHA256SUMS \
  --signature /absolute/path/to/output/SHA256SUMS.sig
go tool sign-release verify \
  --public-key release-signing-public-key \
  --version v0.1.0 \
  --checksums /absolute/path/to/output/SHA256SUMS \
  --signature /absolute/path/to/output/SHA256SUMS.sig
```

## Outputs and verification

Snapshot and local formal builds create exactly:

- `sing-box-panel-linux-amd64`
- `sing-box-panel-linux-arm64`
- `SHA256SUMS`

The binaries use `CGO_ENABLED=0`, fixed CPU baselines, `-trimpath`,
`-buildvcs=false`, and the `webdist` tag. Files are assembled and verified in
a staging directory, then the whole directory is renamed into place so a
failed build cannot leave partial output.

The manual `Build signed release` workflow accepts a strict release version
and optional build date. It runs only from the default branch, passes the
environment public key to the isolated build, signs the completed
release version and `SHA256SUMS` with the environment private key, verifies
both the signature and checksums, and retains these four workflow-artifact
files:

- `sing-box-panel-linux-amd64`
- `sing-box-panel-linux-arm64`
- `SHA256SUMS`
- `SHA256SUMS.sig`

If an authorized operator publishes a GitHub Release manually, all four files
must be attached without renaming them. `sing-box-panel update` verifies the
signature with its embedded public key before trusting the checksum manifest
or downloading a replacement binary. The workflow does not create the GitHub
Release or upload assets to it.

Exercise the complete snapshot path and the current formal-gate outcome with:

```sh
make release-verify
```

For the exact script interface and environment isolation rules, see
[Release build materials](../packaging/release/README.md). Publishing to a
GitHub Release remains a separate privileged operation.
