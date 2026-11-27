# Repository architecture

This guide owns package boundaries, product requirements, and HTTP trust boundaries.
Operator workflows live in the [documentation index](README.md); exact HTTP
operations and schemas remain in [OpenAPI](../api/panel-openapi.yaml).

- [Repository layout](#repository-layout), [package layout](#package-layout) and [dependency direction](#dependency-direction)
- [Version support](#adding-a-sing-box-release-line) and [test ownership](#test-ownership)
- [Product contract](#product-contract)
- [HTTP API and security](#http-api-and-security)

## Repository layout

The product is one panel service and any number of node services. The panel
holds an authenticated session with every node, aggregates their data, manages
each node's configuration and publishes subscriptions; it runs no sing-box. A
node enrolls with the panel, runs one sing-box, reports its configuration,
state, metrics and traffic, and carries out the panel's typed commands. The
services, their protocol and state ownership are in
[Panel and node architecture](panel-node-architecture.md).

| Path | Contents |
| --- | --- |
| `cmd/sing-box-panel/` | The panel binary's `main.go`, which joins the CLI, the server and the embedded Web assets |
| `cmd/sing-box-node/` | The node binary's `main.go` |
| `internal/` | Every package, and the only `internal` directory: the panel's packages in `internal/sing-box-panel/`, the node's in `internal/sing-box-node/` and shared packages beside them |
| `api/` | Contracts only: the panel's management HTTP contract (`panel-openapi.yaml`), the node console's HTTP contract (`node-openapi.yaml`) and the panel–node protobuf module (`proto/`, with its `buf.yaml` and `buf.gen.yaml`; package `sbp.node.v1`) |
| `web/` | The pnpm workspace of the Web applications: the panel's in `apps/panel` and the node console's in `apps/node`, each with the Go package that embeds its production build, and the code both share in `packages/shared` |
| `tools/` | Maintainer commands run through `go tool`: release signing, third-party notices and OpenAPI verification |
| `scripts/` | Installer, isolated release build and native release tests |
| `docs/` | One guide per topic, indexed in [the documentation index](README.md) |
| `.github/` | Workflows, community and security policies, templates, code ownership and the release trust root |

Each Web application's Go package sits in its own directory under `web/apps/`
because Go embedding cannot read files from a parent directory. Other embedded assets live in the package that owns
them: subscription templates in `internal/sing-box-panel/subscription`,
the configuration field order in `internal/configuration`, each service's systemd
templates in `internal/sing-box-panel/systemd` and `internal/sing-box-node/systemd`, and the reviewed 1.13
configuration schema in `internal/sing-box-panel/schema`. The Web editor reads the templates and
field order from the same files.

## Package layout

`cmd/` holds only the two binaries' `main` packages. Each service's packages
sit in `internal/<binary>/`, named after the binary that runs them, and shared
packages sit directly in `internal/`; no Go file sits outside a package
directory. The two services meet only through `internal/nodeapi`, and shared
packages import neither service. Within a layer
the backend keeps one main package per domain and introduces another package
only for a concrete dependency or side-effect boundary; file prefixes make
ownership visible inside a cohesive package.

The panel runs no sing-box: `fleet` sends each command to the node's session
and maps its result to the management API's views, and the node's `service`
package runs the core. Each service has its own systemd unit: the shared
`systemd` package runs the lifecycle, and each service's `systemd` package
supplies its templates and the state that identifies an installation.
Tools under `tools/` are maintainer programs that nothing imports.

| Tree | Package | Responsibility |
| --- | --- | --- |
| Shared (`internal/`) | `buildinfo`, `jsonstrict` | Build identity and its display, strict JSON decoding and canonical objects |
| | `cliui` | What both binaries' command lines share: output formats, classified errors and exit codes, colors, aligned fields, file trees, diagnostics, update progress, foreground server output, help layout and the `version`, `completion`, `server stop|status` and `systemd` commands, the last over the `systemd` lifecycle |
| | `atomicfile` | Crash-safe file publication and directory syncs for both services and the tools |
| | `release`, `selfupdate` | Release-version validation and signatures; fetching, verifying and atomically installing either binary, staging a release for a system service's unit, and the unit's install step |
| | `procctl` | Private local control of a running service without an OS service manager; the service lifetime and lease, never background processes |
| | `settingsfile` | What both services' settings files share: bounded reads, the writer lock, data-directory resolution and the dotted field names of the config commands |
| | `database` | SQLite opening, connection pragmas, schema identity, the migration runner and data-directory locks shared by both stores |
| | `logredact` | Credential and configuration-body redaction for log text and metadata, applied by both services |
| | `configuration` | Strict, lossless sing-box JSON parsing, canonical serialization and the presentation field order |
| | `coreartifact` | Immutable identities of installable sing-box artifacts |
| | `singbox` | The supported sing-box release lines and what each needs: its configuration schema source, inbound family and runtime compatibility switches |
| | `nodeapi` | Generated `sbp.node.v1` protocol code, the protocol version, the WebSocket transport that carries its frames with its endpoint paths, close codes and keepalive, the session proof and the enrollment code format |
| | `systemd` | One service's Linux systemd lifecycle over its definition: audited unit rendering, install, uninstall, status, control and logs, the dedicated system account and its ownership of the service's files, and refusing a root start against files that account owns |
| | `pathguard` | Symlink-resolving containment and refusal of roots, homes and shared system directories, for directories a service owns, cleans, moves or binds |
| | `netguard` | Which addresses count as public: the only addresses the panel's source and core downloads reach without an operator's permission, and the only ones a node reports as its public address |
| | `webauth` | What both management APIs share: Argon2id password hashing, login admission with its rate limits and deadline, origin and CSRF checks, and security headers |
| Node (`internal/sing-box-node/`) | `cli` | The node's command tree: `enroll`, `update`, `server`, `config`, `system`, `systemd` and `completion` |
| | `settings` | The node's settings file (console address, external origin, data directory and console password hash) and the enrollment in its data directory (panel URL, IDs and the Ed25519 identity key) |
| | `agent` | Enrollment, the link that holds it with the session's state, the authenticated session with the panel, with reconnection, and the command executor with its idempotent results; the node only dials out |
| | `httpapi` | The console API: password login, the link's status, enrollment and unenrollment |
| | `systemd` | The node's unit templates and its settings file as the state of its systemd installation: data directory and created settings |
| | `store` | Node SQLite: received configuration revisions, installed cores, checked startup artifacts, activation bundles, runtime intents, observations and history, and command results |
| | `service` | The node's use cases: core installs, checks and activation, process supervision and recovery, core logs, filesystem selection and the public IP hint |
| | `artifacts` | Verification and publication of immutable installed core files |
| | `runtime` | Managed processes, reference-counted disposable configuration files and the restricted Clash API monitoring client |
| | `corelogs` | Bounded, private, sanitized native process output |
| | `host` | Host-wide metrics, the bounded public IP hint and read-only, non-recursive filesystem selection |
| Panel (`internal/sing-box-panel/`) | `store` | Panel SQLite: its migrations, each node's configuration and revisions, the cached core archives, subscription state and operational records |
| | `auth` | The administrator's email and login sessions |
| | `settings` | The panel settings file and its service: validation, defaults, atomic saves and recovery, change notification, backup export and restore |
| | `telemetry` | The panel log and operation outcomes, metrics and runtime history, each node's traffic periods and quota, the dashboard projection |
| | `corecache` | Upstream catalog discovery, staged core uploads and the verified archive cache that nodes install from |
| | `schema` | Configuration schema preparation for the editor and the panel's checks: the embedded reviewed 1.13 schema and the schemas nodes' cores print, with the presentation overlay |
| | `nodeconfig` | Each node's editable configuration file, canonical revisions, a core's configuration schema and its support, preview and the schema check before a command |
| | `subscription` | The node pool and its synchronization, sources and their parsers, inbound conversion, channels, keys, users, templates and renderers, preview and `/sub` delivery |
| | `fleet` | The node registry, enrollment codes, the node endpoints beside the Web application under the base path, authenticated node sessions, the typed commands sent over them and the protocol minor version each needs, batch commands over many nodes, node updates and the release stream they fetch, the shared fleet snapshot and the short-lived grants under which nodes fetch cached archives |
| | `releases` | The panel's releases: the verified cache of the node release of the panel's version that nodes update from, and the panel's own update checks and updates |
| | `app` | The one composition of the panel's services over a store and a settings file, used by the server and the transport tests |
| | `httpapi` | The management API, Web delivery and subscription output over `app.Services` |
| | `installation` | Inspection, moves and removal of the panel's persistent files |
| | `systemd` | The panel's unit templates and its settings file as the state of its systemd installation: data directory, created settings, owned sidecars and pending data moves |
| | `cli`, `server` | The panel's command tree; the server lifecycle around the composed services, the node listener and the HTTP server |
| | `testutil` | Fixtures shared by panel tests |

The panel `store` imports no panel service package; it uses the shared
`configuration` package to canonicalize the documents it stores. Services above
it validate domain rules and choose policies such as how many revisions to
keep; the store keeps structural integrity and the invariants that must hold
inside one transaction, such as traffic deltas against the checkpoint.
Transport packages translate input and output but do not own use-case rules.
Infrastructure packages own process, network, filesystem and database effects.
The web client depends on the documented HTTP contract rather than Go
implementation details.

## Dependency direction

Production imports between the module's packages, as reported by
`go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./internal/... ./cmd/... ./tools/...`.
`panel:` and `node:` abbreviate `internal/sing-box-panel/` and
`internal/sing-box-node/`; the `layout` test package and the panel's
`testutil` fixtures, which only tests use, are left out:

```text
atomicfile, buildinfo, coreartifact, database, jsonstrict, netguard, pathguard,
procctl, release, webauth (no module imports)

configuration      -> jsonstrict
logredact          -> jsonstrict
nodeapi            -> jsonstrict
singbox            -> coreartifact
selfupdate         -> atomicfile, jsonstrict, release
settingsfile       -> atomicfile, jsonstrict
systemd            -> atomicfile, database, pathguard
cliui              -> buildinfo, procctl, selfupdate, systemd

node:host          -> netguard
node:artifacts     -> atomicfile, coreartifact
node:runtime       -> atomicfile, coreartifact, singbox
node:corelogs      -> logredact
node:store         -> configuration, coreartifact, database
node:service       -> configuration, coreartifact, database, jsonstrict,
                      node:artifacts, node:corelogs, node:host, node:runtime,
                      node:store, singbox
node:settings      -> atomicfile, jsonstrict, nodeapi, settingsfile, webauth
node:agent         -> buildinfo, coreartifact, nodeapi, procctl, release,
                      selfupdate, node:artifacts, node:corelogs, node:host,
                      node:service, node:settings, node:store, node:systemd,
                      systemd
node:httpapi       -> buildinfo, jsonstrict, nodeapi, node:agent, node:host,
                      node:settings, webauth
node:systemd       -> settingsfile, node:settings, systemd
node:cli           -> buildinfo, cliui, nodeapi, pathguard, procctl,
                      selfupdate, settingsfile, node:agent, node:httpapi,
                      node:settings, node:systemd, systemd, webauth,
                      web/apps/node

panel:store        -> configuration, database, logredact
panel:auth         -> panel:store, webauth
panel:settings     -> atomicfile, database, jsonstrict, nodeapi, settingsfile,
                      panel:auth, panel:store, webauth
panel:telemetry    -> database, panel:settings, panel:store
panel:schema       -> nodeapi
panel:corecache    -> atomicfile, coreartifact, jsonstrict, netguard,
                      panel:settings, panel:store, panel:telemetry, singbox
panel:nodeconfig   -> configuration, panel:corecache, panel:schema,
                      panel:store, singbox
panel:subscription -> configuration, database, jsonstrict, netguard,
                      panel:settings, panel:store, panel:telemetry, singbox
panel:releases     -> buildinfo, release, selfupdate, panel:settings,
                      panel:store, panel:telemetry, systemd
panel:fleet        -> buildinfo, configuration, database, nodeapi,
                      panel:corecache, panel:nodeconfig, panel:releases,
                      panel:settings, panel:store, panel:telemetry
panel:app          -> buildinfo, panel:auth, panel:corecache, panel:fleet,
                      panel:nodeconfig, panel:releases, panel:settings,
                      panel:store, panel:subscription, panel:systemd,
                      panel:telemetry, systemd
panel:httpapi      -> buildinfo, configuration, coreartifact, jsonstrict,
                      nodeapi, selfupdate, panel:app, panel:auth,
                      panel:corecache, panel:fleet, panel:nodeconfig,
                      panel:releases, panel:schema, panel:settings,
                      panel:store, panel:subscription, panel:telemetry,
                      webauth
panel:installation -> database, jsonstrict, pathguard, procctl, selfupdate,
                      settingsfile, panel:releases, panel:settings,
                      panel:store
panel:systemd      -> settingsfile, panel:installation, panel:settings,
                      systemd
panel:cli          -> buildinfo, cliui, nodeapi, pathguard, procctl,
                      selfupdate, panel:app, panel:fleet, panel:installation,
                      panel:settings, panel:store, panel:systemd, systemd,
                      webauth
panel:server       -> buildinfo, cliui, procctl, panel:app, panel:corecache,
                      panel:fleet, panel:httpapi, panel:installation,
                      panel:releases, panel:settings, panel:store,
                      panel:subscription, panel:telemetry, systemd

cmd/sing-box-panel        -> buildinfo, cliui, selfupdate, panel:cli,
                             panel:server, web/apps/panel
cmd/sing-box-node         -> buildinfo, cliui, node:cli
tools/sign-release        -> release
tools/third-party-notices -> atomicfile
tools/verify-openapi      (no module imports)
```

Settings sit below the services that read them: `telemetry` reads the traffic
sample retention and `corecache` reads the catalog token and
refresh interval from `settings` directly, and `settings` depends on no other
service but `auth`, whose email rule and login sessions it uses for logins and
credential changes. The HTTP route that restores a panel backup records its
operation outcome.

`internal/layout` enforces the layout over `go list`: `cmd/` holds only the two
`main` packages, shared packages depend on neither service, neither service
imports the other's packages, and `sing-box-node` neither depends on the panel
nor embeds the panel's Web application.
`configuration` imports neither `singbox` nor the panel Schema. The panel `cli`
opens `store`, `settings` and `installation` directly for local maintenance,
manages nodes through `app`, and starts the server through a function supplied
by `cmd/sing-box-panel`.

## Adding a sing-box release line

A new patch or minor release of a supported line needs no change. A new line is
a row in [`internal/singbox/line.go`](../internal/singbox/line.go): its first
version, whether its binaries print their configuration schema, its inbound family
and whether it needs the legacy DNS switches. A new inbound family is a converter
in `subscription`. Review the release's configuration changes, migration notes and
inbound types first; see [release lines](core-versions.md#release-lines).

Do not add version-specific directories. Behavior that differs by line belongs in
the package that owns the capability and is selected through the line.

## Test ownership

- Go unit, integration, fuzz, and package contract tests live beside their
  production package. Shared test helpers remain in that package's
  `test_helpers_test.go`.
- Release-line differences use line- and family-driven table tests in
  `internal/singbox` and the panel `schema` and `subscription` packages. No
  test runs a real sing-box binary; the schema a node's core prints is tested
  against a fake core.
- React tests mirror each package's `src` ownership structure. API client tests are
  split by the same contract domains as the implementation.
- External inputs must skip in ordinary local tests and fail when their
  dedicated job requires them; do not recreate a centralized Docker E2E suite.

When moving a responsibility, move its focused tests and update callers in the
same change. Generated contracts and public compatibility boundaries must be
updated only through their designated workflow.

## Product contract

This section records the accepted product behavior and its implementation. It is
the product-behavior reference; OpenAPI and executable tests remain authoritative
for the implemented API. The matrix records the delivered behavior; the validation
record distinguishes automated and browser evidence from native-runtime limits.

### Baseline and precedence

The screens below are the implemented baseline; their visual rules live in
[`DESIGN.md`](../DESIGN.md). User management, draft/history workflows, three log
tabs and generic node-origin badges are not part of the product. Storage has no
upgrade path from earlier formats. Authentication, checked configuration snapshots and
panel release signatures remain enforced; the core installation trust boundary
is described in [Core versions](core-versions.md#checks-and-recorded-hashes).

The screens are login, the nodes home (key figures and node panes) and, for
each node, its overview, installed/available versions, visual and JSON
configuration, core logs and settings; subscriptions with the node pool, sources,
subscription keys, channels and channel detail; the panel log and every node's
runtime states; and panel settings.
Detail tabs and feedback are local states, not duplicate screens.

### Requirement → screen → contract → implementation → verification

All rows below describe the current implementation. The verification workflows
below exercise the corresponding source, API and runtime boundaries.

| ID | Screen and accepted behavior | Contract / implementation boundary | Verification coverage | Status |
| --- | --- | --- | --- | --- |
| UI-01 | App shell with the layout and shell parameters of [`DESIGN.md`](../DESIGN.md): the workspace fills the window and a status line below it leads back to the fleet and lists the scope's windows as numbered tabs, shows the fleet readout and an update hint, and carries theme, language, restarting the panel and sign-out; on phones it is a tab bar; every view's first row is a title strip of flat controls grouped by dividers; a calm glyph field fills the canvas behind the workspace and shows faintly through content surfaces, while fields and floating layers stay opaque; empty states show a title and the next step, and explanations sit in help tooltips; content fills the remaining height and scrolls with overlay scrollbars | App shell, status line, title strip, responsive layout, shared controls | Route tests for windows, preloading, the phone tab bar's more menu and focus; desktop and phone browser reviews | implemented |
| FLEET-00 | The nodes home, opened after sign-in and from `/` and `/overview`: a title strip that filters by state (all, online, offline) with counts and by tag, searches, turns on selection and adds a node; key figures for running cores, live traffic with both directions summed, active connections with the online nodes and this period's summed usage (never a summed quota), which scroll with the panes on phones; adding a node issues a one-time code in one dialog that waits for the host to enroll, shows the node once the fleet lists it and discards the unused code when it closes earlier; the screen follows the fleet stream while visible and the readout marks a stream that stays down | `/fleet/status` and `/fleet/stream` (every node with its metrics); node creation and enrollment code APIs | `fleet` status test with ten-second rates, fleet stream, filter and ETag route test, fleet summary tests, route test, enrollment dialog test; browser review of adding, canceling and re-enrolling a node | implemented |
| FLEET-01 | Every node as a terminal pane in a grid that scrolls in its own region: a title bar with a health dot for the node's worst condition with its reasons, name, host with its tags on hover, and what needs saying (quota reached, an update); a headline with live rates and connections, or how long the node has been away; the core's line; CPU, memory, disk and its own quota's use as meters along one success-to-danger scale, hatched while unknown; offline nodes, then nodes over quota, lead and live load never reorders them; panes off screen skip layout and paint | Fleet status API | Route tests; empty, no-match and load-failure states; 200-node browser review with live readings | implemented |
| FLEET-02 | Opening a node shows its title strip (the previous and next nodes in the current window, name, state, core state and version, uptime, rates, connections, runtime controls) and its windows in the status line; every view inside reads and commands that node only; an offline node keeps its last report marked stale and shows one region state in windows that command it | Node-scoped API client, node routes | Route tests for node scoping and offline state; node strip runtime tests | implemented |
| FLEET-03 | A node's settings save its name, published host, tags (at most 16, as chips) and traffic quota, period and reset day as one group, show the node's version and its last update with an update action confirmed with the brief interruption it causes, and keep re-enrollment, which waits for the host in the dialog that adds nodes, and removal in a separate destructive section that names the node | Node rename, settings, enrollment, update and deletion APIs with settings revisions | Settings validation and conflict tests; tag input test; browser save, re-enroll and remove | implemented |
| FLEET-04 | Selected panes (a selection mode from the title strip, Ctrl or Cmd click, Shift ranges, a touch held on a pane or Space; Escape, clearing or the strip's toggle leaves it) and command-line targets (a name, a glob, `tag:<tag>`, `all`) run start, stop, restart, install a core at an exact catalog version, or update; each batch names its nodes and what it interrupts and asks first, streams one result line per node to the command output and ends with a summary, which a toast carries when the line is closed | `/fleet/commands` with its result and done events; batch runner in the status line | Command parsing and target tests; route tests for a confirmed command-line batch and a selection batch | implemented |
| CMD-01 | A command box floating near the window's top over a scrim, opened by Ctrl or Cmd+K, `:` or the status line's action: `help`, `clear`, `exit`, window numbers, `open`, `goto`, `filter`, the batch verbs and `reboot`, with word completion in a list below the input, a shell's line-editing keys (Ctrl+A, Ctrl+E, Ctrl+U, Ctrl+W) and there the last command's result alone, without the command, until the next command replaces it | Shared terminal input line and buffer, pure command module | Command parsing, completion and scope tests; route test opening windows and nodes and replacing one result with the next; status line tests closing the box one layer at a time, taking a completion pressed with the pointer and editing the line with the shell's keys | implemented |
| UPD-01 | The panel settings' version updates card shows the running version, the newest stable release with when it was checked, a check action and an update confirmed with the brief interruption; the update's state follows until the panel restarts. A node updates to the panel's release from its settings tab or a sequential batch; system services stage verified releases and stop, and their unit's install step installs them before starting the service again; updates go one way, and the panel's update and node updates wait for each other | `/system/update`, `/system/update/check`, `/nodes/{nodeId}/update`, `/fleet/commands` update; `selfupdate`, `releases`, `fleet`, node `agent`; protocol `FetchRelease` and `UpdateNode` | `selfupdate` staged-apply refusals and restore, fleet release proof, minor gate, sequential batch and real-node update tests, OpenAPI conformance, release smoke node update | implemented |
| UI-02 | Shared action buttons in the sizes and variants of [`DESIGN.md`](../DESIGN.md), with matching square icon buttons and 44px touch targets; primary save, outlined secondary actions and semantic destructive actions; consistent hover/pressed/focus; centered dialogs; compact semantic-color Toast for all operation, loading and validation errors, without inserting error banners into page flow, sign-in refusals included; repeated persistent failures reuse one notification and recovery closes it; all dropdown fields use the shared custom Select, including protocol, source and channel forms, with consistent popup styling and keyboard interaction | Shared UI and feedback | Real actions, focus return, Escape, visible validation, disabled options, no false success | implemented |
| UI-03 | Data tables and item lists use regular-weight names, body text, statuses and row actions; table headers use medium weight; dropdown options use regular weight, with existing compact metadata sizes preserved | Shared table typography and feature-owned list styles | Browser checks across versions, sources, keys, channels, configuration rows and logs | implemented |
| AUTH-01 | Email/password login as a terminal prompt (`login:` and `password:` lines) with accessible labels, password visibility, refusals in a danger toast, fixed seven-day persistence and reduced-motion support | Existing authenticated session, CSRF/origin/rate-limit boundaries | Login/logout/error tests, credentials never exposed | implemented |
| RUN-01 | The node's title strip reports running/stopped/unknown accurately; start, stop and restart act on that node, with confirmation for stop and restart; theme/language/sign-out in the status line share one embedded size; keyboard focus remains visible; on narrow screens the context values scroll sideways on their own line | Runtime observation and lifecycle, node title strip | Unknown/stale never reported running; check failure keeps current process | implemented |
| DASH-01 | CPU/memory summary, transfer graph with 1h/24h inset tabs matching version management, and one-hour active connections graph; compact plot margins and external axes/units; rolling windows put the newest bucket at the right edge and remove expired samples on refresh without rebuilding the chart; no refresh/footer/extra core metric strip | Metrics history and real push updates | Real samples, reconnect, missing data, rolling time window, keyboard range switching | implemented |
| DASH-02 | 24h runtime history in 48 equal segments; running/failed/stopped/unknown with legend, no yellow state | Persistent runtime history | Restart persistence and unknown periods | implemented |
| CORE-01 | Installed/available fill-height lists, inline enable/download with persistent selection; source and capabilities retained; 5/10/50 pagination | Catalog, artifacts, runtime switch | Paging and scrolling; invalid switch retains runtime; artifact verification | implemented |
| CORE-02 | Filter assets by deployed OS/CPU architecture; no architecture badge or selector, never browser architecture or unknown→ARM64 | Platform detection, catalog/cache, imported artifact inspection | Cross-architecture assets and rate-limit/cache behavior | implemented |
| CFG-01 | One editable config.json, version-scoped root sections (13 in 1.14) with flat label/control rows, DNS/route sub-tabs and inline list editing; advanced JSON; floating icon tools and find/replace with case, word and regex matching; preserve unknown fields and numbers | Canonical saved text, version-scoped schema, validated startup artifacts | Lossless round trip; invalid JSON can save, cannot start | implemented |
| CFG-02 | Save independent of running process; loaded/dirty/restart-required/start-pending states; start/restart checks saved file before replacement | Saved-file identity vs loaded immutable artifact; CAS and runtime transitions | Concurrent edits, failed validation preserves running process, restart loads saved bytes | implemented |
| CFG-03 | The configuration window copies another node's saved configuration into its draft after a difference view, unsaved until saved | Configuration file API of the source node | Browser review of the copy dialog with a difference | implemented |
| SUB-01 | Pool nodes, sources, channels and keys; URL and file sources with include/exclude name filters and a preview of what they keep; edit settings with a file source's content in one save, refresh, and delete | Source API, SSRF-safe acquisition, strict parsers and one stored content per source | A failed fetch or parse keeps the current content and nodes; invalid filters and unconvertible kept nodes are refused with their location | implemented |
| NODE-01 | One pool of every published fleet inbound and every kept source node, as cards in a saved order without pagination; display name, origin, protocol, endpoint, TLS/Reality/SNI and transport; whole-card dragging saves the order; the detail edits only the display name | Pool synchronization with origin-derived IDs | Removed nodes leave every channel and grant; new nodes append; a stale order is refused | implemented |
| NODE-04 | Published addresses, credentials visible and copyable in the node detail, an inbound's detail links to its configuration; the pool cannot delete an inbound | Inbound provenance and configuration deep links | IPv6, unknown public IP, no listener/SNI/port mutation, real copy results | implemented |
| KEY-01 | Subscription keys replace user UI; expiry and successful-download quota shared across all channels; failed requests do not consume | Token storage and atomic delivery accounting; user-scoped access checks | Concurrent quota requests, expiry, failures, existing grants preserved | implemented |
| CHAN-01 | Strategy groups with ordered members picked from the pool and an optional match by origin, protocol and name; nodes/rules tabs; output/distribution/preview toolbar | A channel publishes only the nodes its groups name or match, its exit rules' exits and its final exit, in native output formats | Membership, match, order and format change; matched nodes follow members in pool order; removed pool nodes leave every group | implemented |
| CHAN-02 | Channel creation and editing open the strategy-group workspace directly; nodes are selected within each group; back, then link/preview/settings/save actions grouped at the right; the channel list shows each channel's client, published nodes, exit rules and final exit, opens a channel from its row and duplicates or deletes it from the row menu; the link popover, from a row or the workspace, shows the full URL with copy and a QR code, keeps its side and corner while it grows, and creates a key from a name when none is active; channel name and output client appear only in a cancelable settings draft, with no separate identity row; lightweight split workspace with independently scrolling group list and editor; group node/rule counts, card-level pencil actions with cancelable name dialogs; sidebar final-exit toggle beside delete; implicitly enabled groups; optional, exclusive final-exit badges; searchable candidate cards; automatic candidate exits, manual domain/IP/CIDR and remote references; rule dialogs omit exit and enablement fields; unmatched fallback | Typed channel configuration, native renderers; list summaries count the published pool nodes and exit rules and name the final exit | New group selects current eligible candidates; filtered selection preserves other members; edit/additional nodes preserve choice; valid exits; summary counts | implemented |
| RULE-01 | Remote metadata only, client fetches; Mihomo YAML/YML/TEXT/MRS + behavior; MRS excludes classical; sing-box source JSON/binary SRS; Loon native lists | Remote-reference validation per client; same-named remotes merge when their metadata matches and are renamed when it differs; native output | No server content fetch/upload/conversion; incompatible format cleared and blocked without URL loss | implemented |
| RULE-02 | A rule editor keeps the context of its rule set or exit rule; smaller child modal returns to correct parent; missing format red border only; anchored menus | Modal state, accessible invalid controls | Nested save/cancel/focus; short viewport sticky actions and complete menus | implemented |
| RULE-03 | Single-line full URL; lightning toggles gh-proxy.com for eligible GitHub/raw/gist; unwrap known proxy, no duplicates; off restores origin | URL normalization and effective render URL | Empty/non-GitHub disabled; preview/delivery uses effective URL; no credential forwarding | implemented |
| RULE-04 | A rule set's rules switch between entries and Clash classic text; a pasted Clash rule list converts, in the text or on the entries' prompt line, leaving out policy names and `no-resolve` with a note and refusing duplicates on the prompt line; problems are listed by line and block saving while the text stays | Rule set validation for its client | Unchanged round trip; per-line errors; refused save keeps the text | implemented |
| CHAN-03 | Distribution: name/client/template/new-node policy; direct node editing and card order; one ordered list of exit rules, each by a rule set of the channel's client or by an own rule, and a final exit | Channel data separation; rule sets of one client shared by its channels, deletion cascading to their exit rules | Preview and delivery share defaults; card and exit-rule order preserved; rule set order and exits in output; client mismatch refused; a deleted rule set advances its channels' versions | implemented |
| TPL-01 | Per-channel native JSON/YAML template; true edit/dirty/validate/location/preview/save/cancel; generated nodes/auth/groups/rules/fallback reserved | New template storage/API/merge/validation; no shared library or DSL | Reject conflicts and invalid save; preserve other channel; preview equals delivery | implemented |
| SET-01 | One page of settings cards; shared draft, changed-row count, revert and save, validation that focuses the invalid field, backup preview and atomic restore | Panel settings API/storage/bootstrap separation | Authenticated update, optimistic concurrency, invalid inputs, secrets redacted | implemented |
| SET-02 | Listen/address/domain, administrator credentials and optional GitHub token; independent email/new-password drafts, shared save; server-only secrets | Bootstrap/security settings and catalog client | Restart semantics, session invalidation, no token in response/log/browser persistence | implemented |
| SET-03 | Each node's public host override, else the public IP it reports; hidden subscription source access values preserved; inbound credentials edited natively; protocol obfuscation stays protocol-specific | Node settings and native configuration | IPv4/IPv6/domain, a node without a host publishes nothing, loopback never exported, imported credentials unchanged | implemented |
| SET-04 | Language, theme, five presets/custom color picker with HEX and explicit apply/cancel after closing; default #6D4ED1, radius 0–32 default12 | Persisted preferences and preview transaction | Save/reload; unsaved category/route changes require confirmation; cancellation retains draft, confirmed departure restores saved; reset only color/radius | implemented |
| SET-05 | Theme links accent/background/border/focus/charts through the derivation and radius steps in [`DESIGN.md`](../DESIGN.md); badges/logo/status independent | Shared CSS tokens and accessible color derivation | Text contrast ≥4.5 for arbitrary light/dark colors, radius bounds, all component states | implemented |
| LOG-01 | A node's core log tab shows its real-time core raw logs; timestamp muted, full message matches level; all native levels, file/date/level selectors consistent and right aligned; one top-right status pill toggles live output and pause, without a separate icon or surrounding container | Core log file/tail API | Real file reading/stream/reconnect/pause/filter, independent scroll | implemented |
| LOG-02 | The panel's logs window lists the panel's own log (operation outcomes and panel and security events about no node, filtered by level); a node's logs window has its events (the panel log's entries about it, where it opens), its runtime states (filtered by state) and its core log, the tab kept in the address; each log's Clear deletes only its scope's entries and shrinks the log's file | `/logs/panel`, `/logs` and `/logs/usage` with `scope=panel` or `node_id`, and `/fleet/runtime/history`; the panel log in `logs.db` | Upgrade, shrink and fleet history store tests; consistent displayed/filter levels; pagination/filter; clear flow | implemented |
| SYS-01 | Restart panel in the status line or `reboot` in the command line confirms, then the panel stops as a stop does and runs its program again in the same process; the readout's slot shows the restart until `/system/status` reports a later `started_at`, and says so when that takes over a minute; refused while an update runs or is staged | `/system/restart`, `/system/status`; `server` restart | Linux lifecycle test from the request to the re-execution; restart flow and command tests | implemented |

### Implementation and evidence map

| Requirements | Owning implementation and guide | Focused evidence |
| --- | --- | --- |
| UI-01/02, AUTH-01, RUN-01, FLEET-00–04, CMD-01 | `components/app-shell`, `components/status-line`, `components/title-strip`, `components/key-figures`, `components/enrollment-dialog.tsx`, `components/backdrop`, `components/node-workspace`, `utils/panel-commands.ts`, `utils/shell-windows.ts`, `utils/fleet-summary.ts`, `fleet/state.go`, `pages/fleet-page`, `pages/node-settings-page`, shared `components/terminal` and `components/ui`, `pages/login-page`; existing HTTP session and origin protections | App-shell, route, command and login tests; browser review harness with `SBP_BROWSER_FLEET`; real centered dialogs, Toast, focus and return journeys |
| DASH-01/02 | `pages/node-overview-page`, `internal/sing-box-node/host`, node `service/traffic.go` and `agent/reports.go`, `fleet/state.go`, `telemetry/nodes.go`, `httpapi/metrics_stream.go`, replicated runtime transitions; [observability guide](subscriptions-and-observability.md) | Node traffic accounting test, `fleet` state report, history and telemetry control test, per-node traffic store tests, chart and timeline tests, browser missing-data and layout review |
| CORE-01/02 | `pages/cores-page`, `fleet` core commands, `corecache` archive cache, `internal/sing-box-node/service` core commands and runtime preflight; [core guide](core-versions.md) | `fleet` install and grant tests, node artifact verification and runtime tests; inline action and platform UI tests |
| CFG-01/02 | `nodeconfig/file.go`, `store/configuration_file.go`, `internal/sing-box-node/service/configuration_runtime.go`, visual/JSON editors; [configuration guide](configuration-and-runtime.md) | File CAS and lossless tests; node `configuration_runtime_test.go`; real HTTP browser invalid-text save/reload; schema form union/unknown-field round trips |
| SUB-01, NODE-01–04 | `subscription/source_parse.go`, `source_filter.go`, `pool.go`, `inbound_convert.go`, `store/subscription_pool.go`, pool and source panels | `source_parse_test.go`, `pool_test.go`, `service_sources_test.go`, `httpapi/subscription_nodes_test.go`, 1.14 inbound/TLS tests; `subscription-node-panel.test.tsx`, `subscription-source-panel.test.tsx`; browser pool order, display-name edit and source preview |
| KEY-01 | Store key accounting, public delivery and key panel; [subscription guide](subscriptions-and-observability.md) | 24-request concurrency tests, cross-channel quota HTTP tests, expiry/revocation/304/failure accounting, explicit secret-read authorization, persistence and UI tests |
| CHAN-01–03, RULE-01–04, TPL-01 | `subscription/channel_*`, `subscription/service_rule_sets.go`, `store/subscription_channels.go`, `store/subscription_rule_sets.go`, channel and rule set API, channel list, link popover, workspace and nested editors, `pages/subscriptions-page/rule-text.ts` | Native renderer, exit-rule, group match and URL tests in `subscription/channel_policy_test.go`; list summary counts in `subscription/service_test.go`; channel list and link tests; `httpapi/subscription_channel_policy_test.go`; rule set CRUD, icons and order in `httpapi/subscription_routes_test.go`; rule text, picker and workspace tests; browser persisted group/reference reload, nested return and preview/copy |
| SET-01–05 | `settings/service_panel.go`, `service_backup.go`, `store/panel_settings.go`, `pages/panel-settings-page`, `theme/appearance.ts` | CAS/redaction/validation/atomic backup tests; appearance contrast/bounds tests; browser preview, draft retention, save/reload, leaving restores saved appearance |
| LOG-01/02 | `internal/sing-box-node/corelogs`, runtime output observer, `store/logs.go`, `store/panel_logs.go`, `store/runtime_history.go`, product log API, `components/panel-log`, `pages/node-logs-page` and `pages/panel-logs-page` | File/rotation/custom-output tests; operation logs/cursors and authenticated file API tests; frontend pause/filter and log rendering tests |

Go paths in this table are relative to `internal/sing-box-panel/` unless they
name another tree; Web paths are relative to `web/apps/panel/src/`. API details remain in `api/panel-openapi.yaml`; the owning guides describe
persistence, security boundaries and runtime semantics.

### Persistence and preserved guarantees

`internal/sing-box-panel/store/migrations/` holds the panel's storage
migrations, one folder for each database file, each from `0001_baseline.sql`.
The baseline in `panel/` creates each node's editable
configuration and canonical
revisions, the cached core archives, subscription state, shared subscription
node order, each node's runtime history, traffic samples, counter checkpoint
and durable daily traffic accounting, login sessions, the node registry with
node public keys, the hashes of unused enrollment tokens, each node's traffic quota and period, tags and its last state report,
and operational records directly. `logs/` builds the log database `logs.db`,
which holds the panel log apart from `panel.db` so that clearing it rewrites
only that file; the second migration in `panel/` moved the log out of
`panel.db`. Each node keeps its
runtime evidence in its own `node.db`, built by
`internal/sing-box-node/store/migrations/`. Settings live in the selected
`setting.json`; SQLite keeps only transaction markers for interrupted settings
updates.

Each store's migrations are files named `NNNN_name.sql`, numbered from 0001
without gaps and embedded in the binary. Opening a database applies the ones it
has not recorded in `migration_history`, in order, inside one transaction, and
sets `user_version` to the latest; an empty database receives the history table,
the application ID and every migration. A shipped migration is never edited: the
history keeps each migration's SHA-256 and a database whose recorded migrations
differ from the binary's is refused. A schema change adds the next file, rebuilding
a table with SQLite's create, copy, drop and rename sequence where `ALTER TABLE`
cannot express it. Databases without a migration history predate the baseline;
they, unknown formats and databases newer than the binary are rejected without
conversion.

Authenticated management, CSRF/origin controls, safe source acquisition,
immutable checked startup artifacts and exact core-version checks remain in place.
The panel verifies official archives against GitHub's size and digest before
caching them; the node verifies the archive it fetches against the identity in
the command and checks that the binary reports the exact version. Core enable
checks the node's platform before selection; runtime preflight checks the
selected file before replacing the current process. Settings and configuration
writes use optimistic concurrency. A backup holds the panel settings only.
Panel settings saves do not modify native inbound credentials.

The obsolete Web user, deployment and history components have been removed.
The CLI is limited to panel settings, panel and systemd service control, installation
maintenance, and binary version/update commands. Core versions and runtime,
subscriptions, logs, and metrics use the Web UI and authenticated HTTP API.
Configuration history, revision, diff and restore commands remain absent.
Rollback to the previous bundle is available only through the authenticated API.
Internal immutable evidence remains for checked startup and runtime recovery. Unknown native configuration fields and numeric
lexemes survive editing. Neither hide nor delete-publication actions delete an
underlying core inbound.

### Verification workflows

- `make check` runs Go tests and vet, Web lint/tests/build, OpenAPI coverage,
  protobuf lint and generated-code drift, shell syntax checks and installer
  scenarios.
- `make test-race` checks concurrent state changes and runtime controls.
- Store tests cover fresh-schema initialization, reopening the current format,
  rejection of unsupported formats without conversion, transactional saved-file
  writes, settings recovery and immutable runtime evidence.
- API tests exercise the single editable file across reopen, including whitespace,
  large numbers, unfinished JSON, stale writes and correction. Removed configuration
  history routes must return 404.
- `make release-smoke` checks packaged startup, persistence and signed self-update
  on a native Linux runner; it requires its documented prerequisites.
- Browser review covers responsive layouts, keyboard navigation, editor rendering,
  settings persistence, notifications and observed runtime controls against a
  running panel and node.

### Deliberate distinctions and validation limits

- An invalid **core configuration** can be saved as text, but cannot launch.
  An invalid **channel template** cannot be saved as an active template.
- Runtime loaded state is independent from saved configuration state. Saving
  during a run does not silently restart or reconfigure the process.
- The pool is independent from core execution: a node leaves it only when its
  origin stops yielding it, and a channel publishes only the nodes its groups name.
- Remote rule files are fetched by the subscribing client, never by the panel.
- A node's source name is independent from its user-entered name and protocol.
- Sing-box channel templates receive JSON syntax and reserved-field validation;
  Mihomo templates YAML syntax, reserved-field and supported-option validation. A
  live client runtime check has not been performed. Unsupported conversions return
  diagnostics.
- Native installed-core lifecycle checks require Linux. Host-independent tests
  verify preflight and concurrency controls with controlled executors.
- The build reports a large configuration-editor chunk. It remains lazily loaded
  with configuration editing; the warning is not a build failure.

Feedback follows completed operations and observed runtime state rather than
hardcoded samples.

## HTTP API and security

The HTTP server delivers the embedded Web application, the authenticated
management API, and token-authenticated subscription output. Its default
listener is loopback-only.

### API contract and routing

The management API is rooted at `/api/v1`. The authoritative operation,
schema, status-code, and problem-detail contract is
[`api/panel-openapi.yaml`](../api/panel-openapi.yaml); this guide describes its trust
boundaries without duplicating the endpoint inventory.

Every response includes `X-Request-ID`, matching `request_id` in problem responses.
The server preserves a single nonempty, valid UTF-8 incoming ID of at most 128
bytes. Missing, empty, oversized, invalid UTF-8 or repeated headers receive a
newly generated ID before routing;
the same ID is used in request context headers and both response representations.
Oversized IDs are replaced, never truncated or echoed back in an error.

`server.base_path` prefixes the Web application, management API, and `/sub`
routes. It must be empty or a normalized path without a trailing slash. Browser
code uses same-origin paths and depends only on the HTTP contract.

Configuration operations address one saved JSON document. Compilation binds an
immutable installed core artifact, not a naked version string. The server
checks the stored installation, safe executable file and exact reported version,
then asks that
binary to validate the immutable configuration bytes. JSON Schema and inbound
conversion availability are independent optional capabilities.

`GET /api/v1/nodes/{nodeId}/core/status` reports the configuration revision and binary
evidence associated with the live artifact or, when stopped, the applied
bundle. It never selects the newest catalog version or a nearby release.

### Management authentication

The shared settings file stores one normalized administrator email in `auth.email`
and an Argon2id hash in `auth.password_hash`. Legacy `auth.token` is rejected.
There is no registration, email verification, browser password recovery, or additional administrator.
Server operators recover access with `config set auth.password --stdin`; the login page has no recovery UI.
`POST /api/v1/auth/session` accepts `{ email, password }`; email is trimmed
and lowercased, while password characters are preserved. Address validation is
owned by the server; the login form accepts Unicode addresses supported by the
settings contract instead of applying native HTML email validation. New passwords contain
12–128 Unicode characters. Password hashing uses `alexedwards/argon2id` with
19 MiB memory, two iterations, parallelism one, a 16-byte salt and a 32-byte key.
Configured hashes are checked before computation (19–64 MiB, 2–4 iterations,
1–4 parallelism, 16–32-byte salt, 32-byte key). At most two password computations
run concurrently.

The HTTP client lazily loads the locally bundled FingerprintJS SDK (5.2.0, MIT)
and sends only its 32-character `visitorId` as `X-Client-Fingerprint` on login.
SDK monitoring is disabled; no fingerprint components or telemetry are sent to
Fingerprint or other third parties. Collection has a 1.5-second budget and the
result is cached only in page memory. SDK failures/timeouts omit the header.
Fingerprints are untrusted, spoofable abuse signals, never authentication factors;
missing fingerprints still receive all peer/global protections. Invalid or repeated
fingerprint headers return 400.

Before reading a login body or waiting for settings/password work, the server
atomically admits at most two concurrent logins, five attempts per direct peer
in a fixed one-minute window, and 30 attempts process-wide per minute. IPv4-mapped
addresses normalize to IPv4; IPv6 addresses share a /64 budget. A supplied
fingerprint also allows five attempts per minute across changing peers. Changing
or omitting the fingerprint cannot bypass the peer/global limits. Successful and
malformed admitted requests count; success does not reset budgets. Exhausted peers
and concurrency rejections do not consume the remaining global budget. Limits
return 429 with `Retry-After` in seconds. State is memory-only, bounded to 4096
keys without evicting active entries, expires automatically, and resets on restart.

Forwarded-IP headers are ignored, including behind a reverse proxy: users sharing
a direct proxy/NAT peer share its budget. `server.external_origin`, when set,
establishes the enforced origin and cookie scheme, not trust in forwarded client IPs. Login JSON
is bounded to 4 KiB; body reads, settings-lock waits and response writes share a
five-second deadline. The write deadline is armed before admission, covers rejected
responses too, and remains active through the server's buffered flush. On expiry,
the connection may close or the stream reset before a problem response is delivered.
HTTP/1 login responses close their connection so rejected requests cannot keep
it occupied draining an unread body. HTTP/2 reuses the connection with per-stream
read/write deadlines. These controls bound application authentication work; ingress
connection/traffic floods still require network or reverse-proxy controls.
The admission, limits and deadlines live in `internal/webauth`, and the node
console applies the same ones to its password login.

Management Bearer authentication is removed. Automation uses the same login endpoint
with JSON, retains the Cookie, then supplies `X-CSRF-Token` for writes; with
`server.external_origin` set, login and writes also send an `Origin` naming it. Login and `GET /api/v1/auth/session` return email, display name,
CSRF token and expiration. Subscription tokens retain their separate purpose.

SQLite stores only a SHA-256 digest of the random session identifier, CSRF token,
expiration and credential fingerprint. Each new login issues a persistent Cookie
with a fixed seven-day server lifetime, `Expires`, and `Max-Age`. All cookies use
HttpOnly, SameSite=Strict and the configured BasePath. Secure follows the active
external origin's HTTPS scheme, or actual request TLS when no external origin is
configured. Setting and clearing cookies use the same rule. Expiration does not slide. Startup and login
remove expired records. Logout deletes the record; login rotates the current browser's
session. Database/service errors return 503 and do not clear the browser Cookie.

The settings API accepts optional `credentials.email` and `credentials.new_password`
independently, without current-password verification. Empty passwords preserve the
current hash; empty email addresses are invalid. Password confirmation is not part of
the interface. Authenticated writes still require the existing session, CSRF token and, when configured, the external origin.

`config set auth.password --stdin` accepts a custom password and atomically
replaces only its hash in the existing settings file after validating the result,
without opening SQLite. Even the same supplied password receives a fresh salt and
invalidates prior session fingerprints. The command works while the service is stopped
or running; no restart is needed. Plaintext is neither persisted nor echoed in
command output. Initialization, CLI changes, and Web settings changes share the
Argon2id implementation; initialization also generates a random password.

Settings reads and session operations share the settings lock. Credential edits
through settings/restore revoke every session in the settings commit transaction.
Both responses clear the current Cookie immediately when credentials change.
Manual changes invalidate old credential fingerprints at the next authentication
boundary. Backups exclude sessions. Browser reload restores identity and CSRF
from the session endpoint. Only a confirmed 401 clears frontend authentication;
network/5xx errors preserve it, and writes are never replayed automatically.

The generated listener is `127.0.0.1:3000`. Before exposing the service beyond
loopback, place it behind a reviewed HTTPS reverse proxy. The panel checks the
`Origin` of logins and writes only when `server.external_origin` is set: it must
then equal that normalized public origin, and forwarded headers never change the
check. Without an external origin no `Origin` is checked, so a proxy works without
origin settings; the JSON-only login, the CSRF token and SameSite=Strict cookies
still stop cross-site requests. An HTTPS external origin also enables Secure
session cookies; origin changes take effect after restart. `Forwarded` and
`X-Forwarded-*` headers are never trusted. Administrator credentials and subscription tokens are separate trust boundaries.
The node endpoints under `/api/node/v1` are a third: they authenticate nodes by
their one-time enrollment tokens and Ed25519 proofs, never by management
sessions or CSRF tokens, accept connections only from loopback peers, and refuse
a browser's WebSocket from another origin.

The removed `auth.secure_cookie` setting and `service.secure_cookie` API field are
rejected, including inside backup settings. Existing files and backups require
manual removal of the obsolete field; new API clients must omit it. There is no
compatibility decoder or automatic migration. Backups are version 3 and hold
the panel settings only.

### Request and download protections

The authenticated filesystem browsing and resolution endpoints expose a node's
host path metadata within the node process's OS permissions and filesystem
namespace.
They do not read file contents, create or modify paths, or use a separate directory
allowlist. Responses are not cached; directory scans and page sizes are bounded.
`internal/sing-box-node/host` owns these read-only effects, with the node's
`service` use cases, its command executor and the panel's HTTP translation above
it. See the configuration guide's
[server path selection](configuration-and-runtime.md#selecting-server-paths).

The server applies bounded request bodies, strict JSON decoding where the
contract requires it, constant-time token comparison, security response
headers, and secret-redacting event metadata.

Network downloads use explicit host and resolved-address checks, bounded
responses, timeouts, and restricted redirects. Browser core import accepts one
bounded multipart file and stages it in a
mode-0700 private data-directory location rather than accepting a server-local
path. Archive verification rejects path traversal, symbolic links, non-regular
entries, duplicate binaries, excessive expansion, and non-canonical gzip/tar
input. See [Core versions](core-versions.md#checks-and-recorded-hashes)
for the artifact trust boundary.

Third-party subscription refresh validates DNS and every redirect destination
against the configured source-network policy. Public subscription requests use
only the stored pool and never fetch an upstream URL.

Keep the following data private:

- settings files and administrator passwords;
- exported sing-box configuration;
- subscription token plaintext and stored source content; and
- diagnostic files that may contain paths or operator-provided values.

Use file or stdin inputs for secrets instead of command arguments.

### Concurrency and immutable evidence

Configuration writes use revision preconditions. The editable file API accepts
`{ revision, content }` in the `PUT /api/v1/nodes/{nodeId}/config/file` JSON body. A stale
revision receives `412 Precondition Failed`
and must be reviewed rather than overwritten automatically. Channel, source, pool node and user-grant
mutations send the record's `updated_at` in `If-Match`; token mutations use
their lifecycle preconditions.

Compilation stores the raw JSON bytes together with the configuration revision,
core installation, exact version, configuration digest and recorded core hashes.
Apply and lifecycle operations revalidate configuration, safe files and exact
version; recorded core hashes are informational and are not compared against
the executable. Uploads have no expected `sha256` parameter. A candidate that
fails the selected binary's `sing-box check` never becomes ready.

Public subscription responses are rendered from one consistency read of the
pool, channel and key. Inbounds from the nodes' applied configurations join
the pool once nodes report those configurations to the panel.
User-bound keys additionally require an enabled user and exact grants;
independent keys use channel publication policies. Response bodies are not frozen
into activation bundles.

Management mutations return HTTP 200 after completion, with the resulting core
artifact, catalog summary, refreshed source, checked startup artifact or
runtime status. The browser keeps controls pending until that response and verifies
process identity for runtime changes. Errors use problem details and leave the
previous usable state intact where the operation has not committed.

Integrations consume the completed resource response directly. Runtime snapshots
are internal evidence; there is no separate configuration-history write or restore API.

### Web presentation boundary

HTML responses generate a unique style nonce, place it in the page metadata and
include it in `style-src`. CodeMirror uses that nonce for its generated styles.
HTML is served with `Cache-Control: no-store`; script policy remains self-only,
without unsafe-inline or unsafe-eval. Ordinary notifications dismiss after three
seconds (hover/focus pauses the accessible toast timer). A notification holds a
one-line title and at most two lines of reason, cut with an ellipsis and shown in
full on hover; a failure is titled by the action that failed.

The React application contains its own trusted structured controls. It does
not load Schema-provided scripts, components, templates, or remote resources.
Changing the selected artifact clears the previous schema state before
requesting the new artifact's schema.

The editable configuration response includes authoritative `content` text,
`revision` and `syntax_valid`. The Web editor uses a lossless codec, preserves
unshown fields, and saves the complete text with the current body revision.
The version selector contains compatible installed core versions and does not
enable or replace a core when its selection changes. A version without a release
line uses the Advanced editor and shows the reason. With no installed
version, Advanced JSON saving remains available but visual editing and binary
validation require the operator to install one.
A `412` response preserves the local draft for review.

See the [Web application reference](../web/README.md) for frontend ownership
and build behavior.
