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

The Web editor manages this document through `GET/PUT /api/v1/config/file`.
The numeric file revision is a compare-and-swap guard, starting at `0` before
the first save. Reads return the stored text exactly, including unfinished
JSON, together with `revision`, `syntax_valid`, and `canonical_revision_id`.
A stale save fails instead of merging implicitly. The logical name `config.json`
is not a filesystem path: the text lives in the `configuration_file` table of
`panel.db` inside `data_dir`. There is no second writable runtime configuration
on disk and no sing-box configuration CLI.

Valid saves synchronize the editable document and immutable revision in one
transaction. Saved invalid text blocks validation, Enable, Start, and Restart until corrected
in the Web editor. Revision values protect concurrent edits; neither the CLI nor
the browser offers historical selection, comparison, or restoration. Internal
immutable evidence remains for native checks, runtime identities, and recovery.
The CLI's `config` group instead manages the panel settings file; see
[Panel settings](cli.md#panel-settings).

Ordinary CI and signed releases exercise this same file API across a real
self-update and restart. The release fixture includes whitespace and a number
beyond JavaScript's safe integer range; the scenario also retains unfinished
JSON through the update before correcting it. The saved text and runtime snapshot identity survive the restart unchanged.
Store/API integration tests separately verify the immutable snapshot bytes and digest. See [Release process](release.md#verification-scope) for the native checks.

## Switching versions without rewriting JSON

Selecting a different core carries the current JSON forward unchanged. The
panel does not infer which keys an older release accepts and does not silently
drop fields. A check snapshots the current valid file and asks that exact,
digest-verified binary to validate the execution snapshot. Snapshot formatting
may differ from the saved text, but field names and values are preserved and
the saved text is untouched. Select the target core and use Check in the Web UI.

If the configuration uses fields unavailable in that version, `sing-box check`
fails, the startup artifact becomes failed, and the currently applied runtime
is left untouched. There is no separate editable startup JSON or
version-specific transformation path. The Web UI requests the check
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

The panel currently commits native Schema output for `1.14.0`. Before installing a core, the Web visual editor can use that bundled schema for authoring and labels the exact target version. Installed artifacts still require the served schema to match the reviewed manifest. Native validation and runtime startup always require an installed matching core. Releases before
sing-box added `sing-box schema`, including `1.13.19`,
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
the complete effective configuration, with JSON syntax highlighting, line numbers,
folding, search/replace, and lossless formatting (also available with Ctrl/Cmd+Shift+F).
Formatting preserves numeric literals and remains undoable; incomplete input is left intact.
The editor loads on demand. The browser can save invalid JSON; it disables visual editing and binary
validation until the text is a valid object. Saving and checking lock editing
until the result arrives, so feedback describes the submitted file. Validation
success is a Toast shown only after the binary check succeeds. Unknown fields and
numeric lexemes are retained through visual edits.

## Validate and load configuration

A check snapshots immutable configuration bytes and runs
`sing-box check` against the selected binary without touching the live
core. The configuration page offers Save configuration and Validate configuration;
validation uses `POST /api/v1/config/compile` and returns the completed artifact state.
There is no separate Apply action on that page. Use Enable in version management
to select a binary, then Start or Restart to load the current saved configuration.
Those operations perform preflight in the serialized runtime controller before changing
the running process. The Web UI uses
`POST /api/v1/core/artifacts/{artifactId}/enable` and the runtime endpoints.
Startup artifacts and activation bundles remain internal evidence.
The CLI retains `core enable CORE_ARTIFACT_ID` to switch
binaries with the current saved document while preserving stopped/running state;
it returns after the operation completes.

Apply rechecks the current file, canonical head, artifact identity, and startup
evidence. A concurrent configuration or artifact change cannot be combined with
stale bytes, and a failed preflight leaves the running core and the saved file
unchanged.

The monitoring tier follows the saved configuration: `limited` when the final
configuration exposes a secret-protected Clash API on a loopback address and
completes the `/version` handshake, otherwise `process_only`, which checks
process health only. Nothing is injected into the file to create that endpoint.

`core status` and `/api/v1/core/runtime` report `enabled_core` separately from
`running`. The former persists while stopped; the latter remains evidence of a
verified live process. A checked selection while stopped does not claim a loaded
configuration or create a running process identity.

`POST /api/v1/core/artifacts/{artifactId}/disable` queues a fenced stop for the
currently selected artifact. Successful completion clears desired, applied and
rollback pointers and removes the current symlink; it keeps immutable artifacts
and lifecycle history. Requests for an artifact that is no longer selected are
rejected. Ordinary `core stop` retains selection. Enable another version before
starting again after disable.

## Lifecycle and rollback

```sh
sing-box-panel core status
sing-box-panel core start
sing-box-panel core restart
sing-box-panel core rollback
sing-box-panel core stop
```

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
requesting call. The server holds the process lease and serializes configuration
checks and process controls. CLI callers use its private Unix socket rather than
creating another process manager. Catalog, artifact and source maintenance can
run directly from the CLI. Interrupted requests are canceled at safe boundaries;
completed side effects retain their evidence. There is no generic operation queue.

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

Lifecycle success and controlled stop commit the final process observation,
transition and runtime intent in one SQLite transaction. Commit failure does not
report success; a newly started process is stopped if its evidence cannot be
committed. Reconciliation and shutdown append their observed history separately.
Stable dedupe keys and verified PID/start-token fencing prevent a stale request
from changing a newer process incarnation. Database initialization adds one
`history_initialized` marker and does not infer earlier uptime from logs.

Runtime recovery uses a dedicated persisted record: at most three attempts with
1, 5 and 30 second delays. Deadlines survive panel restarts. Five minutes of
continuously verified healthy runtime reset an episode; downtime alone does not.
Explicit lifecycle requests supersede the old recovery episode.

## Panel settings and protocol identity

The panel settings API at authenticated `GET/PUT /api/v1/panel/settings` is a
projection of the shared settings file selected by `--config`. The Web UI,
CLI and manual edits have one source; no database preferences override it.
The API's revision is an opaque JSON-safe fingerprint of the exact file bytes,
not a sequence number. Manual edits, including formatting changes, invalidate
older forms; stale writes return a conflict without changing either resource.
The three UI categories remain service/security, nodes/subscriptions, and
statistics/appearance. Credential reads expose only configured flags; an
omitted credential preserves it. GitHub tokens and identity keys have explicit remove controls; clearing the identity key preserves existing inbound credentials.

### Shared settings file

Existing file fields retain their paths. Web-only preferences are added under
`panel`, with matching defaults. Every setting has a Web control. The `service` API projection covers non-secret service options and is optional on writes so older clients preserve those values. Settings validation and revision checks cover the entire file. Changing service options that are captured at startup displays a restart notice.

| Settings field | Web field | Generated default |
| --- | --- | --- |
| `server.host` / `server.port` | Listener host / port | `127.0.0.1` / `3000` |
| `server.external_origin` | External origin | Empty |
| `server.base_path` | Base path | Empty |
| `data_dir` | Data directory (absolute path) | Root or XDG data directory |
| `auth.token` | Management token | Random token |
| `auth.secure_cookie` | HTTPS-only session cookie (synchronized with origin) | `false`; must match HTTPS origin |
| `github.token` | GitHub token | Empty |
| `github.catalog_refresh_interval_hours` | GitHub version check interval | `12` |
| `traffic.quota_gib` | Traffic quota | `null`; `null` and `0` are unlimited |
| `traffic.period_months` | Traffic period | `1` |
| `traffic.sample_retention_days` | Metric retention | `90` |
| `subscription.author` | Subscription author | `reagin` |
| `subscription.provider` | Subscription provider | `default`; editable, existing values retained |
| `subscription.private_source_cidrs` | Allowed private source networks (CIDR per line) | `[]` |
| `logs.retention_days` | Log retention | `7` |
| `panel.public_node_host` | Public node host | Empty; automatic detection |
| `panel.identity_name` / `panel.identity_key` | Protocol identity | Empty / empty |
| `panel.language` | Language | `zh-CN` |
| `panel.appearance.theme` | Theme | `light` |
| `panel.appearance.color` | Color | `#6D4ED1` |
| `panel.appearance.radius` | Radius | `12` |

`data_dir` remains the common root: ordinary users default to
`$XDG_DATA_HOME/sing-box-panel` (or `~/.local/share/sing-box-panel`), and root
defaults to `/var/lib/sing-box-panel`. A relative `data_dir` resolves beside
`setting.json`, independently of the shell's current directory. Derived paths
are not duplicated as independent settings:

```text
data_dir/
  panel.db                # product state, metrics and panel logs
  artifacts/              # installed sing-box cores
  runtime/configs/        # immutable execution snapshots
  imports/                # temporary core uploads
  logs/core/              # managed core log files
  panel-control.sock      # private process control
  runtime-executor.lock    # process ownership
```

The sing-box subprocess working directory is `data_dir/runtime`, so relative
paths inside its native configuration resolve there. The panel settings file
and its write/recovery sidecars remain beside the selected `--config` path.

API revision, detected public IP, configured-secret flags and restart status are
response metadata and are not settings fields. File reads include credentials;
the Web API continues to redact them. Quota accepts whole GiB from zero through
8589934591, preserving the existing file range and preventing byte overflow.

Tokens, public-node host, identity defaults and quota are read at operation
boundaries; language/appearance update when the Web view reloads or saves.
Listener, base path, origin/cookie policy, data directory, traffic
period/retention, private-source allowlist and log retention need a panel restart.
The catalog refresh interval is read dynamically by the background catalog
worker and does not require a restart. The API restart flag includes the
remaining file-only startup fields. Changing `data_dir` requests a relocation on the next explicit start/restart.
The current listener and data directory remain active until stopped.
A token change invalidates existing sessions. File edits of protocol identity
change defaults for new inbounds; updating existing sing-box credentials remains
an explicit Web action: enter the key and save, then perform a checked restart.
CLI file management never rewrites the sing-box document.

Writers use a private persistent `.lock` sidecar and atomic file replacement.
Web saves use a temporary private `.pending` recovery journal and a SQLite commit
marker so a file update and any protocol-identity update recover to the same
outcome after interruption. The marker contains transaction identity, not a
second copy of settings. Startup recovers before serving requests. Incomplete
updates block file commands; a conflicting external edit is never overwritten
by automatic recovery. Keep the selected file and its directory writable by the
service account; see [systemd permissions](../systemd/README.md#system-service).

The public-node host accepts a public IP or domain override; otherwise the
bounded public-IP detector provides the input placeholder and publication host.
It never rewrites the actual listener, port, TLS server name or imported node
address. Detection failure must not publish loopback or wildcard addresses.

Explicitly saving a changed common protocol name or entering its key updates
matching managed credentials in the saved configuration, coordinated with the
settings file through the recoverable transaction above. It preserves other existing
users, external client nodes and protocol-specific obfuscation secrets. UUID-
based protocols receive a UUID; Shadowsocks 2022 receives a method-sized key.
An invalid saved configuration or concurrent edit rejects the whole change.
The running core keeps its existing bytes until a checked restart.
`POST /api/v1/config/inbound-defaults` creates an authenticated, no-store editor
entry using that identity. It does not save or launch anything; the user reviews
and saves the new inbound through the normal configuration flow.

Appearance offers five presets and a custom HEX picker, with radius 0–32px (default 12).
Preview changes page, controls, charts and dialogs immediately while semantic
status colors and the logo stay independent. Card/dialog radius is R, controls
R/2, and the shell min(32,7R/6). Saving persists preferences. Changing category
or leaving the page with unsaved edits requires confirmation: Keep editing
retains the current view and preview; Discard changes restores saved settings
before navigating. Reset changes only theme color/radius and still requires
saving. Help is in hover/focus tips.

### Data directory changes

The existing defaults remain unchanged. Edit `data_dir` through the same
`config set` command or settings file; no separate data-directory command exists.
A private `setting.json.location` records the established directory so manual
edits can be distinguished from a new instance. It is relocation metadata, not
another configuration source. Status and stop continue to locate the old
instance before relocation. Keep this sidecar with the selected settings file.

On the next explicit `server start`, the panel requires exclusive ownership of
both the old runtime and database and proves that the recorded core has exited.
It copies files, takes a consistent SQLite snapshot (including committed WAL),
rebases installed-core and pending-import paths, verifies the copied content,
and only then removes the source data and commits the new location. This works
across filesystems. Interrupted copies are rebuilt; interrupted cleanup resumes
from the verified destination. Relocation markers block ordinary database access
to unfinished locations. Missing established storage, nonempty destinations,
nested instances, overlapping paths and settings located inside either data
root fail closed. Native sing-box JSON and historical evidence are unchanged;
operator-specified absolute paths inside native JSON remain the operator's
responsibility. Relative native paths still resolve under `data_dir/runtime`.

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
