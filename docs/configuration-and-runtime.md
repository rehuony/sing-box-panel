# Configuration and runtime

sing-box-panel keeps one editable `config.json` for each node and maintains
immutable valid JSON revisions as internal runtime evidence. The panel owns the
file; the node receives exact revision bytes only to check or run them. Selecting another
core never creates another configuration branch, and saving configuration
never changes the live process implicitly.

## State model

The control plane separates four layers:

1. The **configuration file** stores exact UTF-8 text, including unfinished JSON.
   Authenticated `GET/PUT /api/v1/nodes/{nodeId}/config/file` uses a numeric compare-and-swap
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

The document is the executable sing-box configuration. Available top-level keys
depend on the selected core's release line. The 1.14 contract includes `log`, `dns`,
`ntp`, `certificate`, `endpoints`, `inbounds`, `outbounds`, `route`, `services`,
`certificate_providers`, `http_clients`, `network_namespaces`, and `experimental`. The panel does not add
`_panel`, disabled-item markers, or another storage envelope to these bytes.

Adding a node saves `{}` and its immutable JSON revision as the node's first
configuration. Initialization does not select or start a core; enabling still
validates the configuration against the selected binary on the node.

The Web editor manages this document through `GET/PUT /api/v1/nodes/{nodeId}/config/file`.
The numeric file revision is a compare-and-swap guard: `0` means no save yet,
and adding a node advances its file to `1`.
Reads return the stored text exactly, including unfinished
JSON, together with `revision`, `syntax_valid`, and `canonical_revision_id`.
A stale save fails instead of merging implicitly. The logical name `config.json`
is not a filesystem path: the text lives in the `node_configurations` table of
the panel's `panel.db`, and the file stays editable while the node is offline. There is no second writable runtime configuration
on disk and no sing-box configuration CLI.

Valid saves synchronize the editable document and immutable revision in one
transaction. Saved invalid text blocks validation, Enable, Start, and Restart until corrected
in the Web editor. Revision values protect concurrent edits; neither the CLI nor
the browser offers historical selection, comparison, or restoration. Internal
immutable evidence remains for native checks, runtime identities, and recovery.
The CLI's `config` group instead manages the panel settings file; see
[Panel settings](cli.md#panel-settings).

### Disposable execution files

Saving updates SQLite; it does not write a separate sing-box JSON file. Checking
or running a configuration materializes its exact snapshot bytes on the node as
`runtime/configs/<config-sha256>.json` in the node's data directory, with private directory permissions and
file mode `0600`. Identical bytes share the same file while in use. The hash is
the configuration digest, independently of the installed core archive digest.

A check releases its file when it finishes, including rejection or cancellation.
A running child retains its file until its process wait completes, even if a
stop request times out. When the last user releases a file, the node removes
it. Normally only the running configuration remains; checking a different
configuration can temporarily add another file, and a completed stop leaves
none. Restart, rollback and recovery reconstruct files from immutable SQLite
startup records as needed. Cleanup does not remove the saved text or history.

After successful startup reconciliation, while holding the runtime ownership
lock, the node also removes unused hash-named snapshots and abandoned writing
temporary files. It does not clean when a prior core may still be alive. Symlinks,
subdirectories and unrelated filenames are left untouched. Cleanup failures
produce `runtime.config_cleanup_failed` warnings in the node's log and are retried on later
cleanup passes; they do not turn successful checks or starts into failures.

Ordinary CI and signed releases exercise this same file API across a real
self-update and restart. The release fixture includes whitespace and a number
beyond JavaScript's safe integer range; the scenario also retains unfinished
JSON through the update before correcting it. The saved text and runtime snapshot identity survive the restart unchanged.
Store/API integration tests separately verify the immutable snapshot bytes and digest. See [Release process](release.md#verification-scope) for the native checks.

## Switching versions without rewriting JSON

Selecting a different core carries the current JSON forward unchanged. The
panel does not infer which keys an older release accepts and does not silently
drop fields. A check snapshots the current valid file and asks that exact,
platform- and version-checked binary to validate the execution snapshot. Snapshot formatting
may differ from the saved text, but field names and values are preserved and
the saved text is untouched. Select the target core and use Check in the Web UI.

For versions with a committed Schema, compilation and fresh runtime candidates
first pass Schema validation. A Schema rejection stops before creating a startup
artifact. The selected binary then checks the snapshot; a native check failure
marks the startup artifact failed. Both failures leave the currently applied
runtime untouched. There is no separate editable startup JSON or
version-specific transformation path. The Web UI requests the check
through `POST /api/v1/nodes/{nodeId}/config/compile`.

## Configuration schemas

The authenticated endpoint
`GET /api/v1/nodes/{nodeId}/core/artifacts/{artifactId}/configuration-schema` returns the
Draft 2020-12 JSON Schema of the artifact's [release line](core-versions.md#release-lines):
the panel's reviewed schema for 1.13, and for 1.14 and later the schema the core
installed on the node prints. The response contains only `exact_version`,
`schema_sha256`, and the schema. Its ETag binds the exact version and digest and
supports `If-None-Match`. A version without a line answers
`409 configuration_schema_unavailable`; for a later-line core on a node that is
offline or runs an older protocol, the node's problem (`node_offline`,
`node_upgrade_required`).

The Web configuration selector lists compatible installed versions,
defaults to the enabled core when available and otherwise uses the highest
installed semantic version. With no installed core, the Web UI keeps Advanced JSON editing and
saving available, while the visual editor directs the operator to version
management. Native validation and runtime startup always require an installed
matching core. A version without a line keeps the visual editor tab disabled with
a tooltip; any other failure to load the schema shows a notice with a retry. A
missing schema disables only structured controls, never JSON save,
compile, check, Apply, Start, Restart, or Rollback.

The selected configuration version and current unsaved document live in one
authenticated browser-session store. Switching between visual modules or
Advanced JSON retains the same draft. Navigation away from configuration and
browser unload remain protected by the unsaved-change confirmation; refreshing
or signing out starts a new selection session.

The served schema is the core's canonicalized output or the reviewed source
definition, with the panel's optional `x-panel` presentation metadata such as
section, order, widget, sensitivity, and bilingual labels. The metadata cannot add
`_panel`, introduce a disabled-item union, fill cross-version fields, or change a
sing-box constraint. References remain local to that one schema document. The
browser renders the served schema and does not validate against it: the panel
checks a configuration against the schema before sending it to a node, and the
core's `sing-box check` is authoritative. The production UI ships no schema and
does not relax the Content Security Policy with `unsafe-eval`.

Structured controls edit only fields the schema declares while preserving
unshown JSON properties. The Advanced editor owns
the complete effective configuration, with JSON syntax highlighting, line numbers,
folding, search/replace, and lossless formatting. The top-right **Format and order
fields** button (also available with Ctrl/Cmd+Shift+F) indents JSON and recursively
applies the shared field order described below. Formatting preserves numeric
literals and remains undoable in one step; incomplete input is left intact.
The editor loads on demand. The browser can save invalid JSON; it disables visual editing and binary
validation until the text is a valid object. Saving and checking lock editing
until the result arrives, so feedback describes the submitted file. Validation
success is a Toast shown only after the binary check succeeds. Unknown fields and
numeric lexemes are retained through visual edits.

Configuration credentials remain visible as plain text, matching the authenticated
JSON editor. A dice button beside supported password, UUID and pre-shared-key
inputs generates a value using the browser's cryptographic random source. Ordinary
passwords use 24 URL-safe ASCII characters; UUID fields use UUID v4. The
[Shadowsocks 2022 methods](https://sing-box.sagernet.org/configuration/inbound/shadowsocks/#method)
use Base64-encoded keys of 16 bytes for AES-128 or 32 bytes for AES-256 and
ChaCha20, including nested users and relay destinations. Other supported encrypted
Shadowsocks methods use ordinary passwords; `none` and unselected methods have no
generator. WireGuard pre-shared keys use 32 random bytes in Base64. Snell PSKs and
user keys use the ordinary password format, which also fits the
[Snell v6 PSK's 12–255 byte range](https://sing-box.sagernet.org/configuration/inbound/snell/#version).
Generation changes only the selected field and remains part of the current draft;
nested dialog changes still require confirmation. Changing a protocol or encryption
method never silently replaces an existing credential. Certificates, asymmetric
keys and externally issued access tokens do not receive generic random generators.

### Field names and inline help

The Simplified Chinese visual editor uses reviewed Chinese names and descriptions
for the fields of the 1.13 and 1.14 schemas. An information button
beside scalar, list and optional-object labels shows a concise description after
a brief hover or immediately on click/tap. Help icons are excluded from Tab
navigation and do not display a focus border or ring.
Help uses the existing Tooltip component with click-to-dismiss disabled, so
clicking before the hover delay opens the explanation instead of cancelling it.
Moving into the tooltip keeps it readable; leaving it and the trigger, losing
focus, Escape or clicking outside dismisses it. Opening another tooltip replaces
the previous one. The tooltip fits the viewport and long descriptions can scroll.
Descriptions include input formats, units and relevant restrictions. Protocol
identifiers and enum values retain their original spelling. Map keys entered by
the operator are not translated. English continues to use the existing labels
and any descriptions supplied by the schema.

The copy is maintained in
[`configuration-fields.ts`](../web/apps/panel/src/i18n/locales/zh-CN/configuration-fields.ts),
with context-specific meanings in
[`configuration-field-help.ts`](../web/apps/panel/src/pages/configuration-page/configuration-field-help.ts).
For example, `final` selects a DNS server in DNS settings and an outbound in
route settings; `method` describes encryption for Shadowsocks and rejection
behavior for a reject action. Presentation annotations are added only to a
browser-side schema copy. The served schema, its digest, JSON property names and
stored values remain unchanged.

The terminology and summaries were checked against the official configuration
documentation at tags
[`v1.13.21`](https://github.com/SagerNet/sing-box/tree/v1.13.21/docs/configuration)
and [`v1.14.1`](https://github.com/SagerNet/sing-box/tree/v1.14.1/docs/configuration),
including [dial fields](https://sing-box.sagernet.org/zh/configuration/shared/dial/),
[TLS](https://sing-box.sagernet.org/zh/configuration/shared/tls/),
[DNS](https://sing-box.sagernet.org/zh/configuration/dns/),
and [route actions](https://sing-box.sagernet.org/zh/configuration/route/rule_action/).
This is authored UI copy, not part of a schema. Fields a later core adds keep
their original labels until this catalog covers them, rather than receiving
guessed explanations; review new field names and changed meanings against that
release's documentation before adding them. The field-help test checks coverage
against the 1.13 and 1.14 test schemas.

### Selecting server paths

Filesystem fields in configuration and subscription-node forms share a Browse
control while remaining editable.
The dialog browses the selected node's filesystem namespace (inside its
container or service sandbox when applicable), subject to the node process's
OS permissions. The panel's own data directory is a plain path field, because
the panel host has no browse API. It provides parent and breadcrumb navigation,
direct path entry, name filtering, hidden-file visibility and pagination.

A node's packaged system service runs as `sing-box-node`, even when installed or
started by root. Its default `CAP_DAC_READ_SEARCH` permits reading root-owned
certificates and traversing their directories without changing file ownership
or modes, including symlink targets. The capability covers the service's visible
filesystem; service sandbox restrictions still apply. Check the effective
service identity, capabilities and drop-ins; see
[systemd permissions](systemd.md#system-service).

Click an entry to select it, then use the toolbar's Confirm button to validate and
use that path. Double-click a directory, or press Enter / Right Arrow on its row,
to open it; Left Arrow returns to its parent. Keyboard navigation restores focus
after loading, and parent/breadcrumb navigation remains available after errors.
In directory mode, Confirm uses the current directory when no child is selected.
For a new output file, enter the complete path directly in the form field. The
eye icon beside the navigation controls toggles hidden files. Closing the dialog or
pressing Escape cancels without changing the field; Escape while editing the
location only exits location editing.

Input-file fields select existing regular files; directory fields select an
existing directory; socket fields select Unix sockets. Output-file fields such
as logs and caches can select an existing file in the dialog or accept a new path
typed into the form field. Selection never creates a file or directory. Confirming checks
the current target type and existence again and updates only the form draft.
Manual paths and the ordinary Save / Validate / runtime workflows remain available.

Relative paths are resolved against `runtime/` in the node's data directory, the
core's working directory, and selections fill in absolute paths. Missing initial paths open the
nearest existing parent with a notice. Errors appear once in the center of the
directory area, replacing its entries until a retry or navigation clears the error.
Symbolic links retain their selected path while their target determines the type.
Dot components such as `link/..` are interpreted by the OS without lexical path
cleaning that could change the target. Broken links cannot be selected.
A successful selection does not validate file
contents or guarantee access, write permission or continued existence at startup.

HTTP request paths, URLs, regular expressions and namespace tags are not filesystem
fields. Path annotations are frontend presentation metadata, never executable JSON
or modifications to the served schema. The browser endpoints return
only directory/path metadata, require management authentication and disable caching.
Browsing is non-recursive, returns at most 200 entries per page, and rejects
directories exceeding 20,000 entries; enter a subdirectory directly in that case.
Unix filenames that are not valid UTF-8 cannot be represented by the API and are
omitted from listings. The existing local-file upload controls remain uploads.

## Validate and load configuration

A check snapshots immutable configuration bytes and runs
`sing-box check` against the selected binary without touching the live
core. The configuration page offers Save configuration and Validate configuration;
validation uses `POST /api/v1/nodes/{nodeId}/config/compile` and returns the completed artifact state.
Use Enable in version management to select a binary, then Start or Restart to load the current saved configuration.
Those operations perform preflight in the serialized runtime controller before changing
the running process. The Web UI uses
`POST /api/v1/nodes/{nodeId}/core/artifacts/{artifactId}/enable` and the runtime endpoints.
Startup artifacts and activation bundles remain internal evidence.
Enable switches binaries with the current saved document while preserving
stopped/running state and returns after the operation completes.

Start and Restart recheck the current file, canonical head, artifact identity,
and startup evidence. A concurrent configuration or artifact change cannot be combined with
stale bytes, and a failed preflight leaves the running core and the saved file
unchanged.

The monitoring tier follows the saved configuration: `limited` when the final
configuration exposes a secret-protected Clash API on a loopback address and
completes the `/version` handshake, otherwise `process_only`, which checks
process health only. Nothing is injected into the file to create that endpoint.

`GET /api/v1/nodes/{nodeId}/core/status` reports `enabled_core` separately from
`running`. The former persists while stopped; the latter remains evidence of a
verified live process. A checked selection while stopped does not claim a loaded
configuration or create a running process identity.

`POST /api/v1/nodes/{nodeId}/core/artifacts/{artifactId}/disable` queues a fenced stop for the
currently selected artifact. Successful completion clears desired, applied and
rollback pointers and removes the current symlink; it keeps immutable artifacts
and lifecycle history. Requests for an artifact that is no longer selected are
rejected. Ordinary Stop retains selection. Enable another version before
starting again after disable.

## Lifecycle and rollback

Use the Web UI's runtime controls to inspect status, start, restart or stop the
selected core. The authenticated runtime API supports the same operations and is
the only rollback entry point (`POST /api/v1/nodes/{nodeId}/core/rollback`); the Web UI has no
rollback control. The CLI manages the panel service itself.

Start and Restart use the last successfully selected binary identity and the current saved
file. A changed file is snapshotted for binary preflight in the serialized runtime
lane. The desired process and current observation remain unchanged until that
check passes. The controller rechecks the current file, canonical head, artifact identity,
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

Core operations return the completed resource or observed runtime state in the
requesting call. The node holds its process lease and serializes configuration
checks and process controls. Catalog, artifact and source maintenance are exposed
through the Web UI and authenticated HTTP API. Interrupted requests are canceled at safe boundaries;
completed side effects retain their evidence. There is no generic operation queue.

## Runtime identity and history

A running `RuntimeIdentity` includes the observed `started_at` value together
with the verified PID, process start token, exact core, recorded installation hashes, and
activation bundle. Installation hashes are metadata and are not compared against
core files during checks, activation, rollback or recovery. Configuration snapshot
and Schema digests remain checked. See [core verification boundaries](core-versions.md#checks-and-recorded-hashes).
While that identity remains verified, the node refreshes
its persisted observation watermark every 30 seconds so a later recovery can
distinguish current evidence from a stale process incarnation.

Runtime lifecycle evidence is append-only and permanently retained in the
node's `runtime_transitions`. The node sends each entry to the panel; after each
handshake the panel names the newest entry it holds for that node database, and
the node sends the rest, so history survives disconnections without a buffer
limit. The panel keeps each node's copy until the node is removed. The
authenticated `GET /api/v1/nodes/{nodeId}/core/runtime/history` endpoint filters
by time, state, reason, or activation bundle and paginates newest-first with the
stable `occurred_at`/`id` cursor pair. Transitions use `running`, `stopped`, `failed`,
or `unknown`; an `unknown` record includes `uncertain_since` and represents an
interval that cannot be classified as uptime or downtime.

Lifecycle success and controlled stop commit the final process observation,
transition and runtime intent in one SQLite transaction. Commit failure does not
report success; a newly started process is stopped if its evidence cannot be
committed. Reconciliation and shutdown append their observed history separately.
Stable dedupe keys and verified PID/start-token fencing prevent a stale request
from changing a newer process incarnation. Database initialization adds one
`history_initialized` marker and does not infer earlier uptime from logs.

Runtime recovery runs on the node and uses a dedicated persisted record: at most
three attempts with 1, 5 and 30 second delays. Deadlines survive node restarts. Five minutes of
continuously verified healthy runtime reset an episode; downtime alone does not.
Explicit lifecycle requests supersede the old recovery episode.

## Panel settings

The panel settings API at authenticated `GET/PUT /api/v1/panel/settings` is a
projection of the shared settings file selected by `--config`. The Web UI,
CLI and manual edits have one source; no database preferences override it.
The API's revision is an opaque JSON-safe fingerprint of the exact file bytes,
not a sequence number. Manual edits, including formatting changes, invalidate
older forms; stale writes return a conflict without changing either resource.
The Web UI has six categories: service settings, traffic usage, log management,
interface preferences, system maintenance, and backup and restore. Service settings
groups access and authentication; interface preferences groups
appearance and language; system maintenance groups storage and version updates.
Log management controls only sing-box capture files; panel events do not expire automatically.
Each topic keeps its own section heading.
The categories are tabs in the settings window's title strip; narrow screens use a category selector there.
All topics share one draft and Save button. Hash navigation and browser history
retain the draft; leaving `/settings` or closing the page warns about unsaved changes.
Validation locates the offending category and field; failed saves retain drafts.
Email and new password are separate inline fields in Authentication. Neither requires
an old password or repeated password confirmation. Both remain drafts until the shared
Save button is used. An empty new password preserves the existing password. Invalid
email or password input prevents the entire save; failed saves retain the draft.
Settings saves return `{ settings, reauthentication_required }`. Actual credential changes
sign out every session; unchanged credentials do not. Credential reads expose the
administrator email and GitHub configured flag;
GitHub tokens have an explicit remove control. Subscription source access settings
are hidden in this UI and preserved during ordinary saves.

### Shared settings file

Existing file fields retain their paths. Web-only preferences are added under
`panel`, with matching defaults. Hidden subscription source access fields remain supported in the file and API. The `service` API projection covers non-secret service options and is optional on writes. Settings validation and revision checks cover the entire file. Changing service options that are captured at startup displays a restart notice.

Configuration validation, settings writes and backup restore reject any field
outside the table below. The one exception is `logs.panel_retention_days` from
the previous release: its value carries over to
`nodes.runtime_history_retention_days`, the server rewrites such a file at its
next start, and `config set` and backup restore write the current shape. `config init --force` replaces settings with current
defaults, including a new administrator password.

Session Cookie `Secure` follows the active `server.external_origin`: HTTPS enables
it and HTTP disables it. When no external origin is configured, the actual request
TLS state determines it. Forwarded headers cannot override this policy. Origin
changes take effect after restart.

| Settings field | Web field | Generated default |
| --- | --- | --- |
| `server.host` / `server.port` | Listener host / port | `127.0.0.1` / `3000` |
| `server.external_origin` | External origin | Empty |
| `server.base_path` | Base path | Empty |
| `nodes.runtime_history_retention_days` | Runtime states retention | `90` |
| `data_dir` | Data directory (absolute path) | Root or XDG data directory |
| `auth.email` | Administrator email | `admin@example.com` |
| `auth.password_hash` | Password (hash stored only) | Argon2id hash of a random password |
| `github.token` | GitHub token | Empty |
| `github.catalog_refresh_interval_hours` | GitHub version check interval | `12` |
| `traffic.sample_retention_days` | Metric retention | `7` |
| `subscription.private_source_cidrs` | File/API only | `[]` |
| `logs.core_retention_days` | Core capture retention (UTC dates including today) | `7` |
| `logs.core_max_files` | Maximum core capture files | `0` (unlimited) |
| `logs.core_max_file_size_mib` | Maximum capture file size | `32` MiB |
| `backup.interval_hours` | Backup interval (0 turns scheduled backups off) | `24` |
| `backup.keep` | Scheduled backups kept | `7` |
| `panel.language` | Language | `zh-CN` |
| `panel.appearance.theme` | Theme | `system` |
| `panel.appearance.color` | Color | `#6D4ED1` |
| `panel.appearance.radius` | Radius | `12` |

### Nodes

Nodes connect to the same `server.host:server.port` as the Web application;
the panel opens no other port. The node endpoints are WebSockets under
`server.base_path`, beside the Web application: with base path `/panel`, a
node's session opens `/panel/api/node/v1/connect`. The panel itself speaks only
plain HTTP, so it accepts node connections only from its own host, from a node
there or from a reverse proxy there that terminates the HTTPS of nodes
elsewhere, and refuses every other peer: node traffic never crosses a network
unencrypted. Each side pings the other every 30 seconds, end to end through any
proxy between them, and ends a session whose peer does not answer within 10
seconds.

Enrollment codes name the panel URL that nodes use: an HTTPS
`server.external_origin` with the base path, else the listener over plain HTTP,
which only nodes on the panel host can use. Nodes on other hosts therefore need
a reverse proxy with a publicly trusted certificate, with
`server.external_origin` set to its origin; issuing a code fails while the
listener is not a loopback address and no HTTPS origin is set. Nodes verify
HTTPS with the system's certificate authorities over TLS 1.3, and use plain
HTTP only to a loopback address.

Nodes keep the URL from their enrollment. Changing the external origin, base
path or listener address therefore requires enrolling nodes again with new
codes. Settings files that still name the removed `nodes.endpoint` or
`nodes.trust` are rejected: delete those fields.

#### Behind Nginx

Set `server.external_origin` to the public HTTPS origin. One location serves
the Web application and the node endpoints together; it only has to pass
WebSocket upgrades, for example with `server.base_path` set to `/panel`:

```nginx
# In the http block: ask the panel to upgrade only when the client did.
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl;
    http2 on;
    server_name www.example.com;
    ssl_certificate     /etc/letsencrypt/live/www.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/www.example.com/privkey.pem;

    location /panel/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
```

Without a base path, use `location /`. HTTP/2 carries a browser's live streams
over one connection: over HTTP/1.1 a browser opens at most six connections to
the panel, and the streams of two open node windows use them all, leaving other
requests waiting. WebSocket upgrades, the nodes' and the browser's alike, still
use HTTP/1.1 on their own. Nginx before 1.25.1 writes it as
`listen 443 ssl http2;`. The keepalive pings keep node sessions
within Nginx's default 60-second `proxy_read_timeout`, and session frames are
not request bodies, so `client_max_body_size` does not limit them. Nginx offers
TLS 1.3 by default since 1.23.4. Keep the existing Web proxy settings, such as
those in
[reverse proxy streaming](subscriptions-and-observability.md#reverse-proxy-streaming).
Run the proxy on the panel host, since the panel refuses node connections from
other hosts. Caddy passes WebSocket upgrades without further settings.

`data_dir` remains the common root: ordinary users default to
`$XDG_DATA_HOME/sing-box-panel` (or `~/.local/share/sing-box-panel`), and root
defaults to `/var/lib/sing-box-panel`. A relative `data_dir` resolves beside
`setting.json`, independently of the shell's current directory. Derived paths
are not duplicated as independent settings:

```text
data_dir/
  panel.db                # product state, nodes and their configurations, metrics and runtime history
  logs.db                 # the panel log, kept until it is cleared
  artifact-cache/         # verified core archives that nodes install from
  imports/                # temporary core uploads
  control.sock            # private process control
  process.lock            # process ownership
```

Each node keeps its installed cores, disposable configuration snapshots and
core logs in its own data directory and runs sing-box with that directory's
`runtime/` as the working directory, so relative paths inside a native
configuration resolve there on the node; see [Nodes](cli.md#nodes). The panel
settings file and its write/recovery sidecars remain beside the selected
`--config` path.

API revision, configured-secret flags and restart status are
response metadata and are not settings fields. File reads include credentials;
the Web API continues to redact them. Traffic quotas and periods are not panel
settings: each node has its own; see
[Limited monitoring and traffic](subscriptions-and-observability.md#limited-monitoring-and-traffic).

Raw sample, runtime history and core-log retention, appearance, language, GitHub token and catalog refresh policy
are applied after saving. Background cleanup/refresh workers receive change
notifications and read consistent snapshots. The shared metrics state refreshes
once after saving, then continues over SSE. Credentials invalidate old sessions.
Listener host/port, base path, origin, secure cookies and data directory are bound
to the running HTTP service and require a panel restart, shown in the UI; see
[Restarting the panel](cli.md#restarting-the-panel).
Changing `data_dir` requests relocation on the next explicit start/restart; saving
does not interrupt the service or change the running sing-box process.

Writers use a private persistent `.lock` sidecar and atomic file replacement.
Web saves use a temporary private `.pending` recovery journal and a SQLite commit
marker so settings saves recover to a known outcome after interruption. The
marker contains transaction identity, not a second copy of settings. Startup recovers before serving requests. Incomplete
updates block file commands; a conflicting external edit is never overwritten
by automatic recovery. Keep the selected file and its directory writable by the
service account; see [systemd permissions](systemd.md#system-service).

Subscriptions publish each node's inbounds at that node's own public host, set
in its settings, or else at the public IP the node reports; see
[Fleet inbounds](subscriptions-and-observability.md#fleet-inbounds).
The host never rewrites the actual listener, port, TLS server name or imported
node address, and loopback or wildcard addresses are never published.

New inbound entries are created locally from the selected protocol and tag.
Credentials belong to each inbound's native configuration and are entered in its
editor. Saving panel settings never generates or rewrites inbound credentials,
and does not require the saved sing-box text to be syntactically complete.

The former shared credential fields `panel.identity_name` and `panel.identity_key`,
the API fields `identity_name`, `identity_key`, `clear_identity_key` and
`identity_key_configured`, and `POST /api/v1/nodes/{nodeId}/config/inbound-defaults` have been
removed. This development-stage change has no compatibility fallback: settings
files and API writes containing the removed fields are rejected. Remove those
panel fields before loading an existing file. Existing native
inbound usernames, passwords and UUIDs are preserved.

Appearance offers five presets and a custom HEX picker, with radius 0–32px (default 12).
Preview changes page, controls, charts and dialogs immediately while semantic
status colors and the logo stay independent. Card/dialog radius is R, controls
R/2, and the shell min(32,7R/6). Saving persists preferences. Leaving the page with unsaved edits requires confirmation: Cancel
retains the current view and preview; Discard changes restores saved settings
before navigating. Reset changes only theme color/radius and still requires
saving. Help is in hover/click tips.

The theme selected in panel settings is the server default. The status line's theme
button (the More menu on phones) applies and persists a browser-only override immediately, without making
requests or changing the settings form. On page load, a valid local choice takes
precedence over the server theme, including an explicit `system` choice. Saving
or discarding panel settings leaves this local override intact; color and radius
continue to follow panel settings and their previews.

### Backup and restore

A backup holds the categories chosen for it, and a restore replaces the
categories chosen of what a backup holds:

| Category | Holds |
| --- | --- |
| `settings` | The saved settings file |
| `nodes` | The panel identity nodes trust, the nodes, their enrollments and configurations |
| `subscriptions` | Subscription sources, the node pool, and channels with their groups and exit rules |
| `rule_sets` | Rule sets |
| `keys` | Subscription keys with their users and grants |
| `history` | Nodes' traffic and runtime records |
| `logs` | The panel log |

Back up now and `system backup` take every category but `logs` unless told
otherwise, as scheduled backups do. A scheduled backup's `history` leaves out
the raw traffic samples and keeps the daily traffic, traffic periods and
runtime records; manual and pre-restore backups keep the raw samples. Unsaved
drafts, login sessions, the release catalog, core binaries and anything on node
hosts are never included, and nothing a backup leaves out stays readable in its
database file. A backup can hold the administrator's password hash, the GitHub
token and the subscription keys; keep it as private as the settings file.

Backups live in `<data_dir>/backups`, each readable by the service account
alone, named `sing-box-panel-<date>-<time>-<kind>.tar.gz`: `manual` for Back up
now and `system backup`, `scheduled` for the schedule, and `pre_restore` for the
state a restore replaced. Every archive starts with a readable manifest of its
categories, record counts, time and panel version, which names no setting or
secret, so the list shows what each backup holds. A backup made with a
passphrase of at least 8 characters keeps the rest in `payload.tar.gz.age`,
encrypted with [age](https://age-encryption.org). The passphrase is stored
nowhere, so a lost passphrase loses that backup. Scheduled and pre-restore
backups are not encrypted. `system prune` removes the data directory with its
backups: copy the ones to keep off the host.

The schedule makes a backup when the newest scheduled one is older than
`backup.interval_hours` (default 24; 0 turns it off), then keeps the newest
`backup.keep` scheduled backups (default 7). It looks a minute after the panel
starts and every 10 minutes after, so a changed setting needs no restart.
Manual and pre-restore backups stay until deleted.

Panel settings → Backup and restore sets the schedule and lists the backups
here, newest first, each with download, restore and delete. Back up now chooses
the categories and an optional passphrase. Restore picks one of these backups or
a file from the browser's computer.

A restore checks the whole backup before anything changes: its manifest and
every file's digest, an encrypted payload's copy of the manifest, that its
databases are intact, are a panel's and are no newer than this release, and
that its settings, if any, are valid here. The categories it holds are then
offered with their counts. The panel keeps the categories left out, so it
repairs what in them referred to records that are gone, by the schema's own
rules: pool nodes published from fleet nodes that are gone leave with their
group places and grants; exit rules whose rule set is gone are removed; exits
that named a removed pool node fall back, an exit rule's to reject and a
channel's final exit to direct; and the records of nodes that are gone are
removed. The dialog says what the restore will change this way before it
starts. Restoring nodes from another panel means each node must enroll again.
Restoring `history` replaces all of it with the backup's, raw traffic samples
included: after a scheduled backup's history the panel has no raw samples,
not even those recorded since the backup, because each node's next traffic
counts from the restored checkpoint and those samples would count it twice.
Charts that read raw samples fill again as nodes report; the pre-restore
backup still holds the samples that were replaced.

The data directory always stays this host's; with the settings, the listener,
base path and external origin stay too unless the restore takes the backup's.
Restore and restart backs up the current state first, all of it but the log
unless the log is restored too, and restarts the panel, which replaces the
chosen categories before it opens its databases. Restoring the settings ends
every login session: sign in with the backup's administrator. Where the panel
cannot restart itself, the restore applies when it next starts. A restore
interrupted midway completes at the next start; one whose staged files are gone,
whose settings are no longer valid, or whose records do not fit this panel's
database is dropped with nothing changed, and the panel log says why.

The Web interface uses `/api/v1/panel/backups` and `/api/v1/panel/restores`; the
[CLI](cli.md#backups-and-restore) backs up and restores a stopped panel.
Backups from earlier releases are rejected.

### Data directory changes

The existing defaults remain unchanged. Edit `data_dir` through
`config set data_dir /path/to/data`, complete-file replacement with
`config set --file`, or the settings file; no separate data-directory command exists.
A private `setting.json.location` records the established directory so manual
edits can be distinguished from a new instance. It is relocation metadata, not
another configuration source. Status and stop continue to locate the old
instance before relocation. Keep this sidecar with the selected settings file.

On the next explicit `server start`, the panel requires exclusive ownership of
the old data directory and its database.
It copies files, takes a consistent SQLite snapshot (including committed WAL),
records the move, verifies the copied content,
and only then removes the source data and commits the new location. This works
across filesystems. Interrupted copies are rebuilt; interrupted cleanup resumes
from the verified destination. Relocation markers block ordinary database access
to unfinished locations. Missing established storage, nonempty destinations,
nested instances, overlapping paths and settings located inside either data
root fail closed. Native sing-box JSON and historical evidence are unchanged.
Paths inside native JSON name files on each node, so moving the panel's data
does not affect them.

For generated systemd units, an explicit `systemd restart` stops the service,
performs the move outside its sandbox, updates directory/ownership declarations,
and starts the service again. `systemd start` does the same preparation for a
stopped service; it never stops an already running one implicitly. Customized
units or drop-ins are not overwritten: stop the service and explicitly reinstall
its unit with the selected settings. Failed relocation leaves the service stopped
and can be retried after the cause is corrected. A sandbox that prevents removal
of an empty old root may leave only a private relocation marker there.

Upgrade once with the original settings and data directory before manually
changing that path. Without an existing location record or the previous file
seen by `config set`, an arbitrary former custom directory cannot be inferred.

### Stable JSON presentation

Visual configuration forms and edits, node JSON previews, explicit formatting,
and sing-box channel output use the shared field priorities in
`internal/configuration/configuration-order.json`. Nodes place identity first, then listen/server
addresses and socket options, authentication, protocol options, and TLS/transport
blocks. Common fields retain the same relative order across protocols. TLS blocks
group enablement and server identity, negotiation (ALPN and versions), certificates
and keys, then advanced options. Explicit Schema presentation priorities still
take precedence in forms. Remaining fields are ordered by name. Object formatting
preserves unknown fields, exact numeric spelling, null/false values, and all array
orders, including route rules and strategy candidates. Editing raw JSON does not
reformat each keystroke; saving the configuration file still preserves exact text,
including incomplete drafts. Runtime canonicalization and content digests are
separate and unchanged.
