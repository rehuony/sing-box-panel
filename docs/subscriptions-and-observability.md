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

Channel names are display-only; use the row's Edit button to open a channel's
workspace. Channel forms omit enablement controls: new channels are enabled and
editing preserves the stored state.

The API also supports user-scoped keys, which require an enabled user and grants
naming pool node IDs. An empty grant set renders an empty subscription. User/grant
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

### Subscription traffic metadata

Every successful public subscription response, including `304 Not Modified`,
includes an HTTP response header in the following format:

```http
Subscription-Userinfo: upload=123; download=456; total=0
```

Values are integer bytes, separated by a semicolon and a space. `upload` is the
recorded outbound traffic of the nodes whose local inbounds the subscription
delivers, each in its own current period, and `download` is the same for
inbound traffic. Those are the fleet nodes with an inbound the channel
publishes and the key may receive; a subscription that publishes no inbounds
reports zero. `total` is always `0`: each node has its own quota, and their sum
would tell a client nothing about how much it can still use on the node it
actually uses. The counters belong to the nodes, so every key that reaches the
same nodes sees the same values. They do not represent individual user usage,
subscription file sizes, or the key's download limit.

Each request reads those nodes' period settings and sums the recorded usage of
each node's current period. When a node reaches its reset day, only its new
period's usage counts; no records means zero. Historical totals remain intact,
and no reset job or running collector is required. Period changes take effect
on the next request.

The header and `GET /api/v1/traffic/status` report the same recorded sing-box
usage even when sampling is stale or stopped. Zero means no recorded usage, not proof that no
traffic occurred. VPS interface totals and unrelated host traffic are not included.
Configuration read or traffic storage failures return `503` instead of fabricated counters. Failed requests
do not consume download quota or expose the traffic header; final authorization
is rechecked before either a `200` or `304` response is committed.

Traffic changes leave the subscription body and its ETag unchanged. Conditional
requests still receive refreshed traffic metadata. The response adds no expiry
or refresh-interval field: a period boundary is not a subscription expiration.
Clients display the new usage on their next successful subscription refresh,
so manually refresh an existing subscription to see it immediately.

## Subscription node pool

The pool is the one list of nodes subscriptions publish: every inbound the
fleet's nodes publish and every node a source's filter keeps. It has no manual
nodes, enable switches or hidden flags. A node in the pool can be published, and
each channel's groups choose what that channel delivers. The Web UI shows the
pool as cards in its saved order, without pagination; dragging a card saves the
new order.

Each node has an ID derived by one rule from its origin: `sn_` and a digest of
the origin kind, the origin's ID and the node's key within the origin. An inbound's key is its type and tag with the identity of its credential, so a
multi-user inbound yields one node per user, named `tag (user)`; a source node's key is its name in the
source, with `#2`, `#3` and so on for later nodes of the same name. The ID holds
while the origin keeps that key: renaming a node at its provider, or changing an inbound's type, tag or user,
makes a new node. Order, display name, channel membership
and user grants all refer to the ID.

A node's display name is the name every channel publishes it under and the only
field an administrator edits. It is unique in the pool, ignoring case, has at
most 256 bytes and no surrounding spaces, control characters or commas; a taken
name answers 409. A new
node takes its origin name, prefixed with its fleet node's name for an inbound
(`tokyo · vless-in`), and appends ` 2`, ` 3` and so on while the name is taken.
Its content is the sing-box outbound its origin yields, stored with the node and
replaced on each synchronization.

### Synchronization

The pool is synchronized in one transaction whenever its inputs change: a fleet node applies a configuration (rollback included), changes its public
host or reported public IP, or is deleted; a source is created, refreshed, given
new content, has its filter changed, or is deleted; and once when the panel starts. Synchronization

1. computes every node the inputs yield, with its ID;
2. deletes the nodes whose ID is gone together with everything that names them:
   their group memberships and user grants are removed by cascade, a rule exit
   that named one falls back to its group's default, and a channel's default
   exit that named one falls back to `direct`;
3. replaces the content of the nodes that remain, keeping their display names
   and positions;
4. appends the new nodes to the end of the order in origin order: fleet nodes by
   name, each node's inbounds in configuration order, then sources by creation
   time, each source's nodes in content order.

Nothing about a removed node is kept, so a node that returns later is new: it
joins the end of the order and no channel. A source whose stored content no
longer reads keeps its nodes and records the failure in `last_error`.

`PUT /subscription/node-order` takes every current ID exactly once. A different
set means the pool was synchronized since it was read and answers 409; the UI
then reloads the pool. Otherwise the last saved order wins.

### Fleet inbounds

Each fleet node contributes the inbounds of the configuration of the bundle it
reports running: the panel finds that revision among the node's revisions,
checks its bytes against the digest the node reported, and converts its inbounds
with the exact core version of the bundle. Draft, failed and unapplied
configuration is never published, and a rollback changes the applied bundle and
therefore the inbounds. A node whose revision the panel no longer holds or whose
digest differs, whose core version has no converter, whose configuration cannot
be read, or that has neither a public host nor a reported public IP contributes
no nodes and adds a diagnostic naming it; its earlier nodes leave the pool with
the next synchronization, and the other nodes publish as usual.

The inbound registry accepts only the exact reviewed releases with an inbound
family in the [support table](core-versions.md#exact-configuration-support);
other versions fail closed. Each converter publishes only the client-usable
inbound types available in that release and reports stable diagnostics for
server-only or unsupported types. A node's public host, set in its settings, or
else the public IP it reports combines with each inbound `listen_port`; server
certificate private keys, ACME configuration and listen-side fields are never
copied.

| Inbound family | Convertible local inbound types |
| --- | --- |
| `1.13` | `mixed`, `socks`, `http`, `shadowsocks`, `vmess`, `trojan`, `hysteria`, `shadowtls`, `vless`, `tuic`, `hysteria2`, `anytls`, `naive` |
| `1.14` | All family 1.13 types plus `snell` |

For these versions, `direct`, `tun`, `redirect`, `tproxy` and `cloudflared` are
explicitly unpublishable. Any other inbound type produces an unsupported-type
diagnostic; it is not guessed from a nearby version.

## Subscription sources

A source is either a URL or a file. A URL source is fetched when it is created,
when an administrator refreshes it and, when `refresh_minutes` is not zero, every
that many minutes (at least 15). A file source holds content uploaded or pasted
in the Web UI and changes only when new content replaces it. A source is created only when its first content parses. The panel holds at most
256 sources with 32 MiB of stored content in all, and at most 10,000 pool nodes.

A source keeps only the last content that parsed, overwritten by the next, so no
history accumulates in the database. A failed fetch or parse keeps the current
content and nodes and records its code in `last_error`, which the next success
clears; a scheduled source tries again after its interval. Fetches allow at most
4 MiB, 20 seconds and five redirects. DNS and every redirect target are checked
against the SSRF policy. Only public unicast addresses are allowed: loopback,
private, link-local, shared (`100.64.0.0/10`), benchmarking, documentation,
reserved and multicast ranges, and the IPv6 translation prefixes that embed them,
are denied unless `subscription.private_source_cidrs` explicitly allows them.
Core downloads apply the same address rules. Public subscription requests never
fetch upstream data.

Each source has a filter of name rules. Plain text matches a case-insensitive
substring of a node's name in the source; text wrapped in slashes is an RE2
regular expression, which matches in time linear in its input. A nonempty
include list keeps only matching nodes, and the exclude list then removes
matching ones. Only kept nodes enter the pool. A changed filter applies to the stored content at once, while a changed format
applies to the next fetch or upload; `POST /subscription/sources/preview` shows which nodes a
filter keeps before it is saved.

Sources, channels and node display names use their `updated_at` timestamp as
their version: RFC 3339 with nanoseconds, strictly increasing for each record.
Changing or deleting one sends it quoted in `If-Match`, and a stale value answers
412. A source's timestamp changes only with its settings, so a refresh never
invalidates an edit in progress.

### Parsing

Parsing is strict. The content must parse as a whole, and every node the filter
keeps must convert to a sing-box outbound; otherwise the change fails, names the
node's location, such as `proxies[7]` or `line 12`, and nothing is saved:
the request answers 422 `subscription_source_unreadable` with that code,
location, name and reason. A node
that cannot convert can be excluded by the filter, since filtering reads only
names and protocols. Entries that are not nodes are not part of the content's
node list: sing-box `direct`, `block`, `dns`, `selector`, `urltest`, `tor` and `bridge`
outbounds and endpoints other than WireGuard, and Mihomo groups, rules and
providers.

| Format | Accepted content |
| --- | --- |
| `sing-box` | A configuration's `outbounds` and `endpoints`, an outbound array, or one outbound |
| `mihomo` | A configuration's `proxies`, or a proxy list; `proxy-providers` are not fetched |
| `share-links` | One link per line, plaintext or Base64: `ss`, `socks`/`socks5`, `http`/`https`, `vmess`, `vless`, `trojan`, `hysteria`, `hy2`/`hysteria2`, `tuic` and `anytls` |

With the format `auto`, Base64-wrapped content is decoded first, then each
format scores the content and the highest score reads it; a source can name its
format instead. A share link without a name is named by its sing-box type, server and port,
such as `hysteria2 example.com:443`. Each format is one parser registered with its name, its scoring and its
reading into named nodes, and share links register one converter per scheme, so
a new format or scheme is added without changing the rest of the pipeline.

Channel groups, user grants, token state and the pool take effect immediately.
The public handler reads them and the stored nodes in one consistent SQLite read.

## Channels

A channel publishes exactly the pool nodes its enabled groups name as members,
as the exits of their enabled rules, or as its default exit, in pool order and under their display names; a
node no group names is not part of its output. A new channel starts with no
nodes. Each group lists its candidates in order, pool nodes, built-in `direct`
and `reject` exits and other groups, and the first is the initial selection.
Creating or opening a channel goes directly to the strategy-group
workspace, without a separate channel-level node-selection screen. The editor
lists strategy groups in a left sidebar and edits
the selected group's nodes and exit rules on the right. Each sidebar card has a
settings action, shown on hover or keyboard focus, opening a compact dialog for
name, strategy type and client-side health checks without selecting that group.
Touch devices keep the action visible. Group icons distinguish manual selection,
automatic latency tests and fallback. Apply updates the channel draft; Cancel or closing
it discards those edits. Groups created, configured or selected as the final exit are enabled; there is no separate
group enablement control. Opening the page or cancelling the dialog preserves
previously stored disabled groups. The top-level toolbar
contains back, channel settings, preview and save. Channel settings contain name,
client and template; node names, membership and order are managed
directly through nodes and candidate cards. Candidate cards show
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
returning to Channels opens the channel list. Source settings use the same navigation protection. Adding a node to a
group publishes it in the channel. Card order is the sole source of each manual group's initial selection: the first
candidate is selected, and an empty group rejects traffic. Removing a node also redirects any
rules explicitly referencing that node to their group. The final group must be
cleared before deletion. Existing exits remain unchanged until explicitly edited.
Rule and rule-set actions sit beside the node/rule tabs. Their dialogs omit exit
and enablement fields: new rules start enabled and route through their group.
Editing existing rules, including their priority, preserves their enabled state
and explicit exit.
Beside them, an entries/text switch shows a group's rules as Clash classic
lines, one per rule in priority order: `DOMAIN`, `DOMAIN-SUFFIX`,
`DOMAIN-KEYWORD`, `IP-CIDR` or `IP-CIDR6`, and `RULE-SET,name,link[,format[,behavior]]`,
whose missing format is inferred from the link and whose proxied GitHub link is
kept direct and marked accelerated. Blank lines, lines starting with `#` or
`//`, a YAML list marker and quotes around a line are ignored, so a pasted
Clash rule list converts directly; a trailing `no-resolve`, and a policy name
after a match value, are left out with a note, because the group sets the
exit. A rule-set link may hold commas: the link is everything between the name
and the trailing format and behavior, so every saved rule reads back the same.
Other rule types,
invalid values, formats the client cannot use and repeated rule-set names are
listed by line. A line that matches one of the group's rules from when the text
view opened, by type and value or by rule-set name, keeps that rule's ID, exit
and enabled state; lines take the group's existing indices in line order, and
further lines continue after the channel's largest index. Valid text becomes
the group's rules as it is typed. While a line has a problem, the text stays,
switching back to entries and saving are refused, and leaving asks about the
unsaved text. The server's channel policy validation remains the final check.
New pool nodes join no group until added. Missing or cyclic dependencies cannot
silently become direct traffic: unreachable designated exits become reject actions. Group and native
node names must not collide with generated reserved names.

Groups store their `type` (`select`, `url-test` or `fallback`) and their ordered
`members`: pool nodes, the built-ins `direct` and `reject`, and other groups.
New groups have no candidates. Candidate cards start unselected. The toolbar orders Select all and
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
a selection dialog containing the pool's nodes in pool order, both built-in cards and other strategy groups;
unsupported client options are disabled. Only confirmed additions appear in the
group; cancelling the dialog discards its selection and sorting. Its title, compact
selection actions and search share a header; there is no corner close button.
Clicking a card toggles its selection outline, and selecting any new cards reveals
the same count, dashed divider and clear action before Select all. Already-added
cards remain checked and disabled and are excluded from the new-selection count.
Card presentation is shared with the node pool.
DND-KIT sorts whole cards in both the picker and the candidate list; a short mouse
movement threshold distinguishes dragging from selection, and touch uses a long
press. Space/Enter toggle selection; F2 starts/finishes keyboard sorting, arrow
keys move, and Escape cancels the drag without closing the picker. Filtering only
reorders visible slots. Confirmed additions retain their mixed node/builtin/group order,
and Save changes persists it. Built-ins remain separate
from pool node IDs. Remove selected nodes removes all selected candidate kinds;
Clear selection leaves group membership unchanged. Built-in candidates
are never automatically injected into a nonempty group. An empty manual group still rejects
traffic through rules referencing it; no candidate is inserted.
Mihomo and Loon support all three types. Mihomo supports both built-ins;
Loon supports them in manual groups only; auto groups reject built-in candidates. Current sing-box supports
select/url-test and direct; fallback and reject candidates are rejected during
validation and disabled in the editor. Sing-box rejection remains a route action,
not an obsolete block outbound.

Automatic groups may configure `health_check` with an HTTP(S) URL, an interval
of 60–86400 seconds and URL-test tolerance of 0–65535 milliseconds. Defaults are
`https://www.gstatic.com/generate_204`, 300 seconds and 50 ms. These checks run on
the subscribing client, never on the panel. Members keep their order, and group
members name enabled groups of the same channel. The picker appends other
strategy groups after the ordinary nodes, in sidebar order, and supports the same
selection, search and reordering interactions. Self references, duplicate references
and cycles are rejected; disabled groups cannot be selected. Renaming a group
retains its identity. Deleting or disabling it in the editor removes all incoming
candidate references; the final exit must be changed first if it uses that group. Duplicate or unknown members are rejected.
Generation emits dependencies before their parents without flattening them.
Empty child groups are skipped as candidates; an empty first manual
dependency still makes routes through the parent reject traffic. Manual groups use the first card as their initial exit; no independently saved
group default can reorder it. Fallback picks the
first available candidate, whereas URL-test chooses by latency. An empty
manual initial candidate does not suppress remaining auto-group candidates.
An empty group is omitted and its routes reject traffic instead of becoming
implicit direct connections. Preview and delivery use the same rendering path.

Each exit rule has a channel-wide integer `sort_index`, including negative values
and zero. Omission means zero; the value must be a JavaScript-safe integer. The
editor keeps ordering help in the information tooltip beside the field label. Generation gathers rules
across enabled groups, sorts by index ascending, then by rule name ascending
(remote-set name or ordinary match value, Unicode scalar order). Equal index/name
pairs preserve their original relative order. Duplicate indices are allowed;
new rules start ten after the current maximum (or ten for an empty channel),
capped at the largest safe integer. The final fallback remains last.
Indices are panel metadata and never appear in client configuration. Loon's
native category precedence still applies.
Each rule occupies one line with its name, compact priority/type/exit summary and
edit/delete actions. Overflowing text is truncated with its full value available on hover.
The list has no enable checkbox; stored disabled rules are labelled as disabled
in the list, and editing preserves their state.

A channel's groups, members and rules are stored in their own tables and
replaced together on each save; the serialized configuration is limited to
640 KiB.

Remote rule sets store metadata only. Sing-box uses source JSON or binary SRS;
Mihomo uses YAML, TEXT or MRS with native behavior (MRS excludes classical).
Loon uses the distinct `loon` format containing native rule statements, not
Mihomo domain/IP lists. Loon omits `behavior`. Rule-set update intervals are not stored, accepted by the
channel API or emitted into client configuration. Refresh belongs to the client:
[sing-box defaults to one day](https://sing-box.sagernet.org/configuration/rule-set/#update_interval),
while [Mihomo does not start its periodic provider refresh loop without an interval](https://github.com/MetaCubeX/mihomo/blob/Meta/component/resource/fetcher.go).
Group health-check intervals are independent and remain configurable.
Rule-set names must be unique across the channel, including disabled rules/groups;
comparison is case-sensitive and names must have no surrounding whitespace.
Mihomo provider keys and sing-box rule-set tags use these names verbatim, as do
all generated references. Sing-box remote sets use an inline `http_client` with
`detour: direct`, matching the native 1.14 Schema. Loon uses native URL-based remote rules and emits the configured rule-set name
as `tag` alongside `policy` and `enabled`, escaping quoted values when necessary.
This development contract change has no legacy interval migration or compatibility
reader; requests containing the removed field are rejected.
The editor infers source format from the original URL's case-insensitive path
extension, ignoring query strings and fragments: Mihomo YAML/YML, TXT/LIST or MRS;
sing-box JSON or SRS; and Loon LOON/LIST/TXT. Unknown or incompatible extensions
require manual selection. Manual choices survive reopening the editor and toggling
GitHub acceleration; changing the original link runs detection again. Detection is
only a suggestion and does not inspect the remote content or infer rule behavior.
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
and scalar values are also preserved. Mihomo YAML exports retain Unicode text,
including emoji in node names, strategy groups and rule references, as UTF-8
characters. Literal backslash sequences and required YAML escaping keep their
original meaning.

Defaults are shared by the editor and renderer from the native files in
`internal/sing-box-panel/subscription/templates`. They use ordinary DNS without geographic
splitting, IPv4, local proxy listeners, and no default-enabled TUN or MITM.
Mihomo includes DNS caching and HTTP/TLS/QUIC sniffing without overriding the
request destination; Loon retains local bypass and rejects unsupported UDP.
The defaults follow the [Mihomo configuration reference](https://wiki.metacubex.one/config/),
[sing-box DNS reference](https://sing-box.sagernet.org/configuration/dns/), and
[Loon general configuration reference](https://nsloon.app/docs/General/).
A missing template uses the same complete default base in both preview entry
points and public delivery. Existing templates, including explicitly empty bases (`{}` for JSON/YAML,
empty text for Loon), are never upgraded or replaced automatically. Deleting a
field removes it; invalid content fails validation instead of loading defaults.

Preview diagnostics and request failures appear behind an issue button beside
the preview title. Previews without diagnostics have no issue indicator; the code area
contains only generated configuration. Authenticated preview diagnostics identify
the node by its pool ID, display name and protocol when available, with an
input field path, localized explanation and corrective suggestion. Positions
refer to renderer input, not lines in the generated configuration. Unknown codes
retain their technical code with a generic explanation. Public diagnostics remain
positional and never include node identities or configuration values.

The node pool, sources, keys, channels and their dialogs show one centered empty
or initial-loading state with a contextual icon, title and brief guidance inside
the relevant content area. Loading does not show
an empty-state message simultaneously; refreshes retain available data. Rule-order
explanations do not occupy a separate banner above the editor.

Both preview entry points use the same
renderer; an unapplied template draft can intentionally differ from the saved
channel. After Apply, the toolbar preview uses that same draft.

In the template dialog, Validate and Preview send the current draft without
persisting it. Validate is available again after every request, including
failures; editing is locked only while that request is pending. Apply closes the
dialog and updates the channel draft, including unfinished content. Save changes
is the sole persistence action: it previews the exact proposed policy, then
saves with the channel's `updated_at`. Validation errors and revision conflicts retain
the draft. Cancelling the template dialog leaves the channel draft unchanged.
Switching output client with an existing template explicitly replaces it with
the target client's seed in the draft; cancelling settings preserves the old
format and template. Without a template, switching leaves it absent.

The Web client validates every channel draft with the native renderer before
saving; a failed preview never falls back to an unvalidated save.

New Loon channels support
the same editing workflow as sing-box and Mihomo, even without a saved template.

Sing-box templates and final output are schema-checked against reviewed 1.14.0,
independently of the server's selected core. Mihomo and Loon check native
structure and reserved fields; client runtime validation remains the subscriber's
responsibility. Diagnostics report field paths and fixed error codes without
reflecting submitted secrets.

Authenticated preview can validate an unsaved draft with an empty pool
before any node exists. Public delivery retains its source
availability and authorization checks. Both use the same renderer. JSON preview
responses preserve the existing byte/Base64 contract; the Web adapter decodes
before display/copy.

Node detail JSON displays credentials directly and copies the complete displayed
JSON. A source node's `detour` names another node of the same source by its
name there; a channel publishes it under that node's display name when it
publishes that node too.

## Target renderers

The sing-box renderer preserves validated publishable nodes in sing-box JSON.
The cross-format renderers convert only their explicit current contracts:

| Target | Explicitly converted node types |
| --- | --- |
| Mihomo | `shadowsocks`, `socks`, `http`, `vmess`, `vless`, `trojan`, `hysteria2`, `tuic`, `anytls` |
| Loon | `shadowsocks`, `socks`, `http`, `vmess`, `vless`, `trojan`, `hysteria2`, `anytls` |

Unsupported types, transports, TLS shapes, networks, dependencies, or options
are omitted with stable positional diagnostics. Loon TLS output preserves the
ordered ALPN list using a quoted comma-separated value, including AnyTLS. A single
identifier containing a comma, line break, NUL or surrounding whitespace is
rejected rather than changed during conversion. Renderers never infer a field
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

`/api/v1/nodes/{nodeId}/core/logs/files`, `/api/v1/nodes/{nodeId}/core/logs/content` and
`/api/v1/nodes/{nodeId}/core/logs/stream` expose only the node's managed file names and
bounded byte cursors; the panel relays each read to the node and keeps no
copy, and the stream reads the node again each second. The node's collector
captures child stdout/stderr. If native `log.output` is
set, it follows new bytes from that regular file, handling creation, truncation
and rotation without reading pre-existing contents or modifying native config.
Disabled output is not followed; symbolic-link files and capture-directory loops
are rejected. On child exit, pending output is drained and its final partial
line is flushed before completion is reported.

Relative `log.output` paths resolve from the node's `<data-dir>/runtime`, the child process's
working directory, rather than from the saved configuration's directory. For
example, `box.log` resolves to `<data-dir>/runtime/box.log`. Absolute paths remain
absolute. The native process must be able to write the destination and the panel
must be able to read it. Saving a new path takes effect on the next Start/Restart.
The follower checks for new bytes every 250 ms; it does not import bytes already
present when following begins. The original output file remains under the
operator's management, including its permissions, retention and raw contents.
The panel separately sanitizes and retains its captured copy under
`<data-dir>/logs/core`. Deleting a captured copy never removes the original file.

`DELETE /api/v1/nodes/{nodeId}/core/logs/files?file=<managed-name>` requires management
authentication and the usual CSRF token for cookie sessions. The file list's
`deletable` field is computed against the server's current UTC date. The browser
only shows deletion for eligible files; the server rechecks the date and rejects
all of today's segments with `409 core_log_current`, even after size rotation.
Invalid names and non-regular files are rejected; missing files return 404.
Deletion outcomes are recorded as panel activity.

`DELETE /api/v1/nodes/{nodeId}/core/logs/content?file=<managed-name>` clears the selected
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
retention applies only to sing-box capture files; panel events follow
`logs.panel_retention_days`, described under [Durable logs](#durable-logs).
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

The Web logs pages and authenticated API expose bounded, sanitized metadata for
panel, core, and security events. Use the Web UI to inspect stored events;
`server start` also streams panel events in its terminal and `systemd logs`
reads the service journal.

`GET /api/v1/logs/stream` is a durable server-sent event stream and accepts
`Last-Event-ID` for reconnection. Like the other streams it ends within a
minute with a `control` event of type `reconnect`, so a client reconnects and
authenticates again; `Last-Event-ID` resumes it after the last entry it
received. Each write has its own deadline.

Panel events and each node's runtime history follow
`logs.panel_retention_days` (1–3650 days, 90 by default, and 90 for a settings
file without the field). The retention pass that also removes expired raw
traffic samples deletes older entries during startup, every 24 hours and after
a settings save; it keeps the newest runtime transition of each node instance,
which gives the state a timeline starts in and where the node's next session
resumes its history. Explicit log clear/delete API operations remain
available. Configuration bytes, subscription bodies, token
plaintext, URL credentials, and known secret fields are not stored as log
payloads.

## Dashboard delivery and host metrics

Each node reports its own metrics, traffic and runtime transitions over its
session, and the panel serves them per node under `/api/v1/nodes/{nodeId}`:
`metrics`, `metrics/stream`, `metrics/history`, `dashboard/stream`,
`traffic/periods` and `core/runtime/history`. A node that is offline keeps its
recorded history and traffic; its live views show no current sample.

A node sends a metrics sample every ten seconds. While any client streams a
node's `metrics/stream`, the panel tells that node to sample once a second and
returns it to ten seconds when the last viewer leaves. The stream shares
snapshots across clients on a one-second cadence, combining the node's latest
sample with the runtime state it last reported. The node reads the running core
every second with a reused, incarnation-bound Clash client. The optional
`metrics.live_sample` is the node's latest reading, with its own `sampled_at`,
PID and start token; it is shown only while the node reports that process
running and for 30 seconds after the reading. Its nullable
`upload_bytes_per_second` and `download_bytes_per_second` fields are calculated
by the panel from adjacent samples of the same process and their actual
elapsed time: one-second rates while a node is viewed, ten-second averages
otherwise. A first sample, process change, decrease, or interval over fifteen
seconds has no rate. Existing cumulative fields remain available; the bundled Web client
uses the new rate fields directly. Live samples do not enter the persisted accounting totals.
Collector availability and historical sampling quality remain separate from
recorded period usage. Snapshots use narrow runtime
projections rather than loading complete saved configurations.

Live, history and log streams have independent browser ownership. The live metrics
stream stays connected across browser tab switches and continues collecting
received samples. Hidden tabs pause history, logs, the fleet stream and polling;
those subscriptions resume when visible. Normal rotation sends
`event: control` with `{"type":"reconnect"}` before the one-minute authentication
limit. Older clients may ignore that event. The client reconnects immediately;
unexpected failures use jittered exponential backoff capped at 30 seconds.
Metrics, dashboard and fleet streams send heartbeat comments every ten seconds. A
25-second idle watchdog reconnects half-open responses, and 401 ends the session.
Live data is stale after ten seconds without a snapshot; history after 45 seconds.
Error feedback has a five-second grace period. Last observations remain visible;
a history delay is shown locally with its update time, not as a panel disconnect.
Linux host CPU/memory/disk metrics remain separate from sing-box process samples;
unsupported hosts report unavailable values. The toolbar retains the localized
`0s` uptime default. Missing or expired rates show `—`; measured idle and verified
stopped states show `0 B/s`. Rates expire three seconds after their sample, even
when fresh stream envelopes repeat the same observation; this does not mark
the entire runtime stream stale before its ten-second threshold.

System services read three narrowly scoped read-only host-counter mounts while
retaining procfs isolation; see [systemd packaging](systemd.md#system-service)
for the required unit refresh when upgrading an older installation.

`GET /api/v1/nodes/{nodeId}/dashboard/stream` sends one node's shared projection immediately (or a fresh
cached value) and refreshes every 30 seconds only while subscribers exist.
Assembly runs independently of live collection. A slow client retains just the
latest pending snapshot, and cannot stall collection or grow an event backlog.
The Dashboard owns this subscription; other routes do not query historical data.
Each event includes the node's one-hour / 24-hour histories, runtime transitions,
its two most recent activity records and `persisted_through`. Both metric ranges and that accepted
sample watermark come from one database read transaction. The watermark replaces
overlapping connection-chart tails when history catches up. Live tails are bounded to 120
seconds and break at missing samples, process changes, counter regression
or reconnects. They never enter period totals or interpolate missing evidence.

Each connection is authenticated again within one minute. Collection belongs to
the server lifetime, with pending reads canceled when the final subscriber leaves.
Each write is bounded by ten seconds and the remaining connection lifetime, then
its deadline is cleared after Flush. Initial collection failure returns a Problem
response before opening SSE; later failure terminates only that stream.
Each complete UTF-8 event frame is limited to 1 MiB. Runtime history keeps at
most 4096 newest transitions, or fewer when necessary to fit the frame limit.
Any truncation retains a `runtime_24h.next` cursor at the last included transition;
the omitted older interval is unknown, not inferred from the preceding state.

The dashboard shows host summaries, live transfer rates, transfer history and
one-hour active connections. Traffic defaults to Live: the last two minutes of
received samples, including while the browser tab is hidden, with seconds on the
time axis. Browser suspension or network failures can still leave actual sampling
gaps; missing observations are not filled in.
Historical refreshes never replace these samples with bucket averages. The
one-hour and 24-hour views show only historical one-minute and five-minute
averages, respectively. Live samples remain in memory
only and start accumulating again after a page reload. Graph gaps remain gaps.
The 24-hour runtime strip uses 48 equal
segments and persisted transitions; unknown intervals are not guessed healthy.
Traffic charts choose a shared B/s, KB/s, MB/s, GB/s, TB/s or PB/s unit from the
largest displayed upload/download rate. The heading, axis and details update
together when live data or the selected history changes; the vertical range
rescales with headroom. Historical traffic keeps the full bucket denominator,
including partial buckets. The connection chart can join its final historical
bucket at the persistence watermark to subsequent live observations. Explicit
missing values and live intervals over three seconds break the line; small
points keep individual live traffic observations visible.
Traffic and connection chart details follow the pointer and stay inside the chart,
showing metric values and units without timestamps. Arrow keys inspect samples;
Escape or leaving the chart dismisses the details.
The node dashboard shows the node's recorded period traffic against the node's
own quota; an unlimited quota is rendered as `∞ GiB` while keeping the observed
used-byte value. Unavailable traffic evidence remains unknown rather than being
rendered as zero.

## Limited monitoring and traffic

`process_only` reports child-process health without counters. `limited`
requires the final configuration to enable a
Clash API on a numeric loopback address with a non-empty secret. The node
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
process runs, the node reads `/connections` every second and adds each
process's counter growth to cumulative totals for its database, kept in a
durable node checkpoint saved every ten seconds and at each process change. A
process's first reading is its baseline, so traffic before it is not guessed;
traffic between the last reading and a process exit is not counted. A decrease
within one process adds nothing and is reported. Sampling failure clears live
evidence without changing accounting.

Nodes report those cumulative totals in their metrics samples, so the panel
proves the delta between any two samples of the same node database, including
across core restarts, configuration changes and periods the node was offline.
The panel stores a node's samples at most every ten seconds, and at once when
the core process changes. A recreated node database starts a new segment whose
first totals are a baseline. A decrease or out-of-order sample within one
segment is rejected without changing the checkpoint. Persisted samples older
than 30 seconds make live metrics unavailable; recorded period usage remains
available. The source is the managed sing-box instance's `/connections`
upload/download counters, not host network interfaces, SSH, system updates or
other VPS traffic.

Each accepted delta is booked entirely in the UTC day of its later sample,
including a cross-day interval or a collection gap longer than 30 seconds.
There is no proportional allocation. This keeps recorded increments from being
lost or counted twice, but an interval across midnight can shift usage into
the later day. Changing period settings does not reset checkpoints or re-add
lifetime counters. The raw sample's nullable deltas and coverage metadata remain
available for historical charts, independently of daily accounting.

Each node has its own public host, quota and period, read and saved with
`GET` and `PUT /api/v1/nodes/{nodeId}/settings`. The Web interface edits them
on the node's settings tab, saved together with the node's name:

| Field | Meaning | Default |
| --- | --- | --- |
| `public_host` | IP address or DNS name, without scheme or port, that subscriptions publish for the node's inbounds; loopback and unspecified addresses are rejected | Empty; the public IP the node reports |
| `traffic.quota_gib` | The node's quota in GiB; `null` or `0` is unlimited | `null` |
| `traffic.period_months` | Period length, 1 to 120 months, aligned to months counted from January 1970 | `1` |
| `traffic.reset_day` | Day of the month, 1 to 28, on which each period starts at 00:00 UTC | `1` |
| `tags` | Up to 16 tags of 1 to 32 lowercase letters, digits, `-` and `_`, stored sorted and without duplicates; fleet reads filter by them and fleet commands select by them | `[]` |

A save carries the `revision` it read and fails with `412` when another save
changed the settings first. Periods and quota status are recomputed at once
from the node's daily ledger; saved traffic does not change. The panel keeps
each node's current period and newest accepted sample in memory: every stored
sample updates them from its own transaction, a read of a range the panel does
not hold, after a rollover or a changed reset day or period length, reads the
daily ledger once, and the quota comes from the node's settings at every read. Each node has its
own daily totals, samples, checkpoint and recorded periods, updated in one
transaction; removing a node removes them. A node's current usage, period
details and paginated period list read its durable daily ledger, even after raw
samples expire or a previously selected period receives more recorded usage.
Period IDs name their first and end days, such as `traffic_20261015_20261115`.
The paired `period_start`/`id` cursor in `next` preserves stable pagination.

Every successful metrics response includes `current_traffic_period`: an empty
period has zero inbound and outbound bytes. Recorded usage and quota status remain
visible while sing-box is stopped, monitoring is unavailable or samples are stale.
`available` and `reason_code` describe live collection only. The retired
`traffic_available` and `traffic_coverage` fields are removed; clients should read
`current_traffic_period` directly. The dashboard no longer evaluates period
completeness. Before a successful response, or when storage fails, the existing
loading/error handling applies rather than inventing zero usage.
A node's metrics report its own `quota_bytes`, absent while unlimited, and
`quota_exceeded`, set once the node's recorded upload and download together
reach it, even while collection is stopped. `GET /api/v1/traffic/status` lists
every node's current period, quota and `quota_exceeded`, and sums only usage
into `upload_bytes` and `download_bytes`; quotas are never summed.
`GET /api/v1/fleet/status` returns every node with the metrics snapshot
`GET /api/v1/nodes/{nodeId}/metrics` returns for it, without `latest_sample`
and the period's `counters`, so the nodes home and the status line read the
fleet's load, live rates and period usage in one request. One read of the
fleet is a single query for the nodes; their live evidence and recorded traffic
come from memory, and each node's item is encoded once for every reader. An
item that differs from the node's previous one only in `collected_at` keeps the
previous item, so `collected_at` in a fleet item is when the panel first read
the values it shows. `GET /api/v1/fleet/stream` shares one such snapshot among
every client, read every two seconds only while a client streams it: a
connection's first `fleet` event has `reset` set and every selected node, and
later events carry only the nodes whose item changed in `items` and the IDs of
nodes deleted or no longer selected in `removed`; a read with no change sends
no event. `/fleet/status` answers from the shared snapshot while it is at most
two seconds old, and a panel write that changes what it shows, such as enrolling,
renaming or removing a node, saving its settings or a command's result, makes
the next read a new one. Both accept `state` (`online` or `offline`), `tag` and `q` (part of a node's name, hostname or public IP,
ignoring case); `/fleet/status` also pages with `limit` and the `next_cursor` of
the previous page as `cursor`, and without parameters both return every node.
`/fleet/status` sends an `ETag` digest of its body, so a client holding an
unchanged page gets `304`. The Web client follows the fleet stream while its
page is visible and applies each event to every fleet view, keeping the
panel's order of names ignoring ASCII case, then IDs; a reconnect starts again
from a reset event, and a stream that stays down hands over to one
`/fleet/status` read.

Raw samples follow the required `traffic.sample_retention_days` setting, which
is initialized to 90 and accepts 1 through 366 days. A settings file without
the field is rejected. The server removes expired raw samples during startup
and every 24 hours, and rechecks immediately after a Web settings save, in
small batches so arriving samples are not held up. Daily and traffic period
totals remain retained after raw samples are removed. The same pass keeps each
node's 50 newest configuration revisions and every revision still in use: the
saved text's, the latest that parsed and the one the node reports applied.

A sample carries the node's own time unless that time lies ahead of the
panel's clock or more than 20 seconds behind it; then the panel's time replaces
it, so a node with a wrong clock never records a time later samples cannot
pass. Counters that decrease within one node instance start a new baseline:
that sample proves no traffic and the next ones count from it.

The authenticated history endpoint is:

```text
GET /api/v1/nodes/{nodeId}/metrics/history?from&to&bucket_seconds&activation_bundle_id
```

`from`, `to`, and a positive `bucket_seconds` are required; the bundle filter
is optional. A request may cover at most 90 days and produce at most 512
buckets. Each bucket returns traffic deltas, average and peak memory, average
and peak active connections, sample count, and `complete`, `partial`, or
`missing` coverage. Missing buckets are emitted explicitly with nullable
measurements, never synthetic zero traffic.

Coverage is derived from persisted sampling intervals rather than sample
count. A bucket is `complete` only when accepted intervals of one node database
within one month, each at most 30 seconds, cover both bucket boundaries and the
whole span between them. An uncovered gap, an over-limit interval, a new node
database, rejected counter evidence, or a retained sample without a provable
interval makes the bucket `partial`; no evidence makes it `missing`. With the
bundle filter, an interval counts toward the bundle of its later sample. Traffic deltas remain nullable whenever the interval that would
justify them is unknown.

The Web dashboard and authenticated metrics API expose each node's current
traffic period and its recorded cumulative traffic, including zero before
collection. Stopped or
stale collection does not hide these totals; availability and reason describe
live collection only. Historical periods remain available through the authenticated
traffic API. Metrics and traffic periods have no CLI command group.

## Management surfaces

The Web interface exposes the node pool, sources, channels and keys.
The former subscription, log, and metrics CLI groups have been removed; use the
Web UI for these management tasks or the authenticated HTTP API for automation.
OpenAPI also provides user profiles and their node grants. Runtime operations and traffic evidence retain
their authenticated management contracts. The
browser core import uses bounded multipart upload and a private staging
directory; it never asks a browser to submit a server-local path.

See [Configuration and runtime](configuration-and-runtime.md) for activation
semantics and [HTTP API and security](architecture.md#http-api-and-security) for management
authentication and request boundaries.

### Reverse proxy streaming

Nginx must pass the event stream without response buffering (see the
[official buffering documentation](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_buffering)).
The application emits `X-Accel-Buffering: no`; avoid configurations that ignore
that header. A dedicated API location can also set:

```nginx
location /api/v1/ {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 75s;
}
```

Keep the existing forwarded-host/TLS and authentication configuration. Validate
through the deployed chain using an authenticated SSE client: the initial event
must arrive before closure, heartbeats must arrive about every ten seconds, and
a control event/renewal must occur before one minute. A successful direct local
connection alone does not establish CDN or reverse-proxy buffering behavior.

Management node lists read only the public-address detector cache. The server
refreshes it in the background; expired or missing addresses are unavailable.
Explicit panel/channel addresses retain precedence, and actual public
subscription publication still resolves and validates its endpoint.
