# Core versions and capabilities

sing-box-panel separates upstream release metadata, locally installed binary
artifacts, and reviewed configuration capabilities. A version appearing in one
layer does not imply trust or support in another.

## Exact-version model

Stable sing-box versions use the canonical form `MAJOR.MINOR.PATCH`. Artifacts
are additionally identified by Linux architecture, variant, archive digest,
binary digest, and immutable artifact ID. Multiple artifacts for one exact
version may coexist; commands require an artifact ID when the choice is
ambiguous.

There is no current-minor allowlist in the artifact or manual-configuration
path. An older or future stable release can use the same workflow when a
matching Linux binary and the required verification evidence are available.
This extensibility is not a claim that every sing-box release has been tested.

For commands where `--core-version` is optional, omission uses the actual
running version. See [Exact core-version resolution](cli.md#exact-core-version-resolution)
for the complete rule.

## Official catalog and installation

Refresh the cached stable-release catalog, inspect installable assets, and
install one by its GitHub asset ID:

```sh
sing-box-panel core catalog refresh
sing-box-panel core catalog list --installable
sing-box-panel core install ASSET_ID
sing-box-panel core list
```

The catalog pins both the `SagerNet/sing-box` repository name and its immutable
GitHub repository ID. Release entries and downloaded bytes remain separate from
installed artifacts. Official installation requires trusted digest evidence;
an asset without that evidence may be listed but is not installable.

The installed binary is accepted only after safe archive extraction, ELF
operating-system and architecture checks, binary hashing, and execution of
`sing-box version`. The reported version must match the requested exact
version. Reported build tags are stored as a sorted feature fingerprint; a
banner without a `Tags:` line is recorded as `not_reported`, not as an empty
feature set.

## Import an administrator-verified archive

An older or custom stable build can be imported when an administrator supplies
its expected identity and independently verified archive SHA-256:

```sh
sing-box-panel core import \
  --file ./sing-box.tar.gz \
  --sha256 ARCHIVE_SHA256 \
  --version 1.13.19 \
  --arch amd64 \
  --variant plain
```

Import applies the same extraction, ELF, binary-digest, and reported-version
checks as official installation. The archive must be a bounded, canonical
`tar.gz` with one regular file named `sing-box`; links, traversal, duplicate
binaries, multiple gzip members, unsafe entry types, and trailing data are
rejected.

## Artifact trust boundary

The official path relies on GitHub HTTPS, validation of the immutable upstream
repository identity, available release digest evidence, and local SHA-256
verification. The administrator-import path relies on the operator to obtain
and verify the expected archive digest through a trusted channel.

The project does not currently provide TUF metadata, project-owned artifact
signatures, or an independent transparency log. These checks therefore do not
claim to protect against compromise of the upstream repository, its authorized
release process, or the operator's digest source. Release distribution and
signing would require a separate security review.

## Trust reduction

An installed artifact begins usable only after verification. Both restriction
commands lower trust permanently for the immutable bytes:

```sh
sing-box-panel core quarantine ARTIFACT_ID
sing-box-panel core revoke ARTIFACT_ID
```

Revocation is terminal, and reinstalling or importing the same bytes cannot
clear quarantine or revocation. A restriction blocks all new checks,
activations, starts, restarts, and rollbacks that would use those bytes. It
requests cancellation of affected work and fences a desired runtime
generation, but does not kill an already running child process.

## Exact-version capabilities

Structured editing is enabled only by a locally stored manifest for the exact
core version, pinned to an immutable repository commit and canonical manifest
SHA-256. The supported levels are:

- `native_structured`: reviewed structured ownership for the exact version;
- `compatible_structured`: reviewed compatibility that requires explicit
  operator acceptance; and
- `manual_json`: lossless manual configuration without inferred structured
  ownership.

No manifest from `latest`, another patch release, or another minor release is
substituted. A missing, invalid, or quarantined exact pin falls back to
`manual_json` for new work.

An offline author can package a complete reviewed set and an operator can load,
inspect, and explicitly accept one exact pin:

```sh
sing-box-panel core capability pack \
  --directory ./manifest-set --commit COMMIT_SHA --file GENERATION.json
sing-box-panel core capability refresh --file GENERATION.json
sing-box-panel core capability inspect \
  --core-version 1.13.19 --commit COMMIT_SHA --sha256 MANIFEST_SHA256
sing-box-panel core capability upgrade \
  --core-version 1.13.19 --commit COMMIT_SHA --sha256 MANIFEST_SHA256
sing-box-panel core capability upgrade \
  --core-version 1.13.19 --commit COMMIT_SHA --sha256 MANIFEST_SHA256 --accept
```

`pack` validates, canonicalizes, hashes, and envelopes supplied manifests. It
does not infer support, generate a baseline or delta, or create missing version
contracts. Capability generations are currently loaded from a file; unlike the
release catalog, they are not fetched from GitHub by the panel.

Capability quarantine is digest-scoped and permanent:

```sh
sing-box-panel core capability quarantine \
  --sha256 MANIFEST_SHA256 --reason security_advisory
```

The first reason, diagnostics, and timestamp remain immutable audit evidence.
There is no unquarantine path. Pins and complete generations are stored locally
and remain sufficient for offline last-known-good operation.

The management UI may receive only semantic facts and inert descriptors from a
revalidated usable pin. Manifest-supplied scripts, components, templates,
transforms, executable code, and remote resources are not accepted. See the
authoritative [capability manifest format](../capabilities/README.md).
