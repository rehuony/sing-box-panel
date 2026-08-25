<!-- SPDX-License-Identifier: GPL-3.0-or-later -->

# Capability manifests

This directory is the reviewed, declarative source for exact-version sing-box
capabilities. Runtime code accepts only the built-in manifest schema and fixed
transformation primitives. Scripts, templates, plugins, WASM, executable code,
and remote `$ref` values are rejected.

Repository files use `capabilities/<major>.<minor>.<patch>.json`. A release
generation package is created from one immutable commit and has this shape:

```json
{
  "schema_version": 1,
  "repository": "rehuony/sing-box-panel",
  "commit_sha": "0123456789abcdef0123456789abcdef01234567",
  "manifest_count": 1,
  "manifests": [
    {
      "path": "capabilities/1.13.19.json",
      "manifest_sha256": "<SHA-256 of canonical standalone manifest JSON>",
      "manifest": {
        "schema_version": 1,
        "core_version": "1.13.19",
        "support_level": "manual_json",
        "semantic_facts": []
      }
    }
  ]
}
```

Build that envelope offline from an isolated directory containing only
complete, strictly named `<major>.<minor>.<patch>.json` manifests:

```sh
sing-box-panel core capability pack \
  --directory ./manifest-set \
  --commit 0123456789abcdef0123456789abcdef01234567 \
  --file GENERATION.json
```

`pack` opens no settings, database, or network connection. It strictly decodes
every manifest, requires the file name to equal its declared exact version,
canonicalizes and hashes each standalone manifest, sorts entries by semantic
version, and writes the commit-bound generation atomically. Existing output is
not replaced unless `--force` is explicit; use `--file -` for canonical JSON on
stdout. Symbolic links, non-regular entries, empty directories, and size or
entry-count limit violations are rejected.

This command packages manifests that an author has supplied; it does not infer
support, derive baseline/delta manifests, or create missing version contracts.

`sing-box-panel core capability refresh --file GENERATION.json` validates and
atomically stores the complete package as candidates; it never changes a pin.
Inspect a candidate by exact version, commit, and digest. Run `upgrade` once to
preview it, then repeat with `--accept` to move that exact-version pin.

An operator can permanently remove trust from a manifest digest without
deleting its audit evidence:

```sh
sing-box-panel core capability quarantine \
  --sha256 MANIFEST_SHA256 --reason security_advisory
```

The first quarantine reason, diagnostics, and timestamp are immutable. There
is no unquarantine path, and accepting or repacking the same bytes cannot make
the digest usable again. A quarantined pin immediately falls back to
`manual_json` for new work.

An exact stable release with no pinned, usable manifest remains in
`manual_json`; the panel never substitutes a manifest from `latest` or another
version. Stored pins and generations are sufficient offline and therefore act
as last-known-good state.

For a revalidated, unquarantined `native_structured` or
`compatible_structured` pin, the management API may expose only the manifest's
semantic facts and inert UI descriptors. The panel renders those descriptors
with built-in controls; transforms, scripts, components, templates, and remote
resources are never sent as presentation instructions. Compatible controls
require explicit operator acceptance before editing or saving.
