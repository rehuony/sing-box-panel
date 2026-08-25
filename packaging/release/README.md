# Release build materials

`scripts/build-release.sh` builds the two supported Linux targets without
publishing, uploading, signing, installing, or modifying repository files.

Prerequisites:

```sh
corepack enable pnpm
(cd web && pnpm install --frozen-lockfile --ignore-scripts --verify-store-integrity)
(cd web && pnpm run build)
```

Build into a new output directory:

```sh
RELEASE_VERSION=v0.1.0 \
RELEASE_COMMIT="$(git rev-parse HEAD)" \
RELEASE_DATE=2026-08-26T00:00:00Z \
./scripts/build-release.sh /absolute/path/to/output
```

The script refuses to overwrite any expected output and creates:

- `sing-box-panel-linux-amd64`
- `sing-box-panel-linux-arm64`
- `SHA256SUMS`

Both binaries are built with `CGO_ENABLED=0`, `-trimpath`, and the `webdist`
tag. CI writes them only to `RUNNER_TEMP`; no workflow in this repository
publishes or retains them. A future release workflow must be reviewed as a
separate privileged change before it can upload these files.

`RELEASE_VERSION` must be set. Only the exact values `dev` and `ci` may build
while GA readiness is false, and they print the open-gate warning. Every other
value must be strict v-prefixed SemVer and passes through the executable GA gate
before any binary is built; unset, empty, arbitrary, and malformed values fail
closed.

GA requires both an embedded SQLite runtime of 3.53.4 or newer and a complete
embedded `release/evidence.json` ledger. Ledger records pin reviewed evidence
documents by source commit and SHA-256. The checked-in ledger is intentionally
incomplete, so upgrading SQLite alone cannot accidentally enable a release.
