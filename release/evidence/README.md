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
build requires its non-zero lowercase full `RELEASE_COMMIT` to equal that
actual `HEAD`, rejects every staged, modified, or untracked source outside the
allowlist above, validates the ledger and record digests, and then embeds the
validated overlay. Ignore rules and hidden tracked-file index flags do not hide
other inputs, and inherited Go workspace/overlay settings are disabled. The
formal path recreates locked Web dependencies, then revalidates the source and
reruns readiness after rebuilding the Web distribution, so the exact evidence
bytes embedded by the subsequent Go build have passed the ledger gate. No
application source or additional evidence filename is accepted through this
exception.
