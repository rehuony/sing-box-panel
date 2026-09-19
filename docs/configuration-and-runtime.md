# Configuration and runtime

sing-box-panel exposes one editable `config.json` and maintains immutable valid
JSON revisions as internal runtime evidence. Selecting another
core never creates another configuration branch, and saving configuration
never changes the live process implicitly.

## State model

The control plane separates four layers:

1. The **configuration file** stores exact UTF-8 text, including unfinished JSON.
   Authenticated `GET/PUT /api/v1/config/file` uses a numeric compare-and-swap
   revision, preserves whitespace and large numbers, and never starts a process.
2. A **configuration revision** stores one immutable strict JSON object. Valid
   file saves synchronize that head atomically. Invalid saves keep prior history
   but cannot compile or fall back to it.
3. A **startup artifact** binds those immutable JSON bytes, the revision, and
   one exact core artifact; it becomes ready only after a real
   `sing-box check` succeeds.
4. An **activation bundle** binds one ready startup artifact to a monitoring
   tier. Applied and rollback pointers advance only after runtime health
   verification succeeds.

Subscriptions remain live authorization and rendering state; activation
bundles do not freeze channel response bodies or third-party source snapshots.

## One editable file and internal JSON history

The minimal document is:

```json
{}
```

The document is the executable sing-box configuration. Top-level keys follow
sing-box concepts (`log`, `dns`, `ntp`, `certificate`, `endpoints`, `inbounds`,
`outbounds`, `route`, `services`, `certificate_providers`, `http_clients`,
`network_namespaces`, and `experimental`). The panel does not add
`_panel`, disabled-item markers, or another storage envelope to these bytes.

The Web editor and the CLI write the same file. The CLI saves it with the
numeric file revision as the compare-and-swap base, starting at `0`:

```sh
sing-box-panel config import \
  --file ./config.json \
  --revision 0
```

`config show` and `config export` return the stored text exactly, including an
unfinished draft; `config show --output json` adds `revision`, `syntax_valid`,
and `canonical_revision_id`. A stale `--revision` fails instead of merging
implicitly. The logical name `config.json` is not a filesystem path: the text
lives in the `configuration_file` table of `panel.db` inside `data_dir`, and
there is no second writable on-disk runtime configuration.

CLI `config get` reads one JSON-pointer value of the current valid file.
`config set` and `unset` edit a value. These writes take `--base-revision`, the
`canonical_revision_id` of the current valid snapshot, and report their own
canonical revision. Read `config show --output json` again before a subsequent
whole-file import to obtain the current numeric revision and text. These
operations refuse to run while the saved file is invalid; correct the text
with `config import` or the Web editor first. Each valid write
synchronizes the editable file in the same transaction. Revision values are
concurrency guards for the current file, not a user-facing history workflow.
Neither the CLI nor the browser offers historical configuration selection,
comparison, or restoration. Immutable internal evidence remains where native
checks, runtime identities and activation recovery require it.

## Switching versions without rewriting JSON

Selecting a different core carries the current JSON forward unchanged. The
panel does not infer which keys an older release accepts and does not silently
drop fields. A check snapshots the current valid file and asks that exact,
digest-verified binary to validate the execution snapshot. Snapshot formatting
may differ from the saved text, but field names and values are preserved and
the saved text is untouched:

```sh
sing-box-panel config check --core CORE_ARTIFACT_ID
```

If the configuration uses fields unavailable in that version, `sing-box check`
fails, the startup artifact becomes failed, and the currently applied runtime
is left untouched. There is no separate editable startup JSON or
version-specific transformation path. The Web UI performs the same check
through `POST /api/v1/config/compile`.

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
the complete effective configuration. The browser can save invalid JSON; it disables visual editing and binary
validation until the text is a valid object. Saving and checking lock editing
until the result arrives, so feedback describes the submitted file. Validation
success is a Toast shown only after the check task succeeds. Unknown fields and
numeric lexemes are retained through visual edits.

## Check and apply

A check atomically snapshots the immutable configuration bytes and queues a
durable `sing-box check` against the selected binary without touching the live
core. Apply snapshots the same current file for preflight in the serialized
runtime lane and restarts the core with it only after that check succeeds:

```sh
sing-box-panel config check                       # applied core
sing-box-panel config apply                       # checked restart, applied core kept
sing-box-panel config apply --core CORE_ARTIFACT_ID
sing-box-panel core enable CORE_ARTIFACT_ID       # same switch as apply --core
```

Both commands default to the currently applied core and require `--core` before
any core has been applied. Both wait for the durable task by default and
expose `--detach`. The Web UI drives the same internals through
`POST /api/v1/config/compile`, `POST /api/v1/core/artifacts/{artifactId}/enable`,
and the runtime endpoints; startup artifacts and activation bundles remain
internal evidence rather than CLI inputs.

Apply rechecks the current file, canonical head, artifact trust, and startup
evidence. A concurrent configuration or trust change cannot be combined with
stale bytes, and a failed preflight leaves the running core and the saved file
unchanged.

The monitoring tier follows the saved configuration: `limited` when the final
configuration exposes a secret-protected Clash API on a loopback address and
completes the `/version` handshake, otherwise `process_only`, which checks
process health only. Nothing is injected into the file to create that endpoint.

## Lifecycle and rollback

```sh
sing-box-panel core status
sing-box-panel core start
sing-box-panel core restart
sing-box-panel core rollback
sing-box-panel core stop
```

Start and Restart use the last applied binary identity and the current saved
file. A changed file is snapshotted for binary preflight in the serialized runtime
lane. The desired process and current observation remain unchanged until that
check passes. The worker rechecks the current file, canonical head, artifact trust,
generation and lease before binding the checked candidate. Invalid text, failed
checks, superseded intents and concurrent edits cannot replace the live process.

Start remains idempotent for unchanged configuration and the same running
identity. A changed configuration cannot implicitly restart an already running
core: use Restart. Restart always performs a real stop/start transition after
preflight. A save alone never stops, starts or reloads a service. Runtime status
returns `loaded_canonical_revision_id` only for a verified live process; the UI
compares this identity with the saved file to distinguish loaded and
restart-required states.
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


## Panel settings and protocol identity

The panel's own settings live at authenticated `GET/PUT /api/v1/panel/settings`
and are separate from sing-box configuration. The singleton revision is a
compare-and-swap boundary; stale edits fail without overwriting newer settings.
The three UI categories are service/security, nodes/subscriptions, and
statistics/appearance. Listener/origin changes require a panel restart. A
management-token change invalidates existing sessions. GitHub credentials and
traffic quota are read at operation boundaries. Credential reads expose only
configured flags; an omitted credential preserves it, and GitHub removal is
explicit.

The public-node host accepts a public IP or domain override; otherwise the
bounded public-IP detector provides the input placeholder and publication host.
It never rewrites the actual listener, port, TLS server name or imported node
address. Detection failure must not publish loopback or wildcard addresses.

Saving a changed common protocol name/key updates matching managed credentials
in the saved configuration in the same transaction. It preserves other existing
users, external client nodes and protocol-specific obfuscation secrets. UUID-
based protocols receive a UUID; Shadowsocks 2022 receives a method-sized key.
An invalid saved configuration or concurrent edit rejects the whole change.
The running core keeps its existing bytes until a checked restart.
`POST /api/v1/config/inbound-defaults` creates an authenticated, no-store editor
entry using that identity. It does not save or launch anything; the user reviews
and saves the new inbound through the normal configuration flow.

Appearance offers six presets/custom HEX and radius 0–32px (default 24).
Preview changes page, controls, charts and dialogs immediately while semantic
status colors and the logo stay independent. Card/dialog radius is R, controls
R/2, and the shell min(32,7R/6). Saving persists preferences; changing category
retains edits, leaving the page restores saved appearance. Reset changes only
theme color/radius and still requires saving. Help is in hover/focus tips.
