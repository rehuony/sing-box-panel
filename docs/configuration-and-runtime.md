# Configuration and runtime

sing-box-panel maintains one lossless sing-box JSON history. Selecting another
core never creates another configuration branch, and saving configuration
never changes the live process implicitly.

## State model

The control plane separates four layers:

1. The **configuration revision** stores one strict JSON object.
2. A **startup artifact** binds those immutable JSON bytes, the revision, and
   one exact core artifact; it becomes ready only after a real
   `sing-box check` succeeds.
3. An **activation bundle** binds one ready startup artifact to a monitoring
   tier. Applied and rollback pointers advance only after runtime health
   verification succeeds.

Subscriptions remain live authorization and rendering state; activation
bundles do not freeze channel response bodies or third-party source snapshots.

## One global JSON history

The minimal document is:

```json
{}
```

The document is the executable sing-box configuration. Top-level keys follow
sing-box concepts (`log`, `dns`, `ntp`, `certificate`, `endpoints`, `inbounds`,
`outbounds`, `route`, `services`, and `experimental`). The panel does not add
`_panel`, disabled-item markers, or another storage envelope to these bytes.

Create the first revision with an empty compare-and-swap base:

```sh
sing-box-panel config import \
  --file ./configuration.json \
  --base-revision none
```

Later `set`, `unset`, `replace`, and restore operations require the current
revision. Stale writes fail instead of merging implicitly. `show`, `get`,
`export`, `diff`, and `revision` inspect immutable state.

## Switching versions without rewriting JSON

Selecting a different core carries the current JSON forward unchanged. The
panel does not infer which keys an older release accepts and does not silently
drop fields. Compilation snapshots the selected revision and asks that exact,
digest-verified binary to validate the same bytes:

```sh
sing-box-panel config compile \
  --artifact CORE_ARTIFACT_ID
```

If the configuration uses fields unavailable in that version, `sing-box check`
fails, the startup artifact becomes failed, and the currently applied runtime
is left untouched. There is no separate editable startup JSON or
version-specific transformation path.

## Reviewed configuration schemas

The authenticated endpoint
`GET /api/v1/core/artifacts/{artifactId}/configuration-schema` returns the
reviewed Draft 2020-12 JSON Schema for the artifact's exact version. Schema
selection deliberately ignores the artifact commit, binary digest, feature
fingerprint, variant, and architecture: the official configuration contract is
version-scoped. The response contains only `exact_version`, `schema_sha256`,
and the Schema. Its ETag binds the exact version and digest and supports
`If-None-Match`.

The panel currently commits native Schema output for `1.14.0`. Releases before
sing-box added `sing-box schema`, including `1.11.15`, `1.12.25`, and `1.13.19`,
remain Advanced-JSON-only; the panel does not synthesize schemas for them. A
missing Schema disables only structured controls, never JSON save,
compile, check, Apply, Start, Restart, or Rollback.

The committed Schema is canonicalized native output with optional `x-panel`
presentation metadata such as section, order, widget, sensitivity, and
bilingual labels. The metadata cannot add `_panel`, introduce a disabled-item
union, fill cross-version fields, or change a sing-box constraint. References
remain local to that one Schema document.

The Web build exports each committed Schema and compiles its root validator into
an Ajv 2020 standalone module. The generated manifest records only the exact
version, file, and digest. At runtime the browser selects by exact version and
verifies the served digest and content against its local asset. An unavailable
Schema or mismatch falls back to the lossless Advanced editor; the production
UI does not fetch schemas from the network or relax the Content Security Policy
with `unsafe-eval`.

Structured controls edit and validate only fields known to the reviewed form
contract while preserving unshown JSON properties. The Advanced editor owns
the complete effective configuration. Any visible validation error, invalid
Advanced JSON, or active save locks all mutations while read-only revision and
activation history remains available.

## Check and apply

Compilation atomically snapshots immutable configuration bytes and queues a durable
check against the selected binary. The CLI waits for that check by default, so
the normal flow does not require a separate `core check` command:

```sh
sing-box-panel config compile --artifact CORE_ARTIFACT_ID
sing-box-panel config apply --artifact READY_STARTUP_ARTIFACT_ID
```

With `config compile --detach`, wait for the returned task ID before applying
the candidate. `core check` remains an explicit operation for an existing
pending startup artifact; only a ready artifact can be applied.

Apply rechecks the configuration head, artifact trust, and startup evidence. A
concurrent configuration or trust change cannot be combined with stale bytes.

`process_only` monitoring checks process health. `limited` additionally
requires the final configuration to expose a secret-protected Clash API on a
loopback address and complete the `/version` handshake.

## Lifecycle and rollback

```sh
sing-box-panel core status
sing-box-panel core start
sing-box-panel core restart
sing-box-panel core rollback
sing-box-panel core stop
```

Start reuses the last applied bundle and remains idempotent for the same
running identity. Restart always performs a real stop/start transition.
Rollback uses the previous immutable bundle and its corresponding configuration
and binary evidence; it does not reinterpret current configuration.

Runtime and maintenance operations are durable tasks with leases,
cancellation, attempts, and terminal results stored in SQLite. The server
holds the process-level runtime executor lease; CLI processes inspect state and
enqueue work without becoming a second runtime manager.

## Runtime identity and history

A running `RuntimeIdentity` includes the observed `started_at` value together
with the verified PID, process start token, exact core, artifact hashes, and
activation bundle. While that identity remains verified, the panel refreshes
its persisted observation watermark every 30 seconds so a later recovery can
distinguish current evidence from a stale process incarnation.

Runtime lifecycle evidence is append-only and permanently retained in
`runtime_transitions`. The authenticated
`GET /api/v1/core/runtime/history` endpoint filters by time, state, reason, or
activation bundle and paginates newest-first with the stable
`occurred_at`/`id` cursor pair. Transitions use `running`, `stopped`, `failed`,
or `unknown`; an `unknown` record includes `uncertain_since` and represents an
interval that cannot be classified as uptime or downtime.

Task-driven lifecycle success, controlled stop, abnormal exit, and failed
health or version handshake commit the final runtime observation, transition,
runtime intent, and task terminal state in one SQLite transaction. If evidence
cannot be committed, the task remains recoverable instead of recording a false
terminal result. Recovery, reconciliation, startup coordination, and shutdown
history that is not owned by a task remains an independently appended record.
Stable dedupe keys make retries idempotent, and verified PID/start-token
fencing prevents a late task from changing a newer process incarnation.
Database initialization adds one `history_initialized` marker and does not
infer earlier history from tasks or logs.
