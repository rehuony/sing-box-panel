# Subscriptions and observability

Public subscription output combines immutable applied runtime state with live,
administrator-managed authorization. Metrics are exposed only when a real
collector sample exists.

## Subscription keys and user-scoped grants

The Web UI exposes subscription sources, keys, and channels. New keys do not
require a subscription user. They authorize the nodes allowed by each channel's
publication policy. A key has a label, optional exclusive expiry time, and an
optional download limit (1–1,000,000,000). Authentication uses its SHA-256 digest.
New keys also retain their plaintext in a separate secret table in the protected
SQLite database so an authenticated administrator can explicitly reveal them
through `GET /subscription/tokens/{tokenId}/secret`. This response is `no-store`;
ordinary token metadata, lists, channel configuration and logs never include it.
The database and its backups therefore contain recoverable subscription credentials.
Creation does not choose a channel or generate a subscription URL.
Link export lists all active subscription keys, rechecks channel/key availability,
reads the selected secret and copies the channel URL without requesting a
subscription body or spending quota. Existing global key scope, user grants,
expiry, quota and revocation checks still govern delivery. Channel-to-key export
bindings have been removed; API clients must omit `config.export_token_ids`.
The key list provides View, enable/disable, rotation and deletion. Its metadata
and in-flight list request survive subscription-tab switches, with refreshes after
key changes or pagination. Leaving the subscriptions page releases that state.
Secret dialog contents are cleared on close or when leaving the tab. The channel
list provides Edit, Copy, Link and Delete; Copy creates an independent channel
with the same configuration. Preview, clipboard and public delivery use the same
formatted output: indented JSON, block-style YAML, or native line-oriented Loon.
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
Source names, including the manual collection, and channel names are display-only;
use the row's Edit button to open their workspace. Source deletion is not exposed
in the list or source settings. The creation dialog sizes to its fields, and
subscription actions use visible button surfaces.

Node cards within each source (including the manual collection) support whole-card
DND-KIT sorting with the same mouse, touch and keyboard sensors as channel cards.
Source display order is a browser-local preference keyed by source ID; refreshes
and revisits retain it, new nodes append, and absent nodes are not rendered.
Sorting a filtered or paginated view changes only its visible positions. This
presentation order does not rewrite source data or channel policies. Header actions
do not initiate dragging; dragging a hidden card does not restore its visibility.
Storage failures use an error toast while keeping the current in-memory order.

The API also supports user-scoped keys, which require an enabled user and exact
node grants. An empty grant set renders an empty subscription. User/grant
management is available through the API; the Web key UI uses channel policies.
The authenticated preview accepts an omitted user ID for the channel policy,
or a user ID for that user's restricted output.

The public endpoint remains `GET /sub/{token}/{channelId}`. A key's body-response
counter is shared across all channels. After rendering succeeds, an atomic write
rechecks enablement, revocation, expiry, user status and remaining quota
before committing a 200 response. Concurrent requests cannot exceed the limit.
A 304 response increments the request counter but not downloads; failed
validation, authorization or rendering consumes neither. Accounting describes
server-committed responses: HTTP cannot prove that a remote client received all
bytes after a connection failure. Rotation replaces the secret while retaining
usage, quota and scope; omission of a replacement expiry retains the old expiry.

Disabled users and disabled, revoked, expired, exhausted, deleted or unknown keys
share one public not-found response. Ordinary management reads contain metadata
only; create, rotate and the explicit authenticated secret endpoint return plaintext.
No request address, user agent, plaintext key or response content is recorded in
usage statistics.

## Applied local nodes and versioned sources

Local nodes are derived only from the immutable startup bytes referenced by
the currently applied activation bundle. Draft, failed, and unapplied
configuration is never published. Rollback changes the applied bundle pointer
and therefore restores the matching local-node input without re-projecting the
current revision.

The inbound registry accepts only the exact reviewed releases `1.13.19`, `1.13.20`, `1.13.21`, `1.14.0`, `1.14.1` and `1.14.2`; other versions fail closed. Each converter publishes
only the client-usable inbound types available in that release and reports
stable diagnostics for server-only or unsupported types. Multi-user inbounds
become separate grantable credentials for user-scoped access. The panel public-host override, existing channel `public_host`, or detected
public IP combines with each inbound `listen_port`; server certificate private keys,
ACME configuration, and listen-side fields are never copied.

The current exact inbound contracts are:

| Core | Convertible local inbound types |
| --- | --- |
| `1.13.19`, `1.13.20`, `1.13.21` | `mixed`, `socks`, `http`, `shadowsocks`, `vmess`, `trojan`, `hysteria`, `shadowtls`, `vless`, `tuic`, `hysteria2`, `anytls`, `naive` |
| `1.14.0`, `1.14.1`, `1.14.2` | All 1.13.19 types plus `snell` |

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

Remote refresh returns the fetched version and node count after completion. New
remote sources receive an initial fetch, retried after 15 minutes on failure.
Periodic refresh is otherwise disabled by default; a configured interval is at
least 15 minutes. Its next deadline is persisted per source. Fetches allow at most
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
inbound, channel membership, or user-scoped grants.

Channel configuration accepts a typed policy: selected/excluded publication IDs,
new-node include/exclude policy, organizer options, ordered rule groups and a
final exit. Creating or opening a channel goes directly to the strategy-group
workspace, without a separate channel-level node-selection screen. The editor
lists strategy groups in a left sidebar and edits
the selected group's nodes and exit rules on the right. Each sidebar card has a
settings action, shown on hover or keyboard focus, opening a compact dialog for
name, strategy type and client-side health checks without selecting that group.
Touch devices keep the action visible. Group icons distinguish manual selection,
automatic latency tests and fallback. Done applies the draft; Cancel or closing
it discards those edits. Groups created, configured or selected as the final exit are enabled; there is no separate
group enablement control. Opening the page or cancelling the dialog preserves
previously stored disabled groups. The top-level toolbar
contains back, channel settings, preview and save; node organization is available
inside channel settings. Candidate cards show
the node name and a compact source/protocol summary; addresses remain available
on hover. A flag beside the sidebar's delete action toggles the selected group's
optional final-exit status; its badge shares the counts row without wrapping or
changing card height. Selecting another group replaces the previous
choice, and clearing it restores direct fallback. New groups are not automatically
selected as the final exit. The channel's existing node/reject/group fallback is
preserved until explicitly changed. Group drafts survive
switching groups and are persisted together by Save changes. Leaving an edited
channel through Back, a subscription tab or another panel route prompts to keep
editing or discard the changes. Confirmed departure clears the detail workspace;
returning to Channels opens the channel list. Source settings and node edits use
the same navigation protection. Adding a node to a
group also selects it for the channel. Changing group membership maintains the current group
exit while that node remains a member, otherwise selecting the first remaining
candidate; an empty group rejects traffic. Removing a node also redirects any
rules explicitly referencing that node to their group. The final group must be
cleared before deletion. Existing exits remain unchanged until explicitly edited.
Rule and rule-set actions sit beside the node/rule tabs. Their dialogs omit exit
and enablement fields: confirmed rules are enabled and route through their group.
Existing groups retain their candidate snapshot when new nodes arrive.
Unavailable, hidden or cyclic node dependencies cannot silently become direct
traffic: unavailable designated exits become reject actions. Group and native
node names must not collide with generated reserved names.

Groups explicitly store `type` (`select`, `url-test`, or `fallback`) and
`builtin_nodes` (an array containing `direct` and/or `reject`, or empty).
These fields are required; this development-stage schema deliberately provides
no migration or implicit defaults for older group records. New groups have no
candidates. Candidate cards start unselected. The toolbar orders Select all and
Add nodes; selecting cards reveals one compact container with the selected count,
a vertical dashed divider, Clear selection and the red Remove selected nodes
button, in that order, before those actions.
Card clicks, Select all and Clear selection only change local card selection;
they do not change group membership, exits, rules or the saved draft. Select all
selects visible candidates, preserving selections hidden by search; Clear selection
clears all selections. Only Remove selected nodes removes the selected candidates
from the group, including selections hidden by search, and clears the selection.
Switching groups resets card selection without changing either group's members.
Add opens
a selection dialog containing available catalog nodes and both built-in cards;
unsupported client options are disabled. Only confirmed additions appear in the
group; cancelling the dialog discards its selection and sorting. Its title, compact
selection actions and search share a header; there is no corner close button.
Clicking a card toggles its selection outline, and selecting any new cards reveals
the same count, dashed divider and clear action before Select all. Already-added
cards remain checked and disabled and are excluded from the new-selection count.
Card presentation is shared with subscription sources.
DND-KIT sorts whole cards in both the picker and the candidate list; a short mouse
movement threshold distinguishes dragging from selection, and touch uses a long
press. Space/Enter toggle selection; F2 starts/finishes keyboard sorting, arrow
keys move, and Escape cancels the drag without closing the picker. Filtering only
reorders visible slots. Confirmed additions retain their mixed node/builtin order,
and Save changes persists it. Built-ins remain separate
from publication IDs. Remove selected nodes removes either kind of selected
candidate; Clear selection leaves both kinds in the group. Built-in candidates
are never automatically injected into a nonempty group. An unavailable fixed manual exit still rejects
traffic; the renderer may add a rejection target to enforce that behavior.
Mihomo and Loon support all three types. Mihomo supports both built-ins;
Loon supports them in manual groups only. Loon auto groups require proxy nodes. Current sing-box supports
select/url-test and direct; fallback and reject candidates are rejected during
validation and disabled in the editor. Sing-box rejection remains a route action,
not an obsolete block outbound.

Automatic groups may configure `health_check` with an HTTP(S) URL, an interval
of 60–86400 seconds and URL-test tolerance of 0–65535 milliseconds. Defaults are
`https://www.gstatic.com/generate_204`, 300 seconds and 50 ms. These checks run on
the subscribing client, never on the panel. Auto groups retain candidate order
using optional `candidate_order`, an exact permutation of `node:<publication ID>`
and `builtin:<kind>` references. Missing, duplicate or unknown entries are rejected.
When omitted, publication IDs precede built-ins. Manual initial-exit behavior is
preserved (Mihomo and Loon promote that exit to the first position). Fallback picks the
first available candidate, whereas URL-test chooses by latency. An unavailable
manual initial candidate does not suppress remaining auto-group candidates.
An empty group is omitted and its routes reject traffic instead of becoming
implicit direct connections. Preview and delivery use the same rendering path.

Remote rule sets store metadata only. Sing-box uses source JSON or binary SRS;
Mihomo uses YAML, TEXT or MRS with native behavior (MRS excludes classical).
Loon uses the distinct `loon` format containing native rule statements, not
Mihomo domain/IP lists. Loon manages remote refresh in the client; omit
`update_interval` and `behavior`. Both other clients still require an interval.
Switching clients retains rules and flags incompatible formats for explicit
source review. No format conversion is implied. Loon evaluates local rules
before remote rules, domain matches before IP matches, and FINAL last; ordering
within each category is retained.
The client fetches the final URL; the panel never fetches, counts, uploads or
converts rule content. GitHub acceleration unwraps known proxies and prefixes
eligible original URLs with `https://gh-proxy.com/`, without a GitHub credential.
The Link field contains a borderless acceleration icon. Clicking it toggles
supported links; empty or unsupported links receive a Toast explanation instead
of leaving the action disabled without feedback.

Per-channel templates contain one complete native base configuration: JSON for
sing-box, YAML for Mihomo, or Loon's `[General]`, `[Host]` and `[MITM]` sections.
There is no separate override layer or deep merge with defaults. The panel owns
nodes, groups, rules, providers and final exits; templates containing these
reserved fields are rejected. Sing-box's other `route` options are preserved.
Loon rejects other sections, duplicate sections/keys and malformed lines while
preserving original comments and values. JSON numeric values and YAML comments
and scalar values are also preserved.

Defaults are editor seeds only, drawn from the native files in
`web/src/constants/channel-templates`. They use ordinary DNS without geographic
splitting, IPv4, local proxy listeners, and no default-enabled TUN or MITM.
Mihomo includes DNS caching and HTTP/TLS/QUIC sniffing without overriding the
request destination; Loon retains local bypass and rejects unsupported UDP.
The defaults follow the [Mihomo configuration reference](https://wiki.metacubex.one/config/),
[sing-box DNS reference](https://sing-box.sagernet.org/configuration/dns/), and
[Loon general configuration reference](https://nsloon.app/docs/General/).
A missing template uses an empty base for delivery until the seed is applied and
saved. Existing templates, including explicitly empty bases (`{}` for JSON/YAML,
empty text for Loon), are never upgraded or replaced automatically. Deleting a
field removes it; invalid content fails validation instead of loading defaults.

In the template dialog, Validate and Preview send the current draft without
persisting it. Validate is available again after every request, including
failures; editing is locked only while that request is pending. Apply closes the
dialog and updates the channel draft, including unfinished content. Save changes
is the sole persistence action: it previews the exact proposed policy, then
saves with the channel revision. Validation errors and revision conflicts retain
the draft. Cancelling the template dialog leaves the channel draft unchanged.
Switching output client with an existing template explicitly replaces it with
the target client's seed in the draft; cancelling settings preserves the old
format and template. Without a template, switching leaves it absent.

The standalone Demo client saves channel drafts in memory without native
validation. Explicit Validate and Preview requests for policies still report
that a connected panel server is required. Connected clients always validate
before saving; a failed preview never falls back to an unvalidated save.

Existing channels without `policy` retain their original node-only output on
metadata-only edits. Saving actual strategy/template changes or an intentional
client switch activates full configuration generation. New Loon channels support
the same editing workflow as sing-box and Mihomo, even without a saved template.

Sing-box templates and final output are schema-checked against reviewed 1.14.0,
independently of the server's selected core. Mihomo and Loon check native
structure and reserved fields; client runtime validation remains the subscriber's
responsibility. Diagnostics report field paths and fixed error codes without
reflecting submitted secrets.

Authenticated preview can validate an unsaved draft with an empty node catalog
before the first publication source exists. Public delivery retains its source
availability and authorization checks. Both use the same renderer. JSON preview
responses preserve the existing byte/Base64 contract; the Web adapter decodes
before display/copy.

The native single-node editor uses reviewed 1.14 fields, with basic address and
credentials first and optional protocol, TLS, transport, multiplexing and dial
sections. Explicit protocol/mode changes clear incompatible known options;
unknown extension fields and large numeric lexemes survive unrelated edits.
The advanced node JSON editor formats valid content on load, on entry and on
blur and save, and provides a format button. Invalid or incomplete text is left intact;
formatting preserves large numeric lexemes and unknown fields.
Node detail JSON displays credentials directly and copies the complete displayed
JSON. Configuration forms likewise show passwords, UUIDs, keys and tokens as text.
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
the right. The toolbar also shows connection state and controls for live updates,
scrolling to the bottom, clearing saved output, and deleting a historical
file. Pausing freezes the selected file and disconnects the browser stream;
collection on the server continues. Resuming uses the last received byte cursor,
or switches to the latest file after rotation. Scrolling up suspends automatic
scrolling; the bottom button restores it without changing live-update state.
Clearing requires confirmation and permanently truncates all saved contents of
the selected file, including records hidden by search or level filters. It works
for today's active file as well as historical files, preserving the file and
continued collection. Other dates and rotated segments remain unchanged.
The browser cancels pending reads before clearing and resets its buffer and byte
cursor to zero after success, preserving filters and paused/live state. Refreshing
or reselecting the file cannot restore cleared contents. Failures are shown without
clearing the display. Readers send the file generation with their cursor, so a
clear in another browser resets the display on the next chunk or after resuming.
Historical deletion requires confirmation, removes only the
selected managed file, and returns the view to the latest remaining file.
**Panel logs** combines sanitized operation outcomes with panel, security and
runtime events. The table shows time, message, log level and source, without an
actions column or operation detail dialogs. Rows use their API log level for both
presentation and filtering. Retry an unsuccessful action from its original control;
old payloads and temporary uploads are never replayed.

`/api/v1/core/logs/files`, `/api/v1/core/logs/content` and
`/api/v1/core/logs/stream` expose only managed file names and bounded byte
cursors. The collector captures child stdout/stderr. If native `log.output` is
set, it follows new bytes from that regular file, handling creation, truncation
and rotation without reading pre-existing contents or modifying native config.
Disabled output is not followed; symbolic-link files and capture-directory loops
are rejected. On child exit, pending output is drained and its final partial
line is flushed before completion is reported.

Relative `log.output` paths resolve from `<data-dir>/runtime`, the child process's
working directory, rather than from the saved configuration's directory. For
example, `box.log` resolves to `<data-dir>/runtime/box.log`. Absolute paths remain
absolute. The native process must be able to write the destination and the panel
must be able to read it. Saving a new path takes effect on the next Start/Restart.
The follower checks for new bytes every 250 ms; it does not import bytes already
present when following begins. The original output file remains under the
operator's management, including its permissions, retention and raw contents.
The panel separately sanitizes and retains its captured copy under
`<data-dir>/logs/core`. Deleting a captured copy never removes the original file.

`DELETE /api/v1/core/logs/files?file=<managed-name>` requires management
authentication and the usual CSRF token for cookie sessions. The file list's
`deletable` field is computed against the server's current UTC date. The browser
only shows deletion for eligible files; the server rechecks the date and rejects
all of today's segments with `409 core_log_current`, even after size rotation.
Invalid names and non-regular files are rejected; missing files return 404.
Deletion outcomes are recorded as panel activity.

`DELETE /api/v1/core/logs/content?file=<managed-name>` clears the selected
capture's contents with the same authentication, CSRF and file validation as
deletion, but permits today's files. It truncates in place so append writers
continue normally. Clients must discard pending reads and streams before this
request and restart from offset zero after success; pre-clear byte cursors are
invalid. New output may already exist when the response arrives. Clearing
outcomes are recorded as panel activity; database events and native output are
not cleared by this endpoint.

Read and stream chunks include an opaque `generation`; send it alongside
`next_offset` in subsequent requests. A clear changes that file's generation,
and a server restart changes all generations. When a generation differs, the
server restarts at byte zero and sets `reset: true`; replace the old displayed
buffer before consuming this chunk. This also handles rapid regrowth past an
old offset. Clients that omit `generation` retain offset-only behavior and are
responsible for discarding pre-clear cursors. Capture operations share the
server-owned file manager so reading a chunk cannot overlap truncation.

Private core logs default to seven UTC dates including today, 32 MiB per file,
and no file-count limit. Panel settings → Log management controls
`logs.core_retention_days` (1–3650), `logs.core_max_files` (0–1024, zero means
unlimited), and `logs.core_max_file_size_mib` (1–1024). A count cap can shorten
the effective retention window. Old settings without these fields use the defaults.
Saving installs the policy immediately and runs cleanup. At panel service startup,
UTC midnight, and settings changes, background maintenance ensures the current
UTC date has a capture file, even when the core is stopped or emits no output,
then runs cleanup. A missing day's file starts at `000` with zero bytes; an
existing day's newest segment is reused without truncation or size rotation.
Empty files follow the same retention and count limits as other captures.
Maintenance failures record `core.log.maintenance_failed` and retry within a
minute. Days when the panel service was offline are not backfilled.
Shrinking the size limit rotates before the next write without
truncating existing content. Files rotate by UTC date and size. Rotation switches only the destination file; the process output pipes and
file follower remain connected. Each day's sequence advances from its newest
retained file, including after a panel restart, and extends beyond three digits
when necessary. The active file is retained while within the date window. Lines and read
chunks are bounded; ANSI sequences and known credentials
are sanitized. The browser resumes from the received cursor, freezes the selected
file while paused and polls for rotation. Streaming uses write deadlines and
closes within a minute to reauthenticate on reconnect. `/api/v1/logs/panel`
provides the combined panel view. The legacy log API below remains available.
Each initial file read loads the last 64 KiB; the browser retains at most 2,000
lines and searches/filters only this buffer. File lists refresh every 10 seconds
without overlapping requests and refresh when the page becomes visible again;
the output stream checks for appended content every second. Core capture
retention applies only to sing-box capture files. Panel events are retained indefinitely.
The neutral status dot and icon buttons float over the output in the same softly
bordered, translucent toolbar used by the Advanced JSON editor. Status tooltips
and the pause/resume icon identify the current state without changing the toolbar's
color. Pause freezes updates, bottom resumes scroll following, and clear truncates
the selected persisted capture after confirmation. Historical deletion also
requires confirmation.
The floating toolbar is hidden whenever the output area is empty, including
search and level filters with no matches. It returns when output is visible.
Clearing while paused preserves that state and offers Resume live output in the
empty message so streaming can continue without the floating controls.

Panel-log and subscription-key lists support `offset` with `limit` for numbered
pages and return `total` before pagination. Panel-log totals include the active
level, search, and time filters. Counts and rows come from one database snapshot;
live inserts or deletions may shift rows between requests. Existing `before_time`
and `before_id` cursors remain supported and cannot be combined with `offset`.
The Web footer supports direct page entry and 5, 10, or 50 items per page.

### Panel log details and search

Panel logs use five centered, single-line columns: occurrence time, level, event
summary, event source, and View details. Known events have Chinese and English
names; the original message remains available in the collapsible JSON record. Action
completion and observed runtime state changes remain separate records. The list
shows only the event name; process IDs and other context are available in details.
Long names are truncated in the list and readable in full in details.
The modal contains the event identifiers, precise local time and timezone,
recorded context and collapsible JSON. Basic information and
context appear side by side on desktop and stack on narrow screens. Copy log copies the
complete sanitized API record. Refreshing the list does not replace the record
being inspected.

`GET /api/v1/logs/panel` accepts `search_codes`, a comma-separated list of up to
128 exact codes (each at most 128 ASCII characters matching `[a-z][a-z0-9_.-]*`).
The complete encoded query may be up to 64 KiB, including percent-escaped codes
and other filters; other endpoints retain their own query limits.
These alternatives are ORed with the case-insensitive original message/code
`search`, then intersected with level and time filters. Totals, cursors and offsets
use the same match conditions. The Web client finds code alternatives from both
Chinese and English event names, regardless of the current interface language.
Omitting `search_codes` preserves the original text-search behavior.

Runtime-transition metadata includes only recorded public evidence: `pid`,
`process_started_at`, `generation`, `activation_bundle_id`, and `uncertain_since`
when available. Internal process identity tokens are excluded. Runtime controls
and core-log clear/delete actions also record bounded target identifiers and
`duration_ms`. Failure events include `error_code` and a safe description from
known error categories; arbitrary wrapped errors and request bodies are not
persisted. Core configuration checks, health checks, version mismatches and
termination failures have distinct error codes and localized descriptions.
Start/restart operation logs retain the activation bundle selected during the
check phase, including when execution subsequently fails. All operation context
still passes through the durable log sanitizer.
Historical events without additional context remain readable and explicitly show
that no extra context was recorded; current runtime state is not used to fill
historical gaps. This is an additive API change with no database migration.

## Durable logs

The log CLI and authenticated API expose bounded, sanitized metadata for
panel, core, and security events:

```sh
sing-box-panel log list
sing-box-panel log show LOG_ID
sing-box-panel log tail --follow
sing-box-panel log clear --before TIMESTAMP
sing-box-panel log delete LOG_ID
```

`GET /api/v1/logs/stream` is a durable server-sent event stream and accepts
`Last-Event-ID` for reconnection. Panel events and runtime history have no automatic
expiration, including on startup, settings changes, or backup restore. The unused
`logs.retention_days` file field and `log_retention_days` API field have been
removed and are rejected. Explicit log clear/delete commands
remain available. Configuration bytes, subscription bodies, token
plaintext, URL credentials, and known secret fields are not stored as log
payloads.

## Dashboard delivery and host metrics

`GET /api/v1/metrics/stream` pushes runtime and metric snapshots every two seconds,
with bounded writes and a one-minute authenticated reconnect. The application
shell reconnects with bounded backoff after interruptions and retains the last
valid event; it does not start a parallel polling loop. Repeated collector
timestamps do not replace the last valid transfer rate with zero. Linux host
CPU/memory/disk metrics are separate from sing-box process samples; unsupported
hosts report unavailable values.
The top toolbar keeps uptime and transfer rates visible with their units when
values are missing: `0s` (localized) and `0 B/s`, including compact layouts.
These are display defaults; the runtime badge still reflects the observed state,
and missing monitoring evidence remains unavailable in the underlying data.
The metrics stream reports an initial collection failure as a Problem response,
rather than an empty successful stream. Its reconnect deadline also bounds
collection, and each write deadline is cleared after flushing. The browser
reports streams that close before the first snapshot as errors.
System services read three narrowly scoped read-only host-counter mounts while
retaining procfs isolation; see [systemd packaging](../../systemd/README.md#system-service)
for the required unit refresh when upgrading an older installation.

`GET /api/v1/dashboard/stream` sends an authenticated dashboard snapshot
immediately, updates it every 30 seconds, and closes after one minute so the
next connection revalidates authentication. Each `dashboard` event contains its
collection time, one-hour traffic and connection history, 24-hour traffic and
runtime history, and the latest two panel activity records. The application
shell owns this stream across route changes, keeps the last valid snapshot while
reconnecting with bounded backoff, and does not fall back to periodic history,
runtime, traffic, or log requests.

Normal dashboard stream closure allows five seconds for reconnection before
showing a reconnect status; transport errors are reported immediately. The status
appears inside the shared top toolbar and clears when the next snapshot arrives,
without moving the dashboard cards. On narrow screens it temporarily occupies
the toolbar's metrics area. Hover, focus or tap the status to read its explanation.

The one-minute lifetime includes snapshot collection, and cancels in-flight
queries when it expires. Each write has a deadline of at most ten seconds, which
is cleared after flushing so idle connections can close normally. An initial
snapshot failure returns a non-success Problem response before opening SSE.
Each complete UTF-8 event frame is limited to 1 MiB. Runtime history keeps at
most 4096 newest transitions, or fewer when necessary to fit the frame limit.
Any truncation retains a `runtime_24h.next` cursor at the last included transition;
the omitted older interval is unknown, not inferred from the preceding state.

The dashboard shows host summaries, transfer history and one-hour active
connections. Graph gaps remain gaps. The 24-hour runtime strip uses 48 equal
segments and persisted transitions; unknown intervals are not guessed healthy.
Traffic and connection chart details follow the pointer and stay inside the chart,
showing metric values and units without timestamps. Arrow keys inspect samples;
Escape or leaving the chart dismisses the details.
An absent or zero traffic quota is rendered as unlimited (`∞ GiB`) while keeping
the observed used-byte value; unavailable traffic evidence remains unknown rather
than being rendered as zero.

The demo uses the saved panel traffic quota for both metrics responses and the
live metrics stream. Saving a new quota refreshes shared metrics once immediately; subsequent
updates use the existing stream, including the usage percentage; an empty or zero quota remains unlimited.

## Limited monitoring and traffic

`process_only` reports child-process health without counters. `limited`
requires the final configuration to enable a
Clash API on a numeric loopback address with a non-empty secret. The panel
reads that configuration and never modifies startup bytes.

`reason_code: process_only` and `monitoring_tier: process_only` explain missing
core counters even while the child is running. This does not disable host CPU,
memory, load, or disk collection. The dashboard retains unknown values and empty
chart states without an additional monitoring explanation or configuration link.
To enable core sampling, merge a Clash API section into the existing core
configuration, using an unused loopback port and a private randomly generated
secret, then save and apply it (or start the stopped core):

```json
{
  "experimental": {
    "clash_api": {
      "external_controller": "127.0.0.1:9090",
      "secret": "REPLACE_WITH_A_PRIVATE_RANDOM_SECRET"
    }
  }
}
```

Preserve other configuration fields. Saving alone does not change the running
configuration. The normal configuration apply/start flow derives `limited` from
the validated saved configuration. Allow two sampling intervals for a proven
traffic delta; earlier missing history is not backfilled with invented values.

Apply, start, and restart first pass process health, then wait up to five
seconds for `/version` to report the exact selected core version. While the
process runs, the panel reads `/connections` every ten seconds and persists
memory, active connections, `uploadTotal`, and `downloadTotal`. A sample older
than 30 seconds is stale.

Counters are checkpointed by PID and OS start token. A restart opens a new
segment and preserves the UTC natural-month period total. A decrease inside
one process is stored as rejected diagnostic evidence and cannot lower totals.
Checkpoints retain the last process counters across settings changes and add
only deltas proven inside a UTC month, so changing a period cannot re-add lifetime
counters. Cross-month intervals are not proportionally guessed. Samples and period contributions use
nullable upload/download deltas: legacy or interrupted intervals that cannot
be proven are marked `partial` instead of being rendered as zero.

Periods span `traffic.period_months`, aligned to UTC natural months from January
1970, and are recomputed immediately after saving a new month count. Durable
monthly totals are updated in the same transaction as samples and checkpoints,
so changing periods does not depend on retained raw samples. Version-11 migration
prefers existing single-month totals and backfills only missing months from
verifiable samples. Unsplit older multi-month records remain available as history;
no proportional allocation or duplicate accumulation is performed. Coverage is
`missing`, `partial`, or `complete`; the dashboard marks partial history as
“Incomplete data”. Missing or stale evidence displays unknown usage while still
showing the configured quota. `traffic.quota_gib=null` or `0` is unlimited.
Current periods aggregate across activation bundles while individual samples
retain bundle evidence. Period list responses include the paired
`period_start`/`id` cursor in `next`, so every matching record remains
reachable beyond the requested limit.

Raw samples follow the required `traffic.sample_retention_days` setting, which
is initialized to 90 and accepts 1 through 366 days. A settings file without
the field is rejected. The server removes expired raw samples during startup
and every 24 hours, and rechecks immediately after a Web settings save. Monthly
and traffic period totals remain retained after raw samples
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
OpenAPI also provides user profiles/grant matrices and source history
for existing clients. Runtime operations and traffic evidence retain
their authenticated management contracts. The
browser core import uses bounded multipart upload and a private staging
directory; it never asks a browser to submit a server-local path.

See [Configuration and runtime](configuration-and-runtime.md) for activation
semantics and [HTTP API and security](../development/architecture.md#http-api-and-security) for management
authentication and request boundaries.
