# Repository architecture

This guide owns package boundaries, product requirements, and HTTP trust boundaries.
Operator workflows live in the [documentation index](README.md); exact HTTP
operations and schemas remain in [OpenAPI](../api/panel-openapi.yaml).

- [Repository layout](#repository-layout), [package layout](#package-layout) and [dependency direction](#dependency-direction)
- [Version support](#adding-a-sing-box-version) and [test ownership](#test-ownership)
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
| `tools/` | Maintainer commands run through `go tool`: sing-box support generation, release signing, third-party notices and OpenAPI verification |
| `scripts/` | Installer, isolated release build and native release and core contract tests |
| `docs/` | One guide per topic, indexed in [the documentation index](README.md) |
| `.github/` | Workflows, community and security policies, templates, code ownership and the release trust root |

Each Web application's Go package sits in its own directory under `web/apps/`
because Go embedding cannot read files from a parent directory. Other embedded assets live in the package that owns
them: subscription templates in `internal/sing-box-panel/subscription`,
the configuration field order in `internal/configuration`, each service's systemd
templates in `internal/sing-box-panel/systemd` and `internal/sing-box-node/systemd`, and Schema assets in
`internal/sing-box-panel/schema`. The Web editor reads the templates and
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
`tools/singbox-support` maintains the panel's Schema assets and inbound
conversion catalog and so imports panel packages; tools are programs that
nothing imports.

| Tree | Package | Responsibility |
| --- | --- | --- |
| Shared (`internal/`) | `buildinfo`, `console`, `jsonstrict` | Build identity and its display, terminal output, strict JSON decoding and canonical objects |
| | `atomicfile` | Crash-safe file publication and directory syncs for both services and the tools |
| | `release`, `selfupdate` | Release-version validation and signatures; download and atomic binary replacement |
| | `procctl` | Private local control of a running service without an OS service manager; the service lifetime and lease, never background processes |
| | `database` | SQLite opening, connection pragmas, baseline schema identity and data-directory locks shared by both stores |
| | `logredact` | Credential and configuration-body redaction for log text and metadata, applied by both services |
| | `configuration` | Strict, lossless sing-box JSON parsing, canonical serialization and the presentation field order |
| | `coreartifact` | Immutable identities of installable sing-box artifacts |
| | `singbox` | Reviewed exact-version support catalog and runtime compatibility; exact versions are catalog data, not forwarding packages |
| | `nodeapi` | Generated `sbp.node.v1` protocol code, the protocol version, the session proof and the enrollment code format |
| | `systemd` | One service's Linux systemd lifecycle over its definition: audited unit rendering, install, uninstall, status, control and logs, the dedicated system account and its ownership of the service's files |
| | `pathguard` | Symlink-resolving containment and refusal of roots, homes and shared system directories, for directories a service owns, cleans, moves or binds |
| | `webauth` | What both management APIs share: Argon2id password hashing, login admission with its rate limits and deadline, same-origin and CSRF checks, and security headers |
| Node (`internal/sing-box-node/`) | `cli` | The node's command tree: `enroll`, `run`, `console` and `systemd` |
| | `settings` | The node's data directory, its enrollment (panel endpoint, trust, IDs and the Ed25519 identity key) and its console settings |
| | `agent` | Enrollment, the link that holds it with the session's state, the authenticated session with the panel, with reconnection, and the command executor with its idempotent results; the node only dials out |
| | `httpapi` | The loopback console API: password login, the link's status, enrollment and unenrollment |
| | `systemd` | The node's unit templates and its data directory as the state of its systemd installation, which must hold an enrollment or an enabled console before it starts |
| | `store` | Node SQLite: received configuration revisions, installed cores, checked startup artifacts, activation bundles, runtime intents, observations and history, and command results |
| | `service` | The node's use cases: core installs, checks and activation, process supervision and recovery, core logs, filesystem selection and the public IP hint |
| | `artifacts` | Verification and publication of immutable installed core files |
| | `runtime` | Managed processes, reference-counted disposable configuration files and the restricted Clash API monitoring client |
| | `corelogs` | Bounded, private, sanitized native process output |
| | `host` | Host-wide metrics, the bounded public IP hint and read-only, non-recursive filesystem selection |
| Panel (`internal/sing-box-panel/`) | `store` | Panel SQLite: the baseline schema, each node's configuration and revisions, the cached core archives, subscription state and operational records |
| | `auth` | The administrator's email and login sessions |
| | `settings` | The panel settings file and its service: validation, defaults, atomic saves and recovery, change notification, backup export and restore |
| | `telemetry` | The panel log and operation outcomes, metrics and runtime history, each node's traffic periods and quota, the dashboard projection |
| | `corecache` | Upstream catalog discovery, staged core uploads and the verified archive cache that nodes install from |
| | `schema` | The exact-version Schema manifest and assets for the editor and validators |
| | `nodeconfig` | Each node's editable configuration file, canonical revisions, Schema support, preview and the exact-Schema check before a command |
| | `subscription` | Sources, manual nodes, inbound conversion, channels, keys, users, templates and renderers, preview and `/sub` delivery |
| | `fleet` | The node registry, enrollment codes, the TLS identity nodes pin, the node gRPC service sharing the Web port under the base path, authenticated node sessions, the typed commands sent over them and the short-lived grants under which nodes fetch cached archives |
| | `app` | The one composition of the panel's services over a store and a settings file, used by the server and the transport tests |
| | `httpapi` | The management API, Web delivery and subscription output over `app.Services` |
| | `installation` | Inspection, moves and removal of the panel's persistent files |
| | `systemd` | The panel's unit templates and its settings file as the state of its systemd installation: data directory, created settings, owned sidecars and pending data moves |
| | `cli`, `server` | The panel's command tree; the server lifecycle around the composed services, the node listener and the HTTP server |
| | `testutil` | Fixtures shared by panel tests |

The panel `store` imports no domain package; services above it validate domain
rules and the store keeps structural integrity and transactional invariants.
Transport packages translate input and output but do not own use-case rules.
Infrastructure packages own process, network, filesystem and database effects.
The web client depends on the documented HTTP contract rather than Go
implementation details.

## Dependency direction

Production imports between the module's packages, as reported by
`go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./internal/... ./cmd/... ./tools/...`.
`panel:` and `node:` abbreviate `internal/sing-box-panel/` and
`internal/sing-box-node/`:

```text
atomicfile, buildinfo, console, coreartifact, database, jsonstrict, pathguard,
procctl, release, singbox, webauth, node:host (no module imports)

configuration      -> jsonstrict
logredact          -> jsonstrict
nodeapi            -> jsonstrict
selfupdate         -> atomicfile, release
systemd            -> atomicfile, database, pathguard
node:artifacts     -> atomicfile, coreartifact
node:runtime       -> atomicfile, coreartifact, singbox
node:corelogs      -> logredact
node:store         -> configuration, coreartifact, database
node:service       -> configuration, coreartifact, database, jsonstrict,
                      node:artifacts, node:corelogs, node:host, node:runtime,
                      node:store
node:settings      -> atomicfile, jsonstrict, nodeapi, webauth
node:agent         -> buildinfo, coreartifact, nodeapi, procctl, node:artifacts,
                      node:corelogs, node:host, node:service, node:settings,
                      node:store
node:httpapi       -> buildinfo, jsonstrict, nodeapi, node:agent, node:host,
                      node:settings, webauth
node:systemd       -> node:settings, systemd
node:cli           -> buildinfo, nodeapi, node:agent, node:httpapi,
                      node:settings, node:systemd, systemd, web/apps/node,
                      webauth

panel:store        -> configuration, database, jsonstrict, logredact
panel:auth         -> panel:store, webauth
panel:settings     -> atomicfile, database, jsonstrict, nodeapi, panel:auth,
                      panel:store, webauth
panel:telemetry    -> database, panel:settings, panel:store
panel:schema       -> coreartifact, singbox
panel:corecache    -> atomicfile, coreartifact, jsonstrict, panel:settings,
                      panel:store, panel:telemetry
panel:nodeconfig   -> configuration, panel:corecache, panel:schema, panel:store
panel:subscription -> configuration, coreartifact, database, jsonstrict,
                      panel:schema, panel:settings, panel:store,
                      panel:telemetry, singbox
panel:fleet        -> atomicfile, buildinfo, configuration, database, nodeapi,
                      panel:corecache, panel:nodeconfig, panel:settings,
                      panel:store, panel:telemetry
panel:app          -> buildinfo, panel:auth, panel:corecache, panel:fleet,
                      panel:nodeconfig, panel:settings, panel:store,
                      panel:subscription, panel:telemetry
panel:httpapi      -> buildinfo, configuration, coreartifact, jsonstrict,
                      panel:app, panel:auth, panel:corecache, panel:fleet,
                      panel:nodeconfig, panel:schema, panel:settings,
                      panel:store, panel:subscription, panel:telemetry, webauth
panel:installation -> database, jsonstrict, pathguard, procctl, panel:fleet,
                      panel:settings, panel:store
panel:systemd      -> panel:installation, panel:settings, systemd
panel:cli          -> buildinfo, console, pathguard, procctl, selfupdate,
                      panel:app, panel:fleet, panel:installation,
                      panel:settings, panel:store, panel:systemd, systemd,
                      webauth
panel:server       -> buildinfo, console, procctl, panel:app, panel:corecache,
                      panel:fleet, panel:httpapi, panel:installation,
                      panel:settings, panel:store, panel:subscription,
                      panel:telemetry

cmd/sing-box-panel        -> buildinfo, panel:cli, panel:server, selfupdate, web/apps/panel
cmd/sing-box-node         -> buildinfo, node:cli
tools/singbox-support     -> atomicfile, coreartifact, panel:schema,
                             panel:subscription, singbox
tools/third-party-notices -> atomicfile
tools/sign-release        -> release
```

Settings sit below the services that read them: `telemetry` reads the traffic
sample retention and `corecache` reads the catalog token and
refresh interval from `settings` directly, and `settings` depends on no other
service. The HTTP route that restores a panel backup records its operation
outcome.

`internal/layout` enforces the layout over `go list`: `cmd/` holds only the two
`main` packages, shared packages depend on neither service, neither service
imports the other's packages, and `sing-box-node` neither depends on the panel
nor embeds the panel's Web application.
`configuration` imports neither `singbox` nor the panel Schema. The panel `cli`
opens `store`, `settings` and `installation` directly for local maintenance,
manages nodes through `app`, and starts the server through a function supplied
by `cmd/sing-box-panel`.

## Adding a sing-box version

Follow [Maintaining support](core-versions.md#maintaining-support)
for exact-version review, source and artifact pins, Schema generation and native
Linux verification. Runtime eligibility, configuration Schema and inbound
conversion are separate capabilities; unreviewed versions never inherit nearby
capabilities.

Do not add exact-version forwarding directories. Version identity belongs in
the catalog; reusable behavior belongs in private family functions of the
package that owns the capability.

## Test ownership

- Go unit, integration, fuzz, and package contract tests live beside their
  production package. Shared test helpers remain in that package's
  `test_helpers_test.go`.
- sing-box behavior differences use catalog- or family-driven table tests in
  `internal/singbox` and the panel `schema` and `subscription` packages. The
  real-binary contract is `internal/sing-box-panel/schema/core_contract_test.go`
  and runs in dedicated native Linux CI jobs.
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

The screens are login, the fleet overview, the node grid and, for each node, its
overview, installed/available versions, visual and JSON configuration, core
logs and settings; subscriptions with sources, manual nodes, source detail,
subscription keys, channels and channel detail; panel logs; and panel settings.
Detail tabs and feedback are local states, not duplicate screens.

### Requirement → screen → contract → implementation → verification

All rows below describe the current implementation. The verification workflows
below exercise the corresponding source, API and runtime boundaries.

| ID | Screen and accepted behavior | Contract / implementation boundary | Verification coverage | Status |
| --- | --- | --- | --- | --- |
| UI-01 | App shell with the layout and shell parameters of [`DESIGN.md`](../DESIGN.md); no global header or page-title band; the rail lists overview, nodes, subscriptions, logs and settings; a calm glyph field fills the canvas behind the rail and workspace and shows faintly through the translucent cards and rail, while fields and floating layers stay opaque; empty states show a title and the next step, and explanations sit in help tooltips; content fills the remaining height and scrolls with overlay scrollbars | App shell, responsive layout, shared controls | Desktop and narrow viewports, keyboard, internal scrolling without clipped controls | implemented |
| FLEET-00 | Fleet overview, opened after sign-in: online nodes, running cores, live traffic and this period's summed usage (never a summed quota); a nodes table with state, core, CPU, memory, traffic and period usage against each node's own quota; what needs attention; recent panel events; subscription counts; each region links to its view and the page refreshes every 5 seconds while visible | `/fleet/status` (every node with its metrics), panel logs and subscription list APIs | `fleet` status test with ten-second rates, fleet summary tests, route test | implemented |
| FLEET-01 | The nodes view as a searchable card grid read from the fleet status, scrolling under a fixed toolbar row that filters by state with each state's count, searches and adds a node; nodes that need attention sort first: each card shows a health dot for the node's worst condition, name, host, the core's state and version while online or a state badge when offline or not enrolled, live rates and connections, CPU, memory and disk, and what is left of its own quota this period, with their details on hover; an offline card keeps when it was last seen and what is left of its quota, and a node not yet enrolled shows its one-time enrollment code from its card, as adding a node does | Fleet status and node creation APIs | Route tests; empty, no-match and load-failure states | implemented |
| FLEET-02 | Opening a node shows its object row (back, name, state, version, uptime, rates, runtime controls) and tabs for overview, core versions, configuration, logs and settings; every view inside reads and commands that node only; an offline or not enrolled node keeps its last report marked stale and shows one region state in tabs that command it | Node-scoped API client, node routes | Route tests for node scoping and offline state; object row runtime tests | implemented |
| FLEET-03 | A node's settings tab saves its name, published host and traffic quota, period and reset day as one group, and keeps re-enrollment and removal in a separate destructive section that names the node | Node rename, settings, enrollment and deletion APIs with settings revisions | Settings validation and conflict tests; browser save, re-enroll and remove | implemented |
| UI-02 | Shared action buttons in the sizes and variants of [`DESIGN.md`](../DESIGN.md), with matching square icon buttons and 44px touch targets; primary save, outlined secondary actions and semantic destructive actions; consistent hover/pressed/focus; centered dialogs; compact semantic-color Toast for all operation, loading and validation errors, without inserting error banners into page flow (sign-in prints its refusal as the prompt's output row instead); repeated persistent failures reuse one notification and recovery closes it; all dropdown fields use the shared custom Select, including protocol, source and channel forms, with consistent popup styling and keyboard interaction | Shared UI and feedback | Real actions, focus return, Escape, visible validation, disabled options, no false success | implemented |
| UI-03 | Data tables and item lists use regular-weight names, body text, statuses and row actions; table headers use medium weight; dropdown options use regular weight, with existing compact metadata sizes preserved | Shared table typography and feature-owned list styles | Browser checks across versions, sources, keys, channels, configuration rows and logs | implemented |
| AUTH-01 | Email/password login as a terminal prompt (`login:` and `password:` lines) with accessible labels, password visibility, an inline refusal row, fixed seven-day persistence and reduced-motion support | Existing authenticated session, CSRF/origin/rate-limit boundaries | Login/logout/error tests, credentials never exposed | implemented |
| RUN-01 | The node's object row reports running/stopped/unknown accurately; start, stop and restart act on that node, with confirmation for stop and restart; theme/language/logout in the rail footer share the same button and icon sizes; keyboard focus remains visible; on narrow screens the context values scroll sideways on their own line | Runtime observation and lifecycle, node object row | Unknown/stale never reported running; check failure keeps current process | implemented |
| DASH-01 | CPU/memory summary, transfer graph with 1h/24h inset tabs matching version management, and one-hour active connections graph; compact plot margins and external axes/units; rolling windows put the newest bucket at the right edge and remove expired samples on refresh without rebuilding the chart; no refresh/footer/extra core metric strip | Metrics history and real push updates | Real samples, reconnect, missing data, rolling time window, keyboard range switching | implemented |
| DASH-02 | 24h runtime history in 48 equal segments; running/failed/stopped/unknown with legend, no yellow state | Persistent runtime history | Restart persistence and unknown periods | implemented |
| CORE-01 | Installed/available fill-height lists, inline enable/download with persistent selection; source and capabilities retained; 5/10/50 pagination | Catalog, artifacts, runtime switch | Paging and scrolling; invalid switch retains runtime; artifact verification | implemented |
| CORE-02 | Filter assets by deployed OS/CPU architecture; no architecture badge or selector, never browser architecture or unknown→ARM64 | Platform detection, catalog/cache, imported artifact inspection | Cross-architecture assets and rate-limit/cache behavior | implemented |
| CFG-01 | One editable config.json, version-scoped root sections (13 in 1.14) with flat label/control rows, DNS/route sub-tabs and inline list editing; advanced JSON; floating icon tools and find/replace with case, word and regex matching; preserve unknown fields and numbers | Canonical saved text, version-scoped schema, validated startup artifacts | Lossless round trip; invalid JSON can save, cannot start | implemented |
| CFG-02 | Save independent of running process; loaded/dirty/restart-required/start-pending states; start/restart checks saved file before replacement | Saved-file identity vs loaded immutable artifact; CAS and runtime transitions | Concurrent edits, failed validation preserves running process, restart loads saved bytes | implemented |
| SUB-01 | Sources/keys/channels only; source and channel names are display-only; toolbar refresh then add; URL-only source creation; source edit/refresh row actions without deletion; stable source-detail navigation across node/settings tabs | Source API, SSRF-safe acquisition and source versions | Source creation/edit/refresh failure retains last success; no single-node source path | implemented |
| NODE-01 | Local publishable inbounds and manual nodes in one manual collection; real name, actual source badge, system/imported badge after the manual collection badge, protocol/endpoint/TLS/Reality/SNI; configuration opens only from the corner details action; hidden cards omit the corner visibility icon and restore visibility through their clickable overlay | Node catalog/provenance and inbound-to-client conversion | No automatic core outbound/TUN/redirect/tproxy publishing; no invented prefixes; card clicks do not open configuration; hidden overlays restore visibility | implemented |
| NODE-02 | Eye controls visibility; hidden cards remain recoverable at source; channel filters before pagination without deleting membership/order | Independent visibility and channel membership | Hide/restore selected node, empty/page bounds, no ghost cards | implemented |
| NODE-03 | Native conditional protocol editor and node-scoped advanced JSON; parse share link then confirm; no unknown-field loss | Protocol capability/schema and references | TLS/QUIC exclusions, HY2 port modes, SS UoT/mux, SSH auth, detour cycles, version support | implemented |
| NODE-04 | Listener vs published addresses; credentials visible/copyable; local edit links to configuration; hiding/deleting publication cannot delete inbound | Local-node provenance and configuration deep links | IPv6, unknown public IP, no listener/SNI/port mutation, real copy results | implemented |
| KEY-01 | Subscription keys replace user UI; expiry and successful-download quota shared across all channels; failed requests do not consume | Token storage and atomic delivery accounting; user-scoped access checks | Concurrent quota requests, expiry, failures, existing grants preserved | implemented |
| CHAN-01 | Continuous node grid, actual source badges; nodes/rules tabs; output/distribution/preview toolbar | Channel selected nodes and native output formats | Selection/order/hidden filtering and format change | implemented |
| CHAN-02 | Channel creation and editing open the strategy-group workspace directly; nodes are selected within each group; back/settings/preview/save actions grouped at the right; channel name and output client appear only in a cancelable settings draft, with no separate identity row; lightweight split workspace with independently scrolling group list and editor; group node/rule counts, card-level pencil actions with cancelable name dialogs; sidebar final-exit toggle beside delete; implicitly enabled groups; optional, exclusive final-exit badges; searchable candidate cards; automatic candidate exits, manual domain/IP/CIDR and remote references; rule dialogs omit exit and enablement fields; unmatched fallback | Typed channel configuration, native renderers | New group selects current eligible candidates; filtered selection preserves other members; edit/additional nodes preserve choice; valid exits | implemented |
| RULE-01 | Remote metadata only, client fetches; Mihomo YAML/YML/TEXT/MRS + behavior; MRS excludes classical; sing-box source JSON/binary SRS | Remote-reference validation and native output | No server content fetch/upload/conversion; incompatible format cleared and blocked without URL loss | implemented |
| RULE-02 | Group context inherited; smaller child modal returns to correct parent; missing format red border only; anchored menus | Modal state, accessible invalid controls | Nested save/cancel/focus; short viewport sticky actions and complete menus | implemented |
| RULE-03 | Single-line full URL; lightning toggles gh-proxy.com for eligible GitHub/raw/gist; unwrap known proxy, no duplicates; off restores origin | URL normalization and effective render URL | Empty/non-GitHub disabled; preview/delivery uses effective URL; no credential forwarding | implemented |
| RULE-04 | A group's rules switch between entries and Clash classic text; a pasted Clash rule list converts, leaving out policy names and `no-resolve` with a note; problems are listed by line and block switching back and saving while the text stays; matching lines keep their rule's identity, exit, state and the group's indices | Channel policy with stable rule IDs and indices | Unchanged round trip; per-line errors; refused save keeps the text | implemented |
| CHAN-03 | Distribution: name/client/template/new-node policy; direct node editing and card order; global rule indices | Channel data separation | Preview and delivery share defaults; card/index order preserved | implemented |
| TPL-01 | Per-channel native JSON/YAML template; true edit/dirty/validate/location/preview/save/cancel; generated nodes/auth/groups/rules/fallback reserved | New template storage/API/merge/validation; no shared library or DSL | Reject conflicts and invalid save; preserve other channel; preview equals delivery | implemented |
| SET-01 | Six grouped categories; shared draft and save, same-path hash navigation, cross-category validation, backup preview and atomic restore | Panel settings API/storage/bootstrap separation | Authenticated update, optimistic concurrency, invalid inputs, secrets redacted | implemented |
| SET-02 | Listen/address/domain, administrator credentials and optional GitHub token; independent email/new-password drafts, shared save; server-only secrets | Bootstrap/security settings and catalog client | Restart semantics, session invalidation, no token in response/log/browser persistence | implemented |
| SET-03 | Each node's public host override, else the public IP it reports; hidden subscription source access values preserved; inbound credentials edited natively; protocol obfuscation stays protocol-specific | Node settings and native configuration | IPv4/IPv6/domain, a node without a host publishes nothing, loopback never exported, imported credentials unchanged | implemented |
| SET-04 | Language, theme, five presets/custom color picker with HEX and explicit apply/cancel after closing; default #6D4ED1, radius 0–32 default12 | Persisted preferences and preview transaction | Save/reload; unsaved category/route changes require confirmation; cancellation retains draft, confirmed departure restores saved; reset only color/radius | implemented |
| SET-05 | Theme links accent/background/border/focus/charts through the derivation and radius steps in [`DESIGN.md`](../DESIGN.md); badges/logo/status independent | Shared CSS tokens and accessible color derivation | Text contrast ≥4.5 for arbitrary light/dark colors, radius bounds, all component states | implemented |
| LOG-01 | A node's logs tab shows its real-time core raw logs only; timestamp muted, full message matches level; all native levels, file/date/level selectors consistent and right aligned; one top-right status pill toggles live output and pause, without a separate icon or surrounding container | Core log file/tail API | Real file reading/stream/reconnect/pause/filter, independent scroll | implemented |
| LOG-02 | The panel's logs view shows operation outcomes and runtime events across the fleet; time, message, log level and source only | Unified event query with sanitized operation outcomes | Consistent displayed/filter levels; pagination/filter | implemented |

### Implementation and evidence map

| Requirements | Owning implementation and guide | Focused evidence |
| --- | --- | --- |
| UI-01/02, AUTH-01, RUN-01, FLEET-00–03 | `components/app-shell`, `components/backdrop`, `components/node-workspace`, `pages/overview-page`, `utils/fleet-summary.ts`, `fleet/state.go`, `pages/fleet-page`, `pages/node-settings-page`, shared `components/ui`, `pages/login-page`; existing HTTP session and origin protections | App-shell, route and login tests; 18 desktop layout cases; real centered dialogs, Toast, focus and return journeys |
| DASH-01/02 | `pages/node-overview-page`, `internal/sing-box-node/host`, node `service/traffic.go` and `agent/reports.go`, `fleet/state.go`, `telemetry/nodes.go`, `httpapi/metrics_stream.go`, replicated runtime transitions; [observability guide](subscriptions-and-observability.md) | Node traffic accounting test, `fleet` state report, history and telemetry control test, per-node traffic store tests, chart and timeline tests, browser missing-data and layout review |
| CORE-01/02 | `pages/cores-page`, `fleet` core commands, `corecache` archive cache, `internal/sing-box-node/service` core commands and runtime preflight; [core guide](core-versions.md) | `fleet` install and grant tests, node artifact verification and runtime tests; inline action and platform UI tests |
| CFG-01/02 | `nodeconfig/file.go`, `store/configuration_file.go`, `internal/sing-box-node/service/configuration_runtime.go`, visual/JSON editors; [configuration guide](configuration-and-runtime.md) | File CAS and lossless tests; node `configuration_runtime_test.go`; real HTTP browser invalid-text save/reload; schema form union/unknown-field round trips |
| SUB-01, NODE-01–04 | Source/node services, `subscription/inbound_convert.go`, `subscription/manual_node.go`, node editor/grid | Source refresh tests; `httpapi/subscription_nodes_test.go`, `manual_node_validation_test.go`, 1.14 inbound/TLS tests; browser hide/restore and conditional node form |
| KEY-01 | Store key accounting, public delivery and key panel; [subscription guide](subscriptions-and-observability.md) | 24-request concurrency tests, cross-channel quota HTTP tests, expiry/revocation/304/failure accounting, explicit secret-read authorization, persistence and UI tests |
| CHAN-01–03, RULE-01–04, TPL-01 | `subscription/channel_*`, channel policy API, channel workspace and nested editors, `pages/subscriptions-page/channel-rule-text.ts` | Native renderer and URL tests; `httpapi/subscription_channel_policy_test.go`; rule text and workspace text-view tests; browser persisted group/reference reload, nested return, preview/copy and source visibility |
| SET-01–05 | `settings/service_panel.go`, `service_backup.go`, `store/panel_settings.go`, `pages/panel-settings-page`, `theme/appearance.ts` | CAS/redaction/validation/atomic backup tests; appearance contrast/bounds tests; browser preview, category retention, save/reload, leaving restores saved appearance |
| LOG-01/02 | `internal/sing-box-node/corelogs`, runtime output observer, `store/panel_logs.go`, product log API, `pages/node-logs-page` and `pages/panel-logs-page` | File/rotation/custom-output tests; operation logs/cursors and authenticated file API tests; frontend pause/filter and log rendering tests |

Go paths in this table are relative to `internal/sing-box-panel/` unless they
name another tree; Web paths are relative to `web/apps/panel/src/`. API details remain in `api/panel-openapi.yaml`; the owning guides describe
persistence, security boundaries and runtime semantics.

### Persistence and preserved guarantees

`internal/sing-box-panel/store/schema.sql` is the panel's storage baseline,
schema version 22. It creates each node's editable configuration and canonical
revisions, the cached core archives, subscription state, shared subscription
node order, each node's runtime history, traffic samples, counter checkpoint
and durable daily traffic accounting, login sessions, the node registry with
enrollment token hashes, node public keys, each node's traffic quota and period, tags and its last state report,
and operational records directly. Each node keeps its
runtime evidence in its own `node.db`, defined in
`internal/sing-box-node/store/schema.sql`. Settings live in the selected
`setting.json`; SQLite keeps only transaction markers for interrupted settings
updates. Earlier, unknown and newer storage formats are rejected without
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
  protobuf lint and generated-code drift, generated support checks, shell
  syntax checks and installer scenarios.
- `make test-race` checks concurrent state changes and runtime controls.
- Store tests cover fresh-schema initialization, reopening the current format,
  rejection of unsupported formats without conversion, transactional saved-file
  writes, settings recovery and immutable runtime evidence.
- API tests exercise the single editable file across reopen, including whitespace,
  large numbers, unfinished JSON, stale writes and correction. Removed configuration
  history routes must return 404.
- `make core-contract` checks reviewed musl artifacts on native Linux amd64/arm64.
  `make release-smoke` checks packaged startup, persistence and signed self-update
  on a native Linux runner; both require their documented prerequisites.
- Browser review covers responsive layouts, keyboard navigation, editor rendering,
  settings persistence, notifications and observed runtime controls against a
  running panel and node.

### Deliberate distinctions and validation limits

- An invalid **core configuration** can be saved as text, but cannot launch.
  An invalid **channel template** cannot be saved as an active template.
- Runtime loaded state is independent from saved configuration state. Saving
  during a run does not silently restart or reconfigure the process.
- Source visibility is independent from channel membership and core execution.
  Hidden nodes are filtered before channel pagination and publication.
- Remote rule files are fetched by the subscribing client, never by the panel.
- A node's source name is independent from its user-entered name and protocol.
- Native sing-box channels use the reviewed 1.14 schema. Mihomo templates receive
  YAML syntax, reserved-field and supported-option validation; a live Mihomo
  runtime check has not been performed. Unsupported conversions return diagnostics.
- Native installed-core lifecycle checks require Linux. Host-independent tests
  verify preflight and concurrency controls with controlled executors.
- The build reports a large precompiled schema-validator chunk. It remains lazily
  loaded with configuration editing; the warning is not a build failure.

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

`GET /api/v1/system/status` reports the configuration revision and binary
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
a direct proxy/NAT peer share its budget. `server.external_origin` establishes the
trusted origin and cookie scheme, not trust in forwarded client IPs. Login JSON
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
with JSON and an `Origin`, retains the Cookie, then supplies `X-CSRF-Token` and
`Origin` for writes. Login and `GET /api/v1/auth/session` return email, display name,
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
the interface. Authenticated writes still require the existing session, Origin and CSRF.

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
loopback, place it behind a reviewed HTTPS reverse proxy and set
`server.external_origin` to its normalized public origin. HTTPS automatically
enables Secure session cookies; origin changes take effect after restart. CSRF origin
checks use this explicit value and never trust `Forwarded` or
`X-Forwarded-*` headers. Administrator credentials and subscription tokens are separate trust boundaries.

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
only persisted successful source versions and never fetch an upstream URL.

Keep the following data private:

- settings files and administrator passwords;
- exported sing-box configuration;
- subscription token plaintext and raw source versions; and
- diagnostic files that may contain paths or operator-provided values.

Use file or stdin inputs for secrets instead of command arguments.

### Concurrency and immutable evidence

Configuration writes use revision preconditions. The editable file API accepts
`{ revision, content }` in the `PUT /api/v1/nodes/{nodeId}/config/file` JSON body. A stale
revision receives `412 Precondition Failed`
and must be reviewed rather than overwritten automatically. Channel, source,
user-grant, and token mutations use their documented compare-and-swap or
lifecycle preconditions.

Compilation stores the raw JSON bytes together with the configuration revision,
core installation, exact version, configuration digest and recorded core hashes.
Apply and lifecycle operations revalidate configuration, safe files and exact
version; recorded core hashes are informational and are not compared against
the executable. Uploads have no expected `sha256` parameter. A candidate that
fails the selected binary's `sing-box check` never becomes ready.

Public subscription responses are rendered from one consistency read of the
current enabled source versions, manual nodes, channel and key. Inbounds from
the nodes' applied configurations join them once nodes report those
configurations to the panel.
User-bound keys additionally require an enabled user and exact grants;
independent keys use channel publication policies. Response bodies are not frozen
into activation bundles.

Management mutations return HTTP 200 after completion, with the resulting core
artifact, catalog summary, refreshed source version, checked startup artifact or
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
seconds (hover/focus pauses the accessible toast timer).

The React application contains its own trusted structured controls. It does
not load Schema-provided scripts, components, templates, or remote resources.
Changing the selected artifact clears the previous Schema state before
resolving the new exact version.

The editable configuration response includes authoritative `content` text,
`revision` and `syntax_valid`. The Web editor uses a lossless codec, preserves
unshown fields, and saves the complete text with the current body revision.
The version selector contains compatible installed core versions and does not
enable or replace a core when its selection changes. Exact versions without a
committed native or reviewed Schema use the Advanced editor and show the reason. With no installed
version, Advanced JSON saving remains available but visual editing and binary
validation require the operator to install one.
A `412` response preserves the local draft for review.

See the [Web application reference](../web/README.md) for frontend ownership
and build behavior.
