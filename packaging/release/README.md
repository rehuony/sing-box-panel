# Release build materials

`build-release.sh` is the repository's only release-packaging implementation.
It builds but never publishes, uploads, signs, installs, or retains artifacts.

## Interface

From the repository root:

```sh
packaging/release/build-release.sh snapshot --output /absolute/path/to/new-output
packaging/release/build-release.sh release --version v0.1.0 --output /absolute/path/to/new-output
packaging/release/build-release.sh release --version v0.1.0 --output /absolute/path/to/new-output --date 2026-08-27T00:00:00Z
packaging/release/build-release.sh verify
```

The Make targets `snapshot`, `release`, and `release-verify` are the normal
developer and CI entry points. The destination of `snapshot` and `release`
must not exist, and its parent directory must already exist.

`snapshot` uses version `dev` and skips only GA authorization. `release`
requires a strict v-prefixed SemVer and a successful readiness check. Both
derive the full source commit from `HEAD`; the build date defaults to that
commit's timestamp.

## Isolated build model

The script:

1. exports committed `HEAD` to a private temporary source tree;
2. replaces only the six permitted evidence paths with regular, non-symlink
   files from the working tree when present;
3. installs and builds the Web application in another temporary tree using
   the package-pinned pnpm, frozen lockfile, disabled lifecycle scripts, and
   an isolated store;
4. verifies the Web distribution before copying it into the source snapshot;
5. downloads and verifies Go modules with isolated caches and inherited
   workspaces, overlays, experiments, and persistent Go settings disabled;
6. cross-builds Linux amd64 and arm64 with `CGO_ENABLED=0`, fixed CPU
   baselines, `webdist`, `-trimpath`, and `-buildvcs=false`;
7. generates and verifies `SHA256SUMS` in staging before atomically renaming
   the complete output directory.

The resulting directory contains:

- `sing-box-panel-linux-amd64`
- `sing-box-panel-linux-arm64`
- `SHA256SUMS`

The script never moves or edits the caller's `web/node_modules` or `web/dist`.
Uncommitted non-evidence files are intentionally ignored: use `make build`
when testing working-tree source changes.

Evidence is a release-authorization input consumed by the repository-only
readiness tool. It is not embedded in the shipped product binary.
