# Configuration and runtime

sing-box-panel maintains one version-independent configuration history for all
sing-box releases. Selecting another core never creates another configuration
branch and saving configuration never changes the live process implicitly.

## State model

The control plane separates four layers:

1. The **canonical revision** stores global schema-v2 superset intent.
2. An exact compiled **adapter projection** produces candidate sing-box JSON
   for one installed core artifact.
3. A **startup artifact** binds projected bytes, adapter identity, canonical
   revision, and core artifact; it becomes ready only after a real
   `sing-box check` succeeds.
4. An **activation bundle** binds one ready startup artifact to a monitoring
   tier. Applied and rollback pointers advance only after runtime health
   verification succeeds.

Subscriptions remain live authorization and rendering state; activation
bundles do not freeze channel response bodies or third-party source snapshots.

## One global schema-v2 history

The minimal document is:

```json
{"schema_version":2,"configuration":{}}
```

Top-level configuration keys follow sing-box concepts (`log`, `dns`, `ntp`,
`certificate`, `endpoints`, `inbounds`, `outbounds`, `route`, `services`, and
`experimental`). Managed collection items include panel metadata:

```json
{
  "schema_version": 2,
  "configuration": {
    "outbounds": [
      {
        "_panel": {"id": "direct", "enabled": true},
        "type": "direct",
        "tag": "direct"
      }
    ]
  }
}
```

The `_panel` object supplies stable management identity and enablement; it is
removed from executable output. Disabled collection items are retained in
history but omitted from projections.

Create the first revision with an empty compare-and-swap base:

```sh
sing-box-panel config import \
  --file ./canonical.json \
  --base-revision none
```

Later `set`, `unset`, `replace`, and restore operations require the current
revision. Stale writes fail instead of merging implicitly. `show`, `get`,
`export`, `diff`, and `revision` inspect immutable state.

## Switching versions without losing fields

Projection is read-only. A supported field is copied or explicitly mapped;
an unavailable field is omitted from that version's executable JSON and
reported as an `ignored` diagnostic. The field remains byte-for-byte part of
the global revision and becomes available again when a supporting core is
selected.

For example, the reviewed 1.11.15 adapter retains but ignores top-level
`certificate` and `services`. The 1.12.25 and 1.13.19 adapters accept those
top-level sections for validation by their exact core. No adapter deletes the
global value.

Before compilation, the Web/API preview reports `direct`, `mapped`, `ignored`,
or `blocking` diagnostics. Ignored fields produce a stable digest. CLI
compilation requires the operator to pass that exact preview digest,
preventing a stale approval from accepting a different ignored set:

```sh
sing-box-panel config compile \
  --artifact CORE_ARTIFACT_ID \
  --accept-ignored IGNORED_DIGEST
```

Without ignored fields, omit `--accept-ignored`. There is no manual startup
JSON path; every executable configuration is derived from the single canonical
revision through a reviewed exact adapter.

## Check and apply

Compilation atomically creates immutable projected bytes and queues a durable
check against the selected binary. The CLI waits for that check by default, so
the normal flow does not require a separate `core check` command:

```sh
sing-box-panel config compile --artifact CORE_ARTIFACT_ID
sing-box-panel config apply --artifact READY_STARTUP_ARTIFACT_ID
```

With `config compile --detach`, wait for the returned task ID before applying
the candidate. `core check` remains an explicit operation for an existing
pending startup artifact; only a ready artifact can be applied.

Apply rechecks the canonical head, adapter revision, artifact trust, and
startup evidence. A concurrent configuration or trust change cannot be
combined with stale bytes.

`process_only` monitoring checks process health. `limited` additionally
requires the final configuration to expose a secret-protected Clash API on a
loopback address and complete the `/version` handshake. `full` remains
explicitly unavailable.

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
Rollback uses the previous immutable bundle and its corresponding canonical
and adapter evidence; it does not re-project current configuration.

Runtime and maintenance operations are durable tasks with leases,
cancellation, attempts, and terminal results stored in SQLite. The server
holds the process-level runtime executor lease; CLI processes inspect state and
enqueue work without becoming a second runtime manager.
