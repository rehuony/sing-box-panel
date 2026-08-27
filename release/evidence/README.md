# Release evidence records

This directory is intentionally empty of release evidence while the project is
not GA-ready. `release/evidence.json` is the machine-readable ledger and the Go
release gate owns the exact list of required record IDs.

Each future `<requirement-id>.json` must be reviewed with the ledger entry that
pins its SHA-256, source commit, completion time, reviewer, and review time. The
document must use schema version 1, report `result=pass`, and contain at least
one named passing check with a concise evidence reference.

Formal builds use a deliberately narrow evidence overlay. Only these paths may
differ from the checked-out commit:

- `release/evidence.json`
- `release/evidence/core-version-matrix.json`
- `release/evidence/structured-capability-matrix.json`
- `release/evidence/linux-runtime-resilience.json`
- `release/evidence/browser-contract-accessibility.json`
- `release/evidence/subscription-observability-e2e.json`

The overlay resolves an unavoidable self-reference: committing a ledger that
names its own resulting commit would change that commit. Every ledger and
record instead names the immutable source `HEAD` that was reviewed. A formal
build derives that commit directly from Git, exports the committed source, and
then replaces only the paths above with regular, non-symlink files from the
working tree. Deleting one of those working-tree files also deletes it from the
snapshot. All other staged, modified, untracked, and ignored files are outside
the build context.

The repository-only readiness tool embeds and validates this overlay, including
the ledger and record digests, before authorizing a formal build. Evidence is
not linked into the shipped `sing-box-panel` binary. Inherited Go workspaces,
overlays, experiments, persistent settings, and ordinary dependency caches are
disabled for the isolated build. No application source or additional evidence
filename is accepted through the overlay.
