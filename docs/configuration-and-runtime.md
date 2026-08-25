# Configuration and runtime

sing-box-panel separates operator intent from generated startup bytes and from
the process that is actually running. Saving configuration never silently
changes the live sing-box process.

## State model

The control plane maintains three related layers:

1. **Canonical configuration** is the global, version-neutral operator intent.
   Each successful change creates an immutable revision.
2. **Startup artifacts** are exact bytes generated or supplied for one exact
   verified core artifact and one canonical revision. They remain pending until
   a real `sing-box check` succeeds.
3. **Activation bundles** freeze the checked startup artifact, core identity,
   capability evidence, and subscription publication inputs used by an apply.
   Applied and rollback pointers advance only after the child passes health
   verification.

This separation makes configuration save, validation, activation, restart, and
rollback explicit operations with auditable inputs.

## Canonical revisions and compare-and-swap

The minimal canonical schema is:

```json
{"schema_version":1,"global":{},"nodes":[],"rules":[],"subscription":{}}
```

Create the first revision with `--base-revision none`:

```sh
sing-box-panel config import \
  --file ./canonical.json \
  --base-revision none
```

Every later mutation supplies the current revision ID. Stale writes fail with
a conflict instead of merging implicitly:

```sh
sing-box-panel config set /global/log_level \
  --file ./value.json \
  --base-revision REVISION_ID
```

`config show`, `get`, `export`, `diff`, and `revision` commands inspect current
or immutable state. `node` and `rule` commands update their canonical
collections under the same revision contract.

## Structured workflow

Structured projection requires a verified core artifact and a usable
capability pin for that exact core version. Compatible support also requires an
explicit `--allow-compatible` decision.

```sh
sing-box-panel config render \
  --core-version 1.13.19 \
  --artifact CORE_ARTIFACT_ID
```

The render transaction rechecks the canonical head, exact pin, manifest
digest, support level, and quarantine state before it creates the immutable
startup artifact and its check task. A concurrent canonical or trust change
cannot be combined with stale evidence.

## Manual JSON workflow

Manual JSON is the fallback for any verified stable binary without a proven
structured capability:

```sh
sing-box-panel config manual preview \
  --file ./sing-box.jsonc \
  --base-revision REVISION_ID \
  --core-version 1.13.19 \
  --artifact CORE_ARTIFACT_ID
sing-box-panel config manual replace \
  --file ./sing-box.jsonc \
  --base-revision REVISION_ID \
  --core-version 1.13.19 \
  --artifact CORE_ARTIFACT_ID
```

`manual preview` is read-only. When an exact capability proves that fields are
owned and losslessly reversible, the preview reports the canonical partial,
proposed canonical document, and residual manual-owned paths. Compatible
ownership is used only after explicit acceptance. Without usable exact
evidence, canonical intent remains unchanged and every discovered path stays
manual-owned.

The save operation recomputes that evidence instead of trusting a previous UI
preview. If capability evidence changed, it retries at most once with a safe
all-manual fallback. It never commits stale reverse-mapping evidence.

Saved UTF-8 JSONC bytes preserve comments, whitespace, number lexemes, and key
order exactly. Duplicate keys and boundedness violations are rejected. The
canonical revision, manual startup artifact, and check task are created
atomically. `config manual show` emits the raw secret-bearing bytes.

A later canonical change makes an unapplied manual candidate stale. Use
`config manual reattach preview` and `config manual reattach apply` for an
explicit base/current/manual three-way reconciliation. Reattachment creates a
new artifact; it does not mutate the old one.

## Check and apply

Every startup artifact is bound to its exact core artifact and canonical
revision and must pass a real core check before activation:

```sh
sing-box-panel core check STARTUP_ARTIFACT_ID
sing-box-panel config apply --artifact STARTUP_ARTIFACT_ID
```

Apply prepares an immutable bundle, starts the selected exact binary with its
frozen startup bytes, and advances applied and rollback state only after health
success. Failed health verification leaves the previous applied state intact.

## Start, restart, and rollback

```sh
sing-box-panel core status
sing-box-panel core start
sing-box-panel core restart
sing-box-panel core rollback
sing-box-panel core stop
```

Start and restart reuse the last applied bundle. Rollback reuses the previous
frozen bundle rather than rendering it again against current state. All paths
recheck artifact and capability restrictions before starting new work.

## Runtime executor and durable work

The server owns the runtime and maintenance workers. Runtime intents use a
monotonic generation: newer work supersedes queued older work and requests
cancellation of older running work at safe boundaries. Tasks retain their
status, result, failure metadata, attempts, and lease state in SQLite.

Before opening SQLite, `server run` acquires a non-blocking process-level
runtime executor lease in the data directory and holds it for its lifetime.
This prevents two panel servers from managing the same child. Ordinary CLI
processes may still open the WAL database, inspect state, and enqueue work.

See [CLI durable task semantics](cli.md#durable-tasks-and-cancellation) for
interactive control and [Subscriptions and observability](subscriptions-and-observability.md)
for the publication data frozen into an activation bundle.
