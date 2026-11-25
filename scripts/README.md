# Project scripts

This directory owns executable project scripts. `installer.sh` installs a
published release on a Linux host. `build.sh` builds and verifies an
isolated source snapshot. `test/` contains local script tests and the
shared release smoke orchestration plus the native Linux sing-box core
contract. GitHub workflow YAML and the release signing trust root remain under
`.github/`.

## Release installer

`installer.sh` supports Linux amd64 and arm64 and installs one release
component per run: `--component panel` (`sing-box-panel`) or `--component node`
(`sing-box-node`). Without `--component` it asks on the terminal and never
picks one itself; without a terminal the option is required. Before choosing,
it reports each component's installed binary and version for the effective
user and the state of its systemd unit, changing nothing. With no other
arguments it installs the latest published stable release;
`--version vMAJOR.MINOR.PATCH` selects one exact release. It downloads the
target binary, `SHA256SUMS`, and `SHA256SUMS.sig`. `SHA256SUMS` must list
exactly the four release binaries. It obtains the Ed25519 trust root exclusively from
`.github/keypair/release-signing-public-key`, then verifies the manifest
signature and binary checksum before executing or installing the binary. The
installer contains no copied public-key value.

The installer chooses the component's layout from the effective user:

- root installs the binary at `/usr/local/bin/NAME`; the panel's default
  settings are at `/etc/sing-box-panel/setting.json`, and default data is under
  `/var/lib/NAME`;
- another user installs the binary under `~/.local/bin` and uses the current
  XDG configuration and data homes.

The node's settings are at `/etc/sing-box-node/setting.json` for root and
under the XDG configuration home otherwise; its data directory holds its
enrollment.

Installation only writes the binary. Settings and data are neither read nor
validated nor initialized, so malformed leftovers do not block replacement. An
existing binary requires interactive confirmation; use `--yes` for unattended
replacement. Questions are answered on the line that asks them. Declining
exits without download or replacement. `server start` initializes missing settings and storage later.
`systemd install --now` also creates missing settings and service directories;
there is no prerequisite `init` or foreground start. Formatted logs use color on a
terminal unless `NO_COLOR` is set: one aligned level label per message (`ERROR`
red, `WARN` and `CONFIRM` yellow, `INFO` cyan, `OK` green). curl runs silently,
so no download progress of its own appears. The final summary aligns the binary
and default settings and data paths like the binaries' own output, then lists
the start command, for a node the enrollment command, the `systemd install`
commands and the Bash, Fish and Zsh completion commands. When the component's
service is installed, it says that the service still runs the previous binary
and gives the `systemd install` command that refreshes its unit, which an
earlier release may have written with other arguments, before `systemd restart`.
The installer does not modify shell profiles or configure, start, stop, or
restart systemd. Run its network-independent contract tests locally with:

```sh
bash scripts/test/installer-test.sh
```

## Release automation

`build.sh` is used by local Make targets, CI, and the signed-release
workflow. It never publishes, uploads, signs, installs, or retains artifacts.
`test/smoke-release.sh` validates signed artifacts and exercises native startup,
node enrollment, editable configuration and panel settings persistence,
authenticated self-update, restart, and a node update through the panel. `test/release-contract.sh` builds disposable candidates from the
current working tree, signs them with a newly generated test key, and invokes
that same scenario in ordinary CI and local Linux checks. It never reads the
repository's private key or replaces the committed public key. Temporary keys,
binaries and instance data are removed on exit.

Working-tree smoke builds preserve the caller's effective `GOPATH`, `GOMODCACHE`
and `GOCACHE`, including paths saved with `go env -w`, while fixing the Go build
settings. The third-party notices generator preserves the same cache paths when
listing dependencies for both Linux architectures.

The `Release Build` workflow supplies the actual signed release artifacts to
the shared scenario, then creates a verified Draft Release for a maintainer
to publish. The isolated packaging check remains separate from the working-tree
smoke build: it validates the formal build's committed inputs and trust root.

## Interface

Use the Make targets from the repository root:

```sh
make snapshot OUT=/absolute/path/to/new-output
make release VERSION=v0.1.0 OUT=/absolute/path/to/new-output
make release-verify
make release-smoke # current working tree; native Linux, non-root user
```

Their underlying script interface is:

```sh
scripts/build.sh snapshot --output /absolute/path/to/new-output
scripts/build.sh release --version v0.1.0 --output /absolute/path/to/new-output
scripts/build.sh verify
bash scripts/test/release-contract.sh # requires both web/apps/*/dist from make build-frontend
```

Release smoke tests require Bash, curl, Git, Go, jq, OpenSSL, Python 3 and
sha256sum. `make release-smoke` builds the Web distribution first, using the same
Node.js/Corepack prerequisites as `make build`. `RELEASE_ARCHITECTURE`, when
provided by CI, must match the native runner; emulation is not a fallback.
These native checks are separate from the cross-platform `make ci` target.

The release configuration fixture is `testdata/release-configuration.json`.
The HTTP contract test uses the same fixture against the real handler,
reopened SQLite database, and `api/panel-openapi.yaml`. The native scenario additionally
tests actual process restart and binary replacement. When changing a public
configuration contract, update that contract, its implementation, this fixture
when needed, and the shared smoke scenario together. Do not add a second
release-only API representation or bypass a failing assertion.

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
2. downloads and verifies Go modules with isolated caches and inherited
   workspaces, overlays, experiments, and persistent Go settings disabled;
3. installs and builds the Web application inside that complete source
   snapshot using the package-pinned pnpm, frozen lockfile, disabled lifecycle
   scripts, and an isolated store; the explicit build command runs `generate`
   for OpenAPI types before TypeScript checking and Vite bundling, without
   reading generated files from the caller's working tree;
4. requires each fresh Web distribution, the panel's and the node console's,
   to contain `index.html`, the single public `favicon.svg`, and bundled
   assets before loading the Go package that embeds it;
5. cross-builds `sing-box-panel` and `sing-box-node` for Linux amd64 and
   arm64 with `CGO_ENABLED=0`, fixed CPU baselines, `-trimpath`, and
   `-buildvcs=false`;
6. embeds and verifies the committed update-verification key in all four
   binaries, since the panel and its nodes both install signed releases;
7. verifies each command's embedded release version, full source commit, and
   source timestamp through a host-native metadata probe built with the same
   flags;
8. generates and verifies `SHA256SUMS` in staging before atomically renaming
   the complete output directory.

The resulting directory contains exactly:

- `sing-box-panel-linux-amd64`
- `sing-box-panel-linux-arm64`
- `sing-box-node-linux-amd64`
- `sing-box-node-linux-arm64`
- `SHA256SUMS`

The workflow adds `SHA256SUMS.sig` only after the isolated build is complete.
Its Ed25519 signature binds the formal version and exact manifest bytes. The
environment private key must match the public key committed in the source
snapshot. Local signing is available through `go tool sign-release`; the
private key is stored locally at the Git-ignored
`.github/keypair/release-signing-private-key.pem` and is never an input to
the packaging script. Packaging also rejects a source commit containing that
path, even if someone force-added it through the ignore rule.

The script never moves or edits the caller's `web/node_modules` or `web/apps/*/dist`.
All uncommitted files are intentionally ignored: use `make build` when testing
working-tree source changes.

`verify` exercises the formal-release path with the committed public key. It
requires both architectures to build successfully, checks their Go build
metadata and key, checks the embedded release identity, and confirms that
invalid release versions fail without leaving an output directory.

See [Core versions](../docs/core-versions.md) for
the supported sing-box release lines. See
[Release process](../docs/release.md) for signing-key setup, native
amd64 and arm64 smoke tests, Draft Release verification, manual publication,
trust bootstrap, and the release-hardening backlog.
