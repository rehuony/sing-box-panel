# Project scripts

This directory owns executable project scripts. `installer.sh` installs a
published release on a Linux host. `build-release.sh` builds and verifies an
isolated source snapshot. `test/` contains local script tests and the
GitHub Actions-only native release smoke orchestration. GitHub workflow YAML
and the release signing trust root remain under `.github/`.

## Release installer

`installer.sh` supports Linux amd64 and arm64. With no arguments it installs
the latest published stable release; `--version vMAJOR.MINOR.PATCH` selects one
exact release. It downloads the target binary, `SHA256SUMS`, and
`SHA256SUMS.sig`. It obtains the Ed25519 trust root exclusively from
`.github/keypair/release-signing-public-key`, then verifies the manifest
signature and binary checksum before executing or installing the binary. The
installer contains no copied public-key value.

The installer chooses the existing application layout from the effective user:

- root installs the binary at `/usr/local/bin/sing-box-panel`, settings at
  `/etc/sing-box-panel/setting.json`, and default data under
  `/var/lib/sing-box-panel`;
- another user installs the binary under `~/.local/bin` and uses the current
  XDG configuration and data homes.

Existing settings and data are retained and verified. Missing settings are
initialized through the verified release binary. The installer does not
modify shell profiles or configure, start, stop, or restart systemd; it prints
the appropriate explicit `system install` and `system restart` commands after
installation. Run its network-independent contract tests locally with:

```sh
make installer-test
```

## Release automation

`build-release.sh` is used by local Make targets, CI, and the signed-release
workflow. It never publishes, uploads, signs, installs, or retains artifacts.
`test/smoke-release.sh` is GitHub Actions-only orchestration for native release
smoke tests. The `Release Build` workflow adds the signature, runs those
smoke tests, and creates a verified Draft Release for a maintainer to publish.

## Interface

Use the Make targets from the repository root:

```sh
make snapshot OUT=/absolute/path/to/new-output
make release VERSION=v0.1.0 OUT=/absolute/path/to/new-output
make release-verify
```

Their underlying script interface is:

```sh
scripts/build-release.sh snapshot --output /absolute/path/to/new-output
scripts/build-release.sh release --version v0.1.0 --output /absolute/path/to/new-output
scripts/build-release.sh verify
```

The destination of `snapshot` and `release` must not exist, and its parent
directory must already exist. `snapshot` uses version `dev`. `release`
requires strict v-prefixed SemVer and embeds the standard-Base64 Ed25519 public
key committed at `.github/keypair/release-signing-public-key`. Both modes
derive the full source commit and build date from `HEAD`; the build date is the
source commit timestamp and has no caller override.

The manually dispatched GitHub workflow applies an additional release policy:
its version must be stable `vMAJOR.MINOR.PATCH`, without prerelease or build
metadata.

Key paths are centralized at their two execution boundaries. The packaging
script derives both filenames from `release_keypair_dir`; the workflow derives
all public-key references from its top-level `RELEASE_KEYPAIR_DIR`. Keeping one
directory variable in each environment avoids another configuration format
while making a future directory move a two-location change.

## Isolated build model

The script:

1. exports committed `HEAD` to a private temporary source tree;
2. installs and builds the Web application in another temporary tree using
   the package-pinned pnpm, frozen lockfile, disabled lifecycle scripts, and an
   isolated store;
3. verifies the Web distribution before copying it into the source snapshot;
4. downloads and verifies Go modules with isolated caches and inherited
   workspaces, overlays, experiments, and persistent Go settings disabled;
5. cross-builds Linux amd64 and arm64 with `CGO_ENABLED=0`, fixed CPU
   baselines, `webdist`, `-trimpath`, and `-buildvcs=false`;
6. embeds and verifies the committed update-verification key in both release
   binaries;
7. verifies the embedded release version, full source commit, and source
   timestamp through a host-native metadata probe built with the same flags;
8. generates and verifies `SHA256SUMS` in staging before atomically renaming
   the complete output directory.

The resulting directory contains exactly:

- `sing-box-panel-linux-amd64`
- `sing-box-panel-linux-arm64`
- `SHA256SUMS`

The workflow adds `SHA256SUMS.sig` only after the isolated build is complete.
Its Ed25519 signature binds the formal version and exact manifest bytes. The
environment private key must match the public key committed in the source
snapshot. Local signing is available through `go tool sign-release`; the
private key is stored locally at the Git-ignored
`.github/keypair/release-signing-private-key.pem` and is never an input to
the packaging script. Packaging also rejects a source commit containing that
path, even if someone force-added it through the ignore rule.

The script never moves or edits the caller's `web/node_modules` or `web/dist`.
All uncommitted files are intentionally ignored: use `make build` when testing
working-tree source changes.

`verify` exercises the formal-release path with the committed public key. It
requires both architectures to build successfully, checks their Go build
metadata and key, checks the embedded release identity, and confirms that
invalid release versions fail without leaving an output directory.

See [Release process](../docs/release.md) for signing-key setup, native
amd64 and arm64 smoke tests, Draft Release verification, manual publication,
trust bootstrap, and the release-hardening backlog.
