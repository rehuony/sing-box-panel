# Subscriptions and observability

Public subscription output combines immutable applied runtime state with live,
administrator-managed authorization. Metrics are exposed only when a real
collector sample exists.

## Subscription keys and legacy grants

The Web UI exposes subscription sources, keys, and channels. New keys do not
require a subscription user. They authorize the nodes allowed by each channel's
publication policy. A key has a label, optional exclusive expiry time, and an
optional download limit (1–1,000,000,000). Plaintext is returned only by creation
or rotation; only its digest is stored. Key creation does not choose a channel
or generate a subscription URL. Channel distribution settings accept an existing
plaintext key to copy that channel's subscription URL; the key is held only while
the dialog is open and is never included in saved channel configuration.
The key list shows labels without a legacy-access description and exposes enable/disable,
rotation and deletion directly, without a detail dialog or a revoke action.
Any unrevoked key can be rotated, whether enabled, disabled, expired or exhausted.
Rotation replaces its secret while preserving enablement, scope, quota and usage;
expiry is retained unless explicitly replaced through the API. Disabled keys
remain disabled after rotation, and revoked keys cannot be rotated again.
Column proportions and action-button widths stay fixed when key status changes;
narrow viewports scroll the table within its panel.
Disabling or deleting a key invalidates future requests, not credentials already
downloaded by a client. Existing API revocation and rotation invalidation remain
supported; revoked keys cannot be re-enabled.

Source and channel forms omit enablement controls. New records are enabled;
editing existing records preserves their stored enablement state.
Source names, including the manual collection, are display-only; use the row's
Edit button to open the source workspace.
Sources can be removed directly from the list after confirmation. The creation
dialog sizes to its fields, and subscription actions use visible button surfaces.

Migration retains every existing key's user ID, digest, expiry, usage and grants.
Those keys continue to require an enabled user and exact node grants; an empty
grant set still renders an empty subscription. Existing user/grant management
APIs remain available for compatibility, but are no longer a Web management tab.
A key governed by channel publication policies has no user ID. The authenticated preview accepts an omitted
user ID to show the channel policy, or a legacy user ID to preview that user's
restricted output. No migration silently expands an existing key's access.

The public endpoint remains `GET /sub/{token}/{channelId}`. A key's body-response
counter is shared across all channels. After rendering succeeds, an atomic write
rechecks enablement, revocation, expiry, legacy user status and remaining quota
before committing a 200 response. Concurrent requests cannot exceed the limit.
A 304 response increments the request counter but not downloads; failed
validation, authorization or rendering consumes neither. Accounting describes
server-committed responses: HTTP cannot prove that a remote client received all
bytes after a connection failure. Rotation replaces the secret while retaining
usage, quota and scope; omission of a replacement expiry retains the old expiry.

Disabled users and disabled, revoked, expired, exhausted, deleted or unknown keys
share one public not-found response. Management responses contain metadata only.
No request address, user agent, plaintext key or response content is recorded in
usage statistics.

## Applied local nodes and versioned sources

Local nodes are derived only from the immutable startup bytes referenced by
the currently applied activation bundle. Draft, failed, and unapplied
configuration is never published. Rollback changes the applied bundle pointer
and therefore restores the matching local-node input without re-projecting the
current revision.

The inbound registry accepts only the exact reviewed releases `1.11.15`,
`1.12.25`, `1.13.19`, and `1.14.0`; other versions fail closed. Each converter publishes
only the client-usable inbound types available in that release and reports
stable diagnostics for server-only or unsupported types. Multi-user inbounds
become separate grantable credentials for legacy access compatibility. The panel public-host override, existing channel `public_host`, or detected
public IP combines with each inbound `listen_port`; server certificate private keys,
ACME configuration, and listen-side fields are never copied.

The current exact inbound contracts are:

| Core | Convertible local inbound types |
| --- | --- |
| `1.11.15` | `mixed`, `socks`, `http`, `shadowsocks`, `vmess`, `trojan`, `hysteria`, `shadowtls`, `vless`, `tuic`, `hysteria2` |
| `1.12.25` | All 1.11.15 types plus `anytls` |
| `1.13.19` | All 1.12.25 types plus `naive` |
| `1.14.0` | All 1.13.19 types plus `snell` |

For these versions, `direct`, `tun`, `redirect`, `tproxy`, and
`cloudflared` are explicitly unpublishable. Any other inbound type currently
produces an unsupported-type diagnostic; it is not guessed from a nearby
version.

Third-party sources accept sing-box JSON, Mihomo YAML, or plaintext/Base64
share-link lists. URI lists strictly recognize `ss`, `socks`/`socks5`,
`http`/`https`, `vmess`, `vless`, `trojan`, `hysteria`, `hy2`/`hysteria2`,
`tuic`, and `anytls`. Successful versions are append-only and store a raw
digest, normalized nodes, detected format, fetch time, and diagnostics. An
older successful version can be restored as current. Parse or fetch failure
never replaces the current version.

Remote refresh is a durable maintenance task. It is disabled on a schedule by
default; a configured interval is at least 15 minutes. Fetches allow at most
4 MiB, 20 seconds, and five redirects. DNS and every redirect target are
checked against the SSRF policy. Loopback, link-local, and private addresses
are denied unless `subscription.private_source_cidrs` explicitly allows them.
Public subscription requests never fetch upstream data.

Channel policy, user grants, token state, and source current-version pointers
take effect immediately. The public handler reads these values, the applied
startup artifact, and enabled source versions in one consistent SQLite read.

## Manual publication and channels

Manual nodes store native client-node JSON with optimistic concurrency. Local
automatically derived nodes and manual nodes share the “manual nodes” collection;
source membership and node names are independent. The source list folds legacy
local-source records into that collection without changing their IDs. Cards use
actual source names, with no invented local/self-hosted node-name prefix.
Node cards use a compact, wrapping grid with badges aligned directly below the
header. Hidden cards retain their content beneath a frosted overlay, while the
name, visibility and detail controls remain accessible. The masked body is not
interactive; reduced transparency and increased contrast use an opaque overlay.
Publication IDs remain stable
across credential updates. Hiding a node keeps it recoverable at the source but
omits it from channel selection views and downloads. It does not delete the
inbound, channel membership, or existing legacy grants.

Channel configuration accepts a typed policy: selected/excluded publication IDs,
new-node include/exclude policy, organizer options, ordered rule groups and a
final exit. The channel editor lists strategy groups in a left sidebar and edits
the selected group's nodes and exit rules on the right. Group drafts survive
switching groups and are persisted together by Save changes. Adding a node to a
group also selects it for the channel. Referenced exits must be changed before
removing their nodes or deleting/disabling the fallback group.
Existing groups retain their candidate snapshot when new nodes arrive.
Unavailable, hidden or cyclic node dependencies cannot silently become direct
traffic: unavailable designated exits become reject actions. Group and native
node names must not collide with generated reserved names.

Remote rule sets store metadata only. Sing-box uses source JSON or binary SRS;
Mihomo uses YAML, TEXT or MRS with native behavior (MRS excludes classical).
The client fetches the final URL; the panel never fetches, counts, uploads or
converts rule content. GitHub acceleration unwraps known proxies and prefixes
eligible original URLs with `https://gh-proxy.com/`, without a GitHub credential.

Per-channel templates use native JSON/YAML and cannot replace generated nodes,
groups, routing rules, providers or fallback. Sing-box templates and final output
are schema-checked against reviewed 1.14.0, independently of the server's selected
core; the subscriber still owns runtime validation. Mihomo checks YAML structure
and reserved fields; it does not execute a Mihomo runtime check. Diagnostics
report field paths and fixed error codes without reflecting submitted secrets.

Authenticated preview can accept an unsaved draft without persisting it. Preview
and public delivery share the renderer. JSON preview responses preserve the
existing byte/Base64 contract; the Web adapter decodes before display/copy.

The native single-node editor uses reviewed 1.14 fields, with basic address and
credentials first and optional protocol, TLS, transport, multiplexing and dial
sections. Explicit protocol/mode changes clear incompatible known options;
unknown extension fields and large numeric lexemes survive unrelated edits.
The advanced node JSON editor formats valid content on load, on entry and on
blur and save, and provides a format button. Invalid or incomplete text is left intact;
formatting preserves large numeric lexemes and unknown fields.
HY2 supports a single port, port ranges or Realm, SSH supports password/key/key
file, and Shadowsocks UDP-over-TCP and multiplexing are mutually exclusive.
QUIC does not expose uTLS/Reality or TCP fragmentation. Detour references must
resolve within the manual-node collection and cannot introduce cycles. Server
validation rejects invalid known fields without echoing credential values.

## Target renderers

The sing-box renderer preserves validated publishable nodes in sing-box JSON.
The cross-format renderers convert only their explicit current contracts:

| Target | Explicitly converted node types |
| --- | --- |
| Mihomo | `shadowsocks`, `socks`, `http`, `vmess`, `vless`, `trojan`, `hysteria2`, `tuic`, `anytls` |
| Loon | `shadowsocks`, `socks`, `http`, `vmess`, `vless`, `trojan`, `hysteria2`, `anytls` |

Unsupported types, transports, TLS shapes, networks, dependencies, or options
are omitted with stable positional diagnostics. Renderers never infer a field
mapping that is not implemented and tested.

Mihomo mapping also covers supported WebSocket/gRPC transport options,
uTLS/Reality and stream multiplexing. External YAML parsing uses the same
explicit option contract: an unhandled option fails the refresh rather than
silently dropping transport, security or routing semantics. The previous source
version remains active. Sing-box native nodes preserve their original supported
options; unsupported cross-format nodes produce diagnostics.

## Core and panel log views

The Web page has two tabs. **Real-time logs** shows sanitized sing-box output,
with a muted timestamp and the entire remaining message colored by TRACE,
DEBUG, INFO, WARN, ERROR, FATAL or PANIC. A file selector and level filter sit on
the right; search, pause/resume and LIVE state operate on a bounded local buffer.
**Panel logs** combines each durable task's current state with standalone panel
and runtime events once. The table shows time, message, log level and source,
without an actions column. Every row uses the API log level for both presentation
and filtering, including task rows; task lifecycle states do not replace levels.
Task links still open readable operation status and guidance;
task IDs and raw result/failure metadata stay internal. Failed/canceled
catalog refresh, official core installation and source refresh can queue a fresh
validated attempt. Runtime commands and temporary-file imports require a new
explicit operation, never replay of stale payloads.

`/api/v1/core/logs/files`, `/api/v1/core/logs/content` and
`/api/v1/core/logs/stream` expose only managed file names and bounded byte
cursors. The collector captures child stdout/stderr. If native `log.output` is
set, it follows new bytes from that regular file, handling creation, truncation
and rotation without reading pre-existing contents or modifying native config.
Disabled output is not followed; symbolic-link files and capture-directory loops
are rejected. On child exit, pending output is drained and its final partial
line is flushed before completion is reported.

Private core-log retention is at most 32 files of 32 MiB, rotated by UTC date and
size. Rotation switches only the destination file; the process output pipes and
file follower remain connected. Each day's sequence advances from its newest
retained file, including after a panel restart, and extends beyond three digits
when necessary. The active file is never removed by retention. Lines and read
chunks are bounded; ANSI sequences and known credentials
are sanitized. The browser resumes from the received cursor, freezes the selected
file while paused and polls for rotation. Streaming uses write deadlines and
closes within a minute to reauthenticate on reconnect. `/api/v1/logs/panel`
provides the combined panel view. The legacy log API below remains available.

## Durable logs

The log CLI and authenticated API expose bounded, sanitized metadata for
panel, core, task, and security events:

```sh
sing-box-panel log list
sing-box-panel log show LOG_ID
sing-box-panel log tail --follow
sing-box-panel log clear --before TIMESTAMP
sing-box-panel log delete LOG_ID
```

`GET /api/v1/logs/stream` is a durable server-sent event stream and accepts
`Last-Event-ID` for reconnection. `logs.retention_days` is enforced at server
startup and every 24 hours. Configuration bytes, subscription bodies, token
plaintext, URL credentials, and known secret fields are not stored as log
payloads.

## Dashboard delivery and host metrics

`GET /api/v1/metrics/stream` pushes runtime and metric snapshots every two seconds,
with bounded writes and a one-minute authenticated reconnect. Polling recovers
stream interruptions. Repeated collector timestamps do not replace the last
valid transfer rate with zero. Linux host CPU/memory/disk metrics are separate
from sing-box process samples; unsupported hosts report unavailable values.

The dashboard shows host summaries, transfer history and one-hour active
connections. Graph gaps remain gaps. The 24-hour runtime strip uses 48 equal
segments and persisted transitions; unknown intervals are not guessed healthy.

## Limited monitoring and traffic

`process_only` reports child-process health without counters. `limited`
requires the final configuration to enable a
Clash API on a numeric loopback address with a non-empty secret. The panel
reads that configuration and never modifies startup bytes.

Apply, start, and restart first pass process health, then wait up to five
seconds for `/version` to report the exact selected core version. While the
process runs, the panel reads `/connections` every ten seconds and persists
memory, active connections, `uploadTotal`, and `downloadTotal`. A sample older
than 30 seconds is stale.

Counters are checkpointed by PID and OS start token. A restart opens a new
segment and preserves the UTC natural-month period total. A decrease inside
one process is stored as rejected diagnostic evidence and cannot lower totals.
Cross-period checkpoints retain the last process counters but add only the
delta proven inside the new natural period, so a long-lived process cannot
re-add its lifetime counters each month. Samples and period contributions use
nullable upload/download deltas: legacy or interrupted intervals that cannot
be proven are marked `partial` instead of being rendered as zero.

Periods span `traffic.period_months`; `traffic.quota_gib=0` is unlimited.
Current periods aggregate across activation bundles while individual samples
retain bundle evidence. Period list responses include the paired
`period_start`/`id` cursor in `next`, so every matching record remains
reachable beyond the requested limit.

Raw samples follow the required `traffic.sample_retention_days` setting, which
is initialized to 90 and accepts 1 through 366 days. A settings file without
the field is rejected. The server removes expired raw samples during startup
and every 24 hours. Traffic period totals remain retained after raw samples
are removed.

The authenticated history endpoint is:

```text
GET /api/v1/metrics/history?from&to&bucket_seconds&activation_bundle_id
```

`from`, `to`, and a positive `bucket_seconds` are required; the bundle filter
is optional. A request may cover at most 90 days and produce at most 512
buckets. Each bucket returns traffic deltas, average and peak memory, average
and peak active connections, sample count, and `complete`, `partial`, or
`missing` coverage. Missing buckets are emitted explicitly with nullable
measurements, never synthetic zero traffic.

Coverage is derived from persisted sampling intervals rather than sample
count. A bucket is `complete` only when accepted same-process, same-bundle,
same-period intervals of at most 30 seconds cover both bucket boundaries and
the whole span between them. An uncovered gap, an over-limit interval, a
process or bundle boundary, rejected counter evidence, or a retained sample
without a provable interval makes the bucket `partial`; no evidence makes it
`missing`. Traffic deltas remain nullable whenever the interval that would
justify them is unknown.

```sh
sing-box-panel metrics show
sing-box-panel metrics watch
sing-box-panel metrics history
sing-box-panel metrics period PERIOD_ID
```

`show` and `watch` include the current traffic period and its cumulative
traffic when evidence is available. Historical periods remain available
through `history` and `period`; a separate `traffic` CLI group is unnecessary.

## Management surfaces

The Web interface exposes sources, manual nodes, keys and channel policy.
OpenAPI also preserves legacy user profiles/grant matrices and source history
for existing clients. Runtime operations, tasks and traffic evidence retain
their authenticated management contracts. The
browser core import uses bounded multipart upload and a private staging
directory; it never asks a browser to submit a server-local path.

See [Configuration and runtime](configuration-and-runtime.md) for activation
semantics and [HTTP API and security](http-api-and-security.md) for management
authentication and request boundaries.
