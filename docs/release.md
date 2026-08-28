# Release process

This guide is for maintainers producing signed Linux releases. It describes
the packaging and publication boundary; it is not a deployment guide or a
sing-box core compatibility matrix.

The release path has one responsibility at each boundary:

1. CI proves that one exact source commit passes the repository checks.
2. An Ed25519 signature authenticates the release version and checksum
   manifest.
3. Native Linux amd64 and arm64 smoke tests exercise the packaged binaries and
   authenticated self-update.
4. GitHub Actions creates and verifies a Draft Release.
5. A maintainer reviews and publishes that draft.

The workflow never publishes a release automatically. All required checks are
either reproducible CI/workflow jobs or the maintainer's final draft review.

## Verification scope

The ordinary `CI` workflow runs Go and contract checks, race and fuzz smoke
tests, Web and notice checks, and isolated package verification. The repository
has no Docker E2E harness or Docker CI job. Tests remain beside their owning Go
or Web packages.

The manually dispatched signed-release workflow adds native amd64 and arm64
smoke tests for the packaged panel binary, HTTP startup, persistent state, and
authenticated self-update. Those release smoke tests do not download or run a
sing-box core. Exact core/config compatibility is enforced later by the
installed artifact profile, compiled adapter, and real `sing-box check` on the
operator host.

## Local development build

Build the current working tree, including uncommitted source changes:

```sh
make bootstrap
make build
```

This writes `bin/sing-box-panel`. It is the appropriate command while
developing, but it is not the release packaging path.

## HEAD snapshot

Build both supported Linux architectures from committed `HEAD`:

```sh
make snapshot OUT=/absolute/path/to/new-output-directory
```

The output directory must not already exist and its parent must already be a
directory. A snapshot uses version `dev`; its commit and date come from
`HEAD`. Uncommitted application changes, untracked files, ignored files, and
existing `web/node_modules` or `web/dist` do not enter or block the build.

## Formal release

A local formal build accepts strict v-prefixed SemVer:

```sh
make release VERSION=v0.1.0 OUT=/absolute/path/to/new-output-directory
```

The public Ed25519 update-verification key is committed at
`.github/keypair/release-signing-public-key` and is embedded in both binaries.
It is the repository's release trust root, not a secret or a per-build input.
The release commit and date are always derived from `HEAD`; the date is the
source commit timestamp and cannot be overridden by the caller.

The packaging command creates exactly:

- `sing-box-panel-linux-amd64`
- `sing-box-panel-linux-arm64`
- `SHA256SUMS`

The signing step adds `SHA256SUMS.sig`. Its Ed25519 signature binds the exact
release version and the exact checksum-manifest bytes, preventing the
manifest from being reused for a different version.

## Isolated build model

Snapshot and release builds share the implementation in
`.github/scripts/build-release.sh`. It exports committed `HEAD` into a
temporary source tree, builds the Web application from a separate temporary
copy with locked dependencies, and copies only the verified `web/dist` into
the source snapshot. Go module, build, and package-manager state is isolated
from the caller's normal caches.

Both Linux binaries use `CGO_ENABLED=0`, fixed CPU baselines, `-trimpath`,
`-buildvcs=false`, and the `webdist` tag. Files are assembled and verified in a
staging directory, then the whole directory is renamed into place so a failed
build cannot leave a partial destination.

## One-time repository setup

Create the signing key once on a trusted machine:

```sh
release_keypair_dir=.github/keypair
mkdir -p -- "${release_keypair_dir}"
openssl genpkey -algorithm ED25519 \
  -out "${release_keypair_dir}/release-signing-private-key.pem"
chmod 0600 "${release_keypair_dir}/release-signing-private-key.pem"
go tool sign-release public-key \
  --private-key "${release_keypair_dir}/release-signing-private-key.pem" \
  >"${release_keypair_dir}/release-signing-public-key"
```

Review and commit `.github/keypair/release-signing-public-key`. The private PEM
lives locally at `.github/keypair/release-signing-private-key.pem`; the
repository's exact `.gitignore` rule excludes it from Git. Git ignore is not
encryption or a backup, so keep a second copy offline under equivalent access
controls and never force-add the private file. Release packaging rejects any
source commit that contains this private-key path. Configure the GitHub
repository as follows:

1. Create an environment named `release` and restrict it to the default
   branch.
2. Add the complete private PEM as the environment secret
   `RELEASE_SIGNING_PRIVATE_KEY`. Do not add the public key as a secret or
   environment variable; the committed file is authoritative.
3. Do not require an environment reviewer. The maintainer's single approval
   point is publishing the verified Draft Release.
4. Protect the default branch and require the `ci.yml` checks to pass before
   merge.
5. Push the repository and confirm that CI succeeds before attempting the
   first release.

For example, after creating the `release` environment, upload the ignored
local PEM without printing it:

```sh
gh secret set RELEASE_SIGNING_PRIVATE_KEY \
  --env release \
  <.github/keypair/release-signing-private-key.pem
```

The workflow derives the public key from the private key and rejects the
secret if it does not match the committed trust root. Keep the key pair stable:
existing installations cannot authenticate a replacement key without an
explicit transition release that trusts both keys.

The first release containing this public key is a trust bootstrap. Unsigned,
development, and older release builds cannot authenticate that transition
through `update`; install the first signed release once through an
independently verified manual path.

## Local signing and verification

An authorized maintainer can sign a local formal build and verify it
immediately:

```sh
release_keypair_dir=.github/keypair
go tool sign-release sign \
  --private-key "${release_keypair_dir}/release-signing-private-key.pem" \
  --public-key "${release_keypair_dir}/release-signing-public-key" \
  --version v0.1.0 \
  --checksums /absolute/path/to/output/SHA256SUMS \
  --signature /absolute/path/to/output/SHA256SUMS.sig
go tool sign-release verify \
  --public-key "${release_keypair_dir}/release-signing-public-key" \
  --version v0.1.0 \
  --checksums /absolute/path/to/output/SHA256SUMS \
  --signature /absolute/path/to/output/SHA256SUMS.sig
```

Use the same strict SemVer validator as signing and self-update when checking
automation input:

```sh
go tool sign-release validate-version --version v0.1.0
```

The validator accepts the complete strict SemVer contract, including valid
prerelease and build metadata. The GitHub release workflow is intentionally
narrower and accepts only a stable `vMAJOR.MINOR.PATCH` version.

## Automated Draft Release

Manually dispatch `Build signed release` from the default branch and provide
the stable release version. The workflow freezes the full source commit and
runs four stages:

1. `guard` rejects a non-default branch, an invalid or non-stable version, an
   existing tag or Release, and a source commit whose latest `ci.yml` push run
   did not succeed.
2. `build-sign` builds the isolated amd64 and arm64 artifacts, verifies their
   build metadata, checksum set, and committed public key, checks that the
   environment private key matches that public key, and signs the checksum
   manifest.
3. `smoke` runs independently on native `ubuntu-24.04` and
   `ubuntu-24.04-arm` runners. It verifies the target binary's release version,
   source commit, source date, build metadata, key, checksums, and signature;
   initializes and verifies settings; starts the server; checks the health and
   authenticated API; writes persistent state; and exercises a real update
   from a lower-version probe through a temporary release endpoint. The test
   confirms that the running process remains unchanged until restart and that
   both the new version and SQLite state survive the restart. The arm64 job
   never falls back to QEMU. The Actions-only orchestration is implemented by
   `.github/scripts/smoke-release.sh`.
4. `draft` obtains `contents: write` only after both smoke jobs pass. It creates
   a Draft Release targeted at the frozen commit, generates release notes,
   uploads the four exact assets, downloads them into an empty directory, and
   re-verifies the asset set, bytes, checksums, signature, version, source
   commit, and embedded public key.

The workflow never replaces an existing tag, Release, or asset. If draft
verification fails or is cancelled after creation, cleanup is limited to the
draft and tag created for the same target commit by that workflow run.

## Publish a release

For every release:

1. Merge the release source into the default branch and wait for all required
   CI checks to pass.
2. Dispatch `Build signed release` with the next stable version.
3. Wait for build, signing, and both native smoke jobs to succeed.
4. Open the generated Draft Release and review its version, target commit,
   generated notes, and four assets.
5. Publish the draft once. Only a published, non-prerelease GitHub Release is
   visible to `sing-box-panel update`.

Exercise the formal build, version rejection, and output-cleanup behavior
locally with:

```sh
make release-verify
```

For the exact script interface and environment-isolation rules, see
[Release automation](../.github/scripts/README.md).

## Release-hardening backlog

The following work should expand automated release coverage.

### P1

- Test a real previous stable release upgrading to the candidate, including
  database migrations, instead of relying only on a lower-version build of the
  same source.
- Verify systemd installation, startup, update, restart, and failed-update
  recovery in clean virtual machines.
- Add failure injection for insufficient disk space, permissions, interrupted
  downloads, killed processes, and executable replacement failures.
- Pin and run `govulncheck`; block only for an applicable vulnerability or a
  documented CVE requirement rather than an arbitrary SQLite version floor.
- Design a signing-key rotation protocol that lets existing clients trust a
  transition release before the old key is retired.

### P2

- Add independent double-build comparison, an SBOM, and GitHub provenance
  attestations.
- Expand supported-distribution, browser, and long-running tests over time.
  Track incomplete coverage as issues until it can be verified automatically;
  do not convert it into hand-authored publication blockers.
