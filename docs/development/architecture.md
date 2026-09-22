# Repository architecture

This guide owns package boundaries, product requirements, and HTTP trust boundaries.
Operator workflows live in the [documentation index](../README.md); exact HTTP
operations and schemas remain in [OpenAPI](../../api/openapi.yaml).

- [Dependency direction](#dependency-direction) and [package ownership](#domain-package-layout)
- [Version support](#adding-a-sing-box-version) and [test ownership](#test-ownership)
- [Product contract](#product-contract)
- [HTTP API and security](#http-api-and-security)

## Dependency direction

The backend keeps one main package per domain and introduces another package
only for a concrete dependency or side-effect boundary:

```text
configuration --\
subscription ---+--> singbox --> application --> cli / httpapi / server
coreartifact ---/

catalog / artifactstore / runtime / store --> application / server
release --> selfupdate / release commands
```

`configuration` and `subscription` never import `singbox`. Transport packages
translate input and output but do not own use-case rules. `application`
composes use cases and stable package contracts. Infrastructure packages own
process, network, filesystem, and database effects. The web client depends on
the documented HTTP contract rather than Go implementation details.

## Domain package layout

A cohesive domain stays in one package and uses file prefixes to make ownership
visible. File length alone is not a reason to create another package.

- `internal/settings` owns the shared panel settings file, validation, defaults,
  atomic replacement and writer locking. `application` owns recovery when a
  Web save also updates sing-box protocol identity.
- `internal/configuration` owns strict, lossless sing-box JSON parsing and
  canonical serialization. `store` retains immutable
  snapshots as runtime evidence and initializes the current `schema.sql` directly.
- `internal/subscription` owns documents, normalized nodes, source parsing and
  fetching, rendering, and inbound conversion contracts. Files use
  `document_*`, `node_*`, `source_*`, `render_*`, and `inbound_*` prefixes.
- `internal/singbox` owns the reviewed support catalog, version-scoped native and reviewed
  Schema assets, inbound conversion, and behavior-family dispatch. Exact
  versions exist as catalog data rather than forwarding packages.
- `internal/runtime` owns managed processes and its restricted Clash API
  monitoring client.
- `internal/application` owns use cases and runtime identity resolution backed
  by persistent state.
- `internal/server` owns server composition, serialized runtime controls, bounded recovery,
  and periodic subscription refresh.
- `internal/panelprocess` owns private local process control; it reuses the
  server's lifetime and lease and does not launch background processes.
- `internal/installation` inventories and cleans one selected instance's
  persistent paths. Database-directory locks in `internal/store` exclude
  cleanup while the panel or another CLI command owns a database connection.
- `internal/release` owns release-version validation and signatures;
  `internal/selfupdate` remains the download and atomic-replacement boundary.

Packages such as `store`, `catalog`, `artifactstore`, `coreartifact`, and
`runtime` remain separate because they represent durable dependency or
side-effect boundaries. The top-level `systemd` resource package remains
separate because Go embedding cannot read files from a parent directory.

## Adding a sing-box version

Follow [Maintaining support](../guides/core-versions.md#maintaining-support)
for exact-version review, source and artifact pins, Schema generation and native
Linux verification. Runtime eligibility, configuration Schema and inbound
conversion are separate capabilities; unreviewed versions never inherit nearby
capabilities.

Do not add exact-version forwarding directories. Version identity belongs in
the catalog; reusable behavior belongs in private `singbox` family functions.

## Test ownership

- Go unit, integration, fuzz, and package contract tests live beside their
  production package. Shared test helpers remain in that package's
  `test_helpers_test.go`.
- sing-box behavior differences use catalog- or family-driven table tests in
  `internal/singbox`. The real-binary contract is
  `internal/singbox/core_contract_test.go` and runs in dedicated native Linux
  CI jobs.
- React tests mirror the `web/src` ownership structure. API client tests are
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

### Accepted baseline and precedence

The accepted design is [sing-box-panel in Figma](https://www.figma.com/design/ppQe6qWogFBlM2gEhxAoEh/sing-box-panel).
The final review contains 15 formal screens, 402 components and 46 local overlays.
Earlier references to 16 screens, three log tabs, six settings categories, user
management, draft/history workflows or generic node-origin badges are superseded.
Development storage has
no automatic upgrade path. Authentication, checked configuration snapshots and
panel release signatures remain enforced; the core installation trust boundary
is described in [Core versions](../guides/core-versions.md#checks-and-recorded-hashes).

The formal screens are login, dashboard, installed/available versions, sources,
manual nodes, source detail, subscription keys, channels, channel detail, visual
configuration, JSON configuration, panel settings, core logs and panel logs.
Detail tabs and feedback are local states, not duplicate screens.

### Requirement → screen → contract → implementation → verification

All rows below describe the current implementation. The verification workflows
below exercise the corresponding source, API and runtime boundaries.

| ID | Screen and accepted behavior | Contract / implementation boundary | Verification coverage | Status |
| --- | --- | --- | --- | --- |
| UI-01 | Floating 248px sidebar, 64px header; no visible page-title band; desktop content starts 16px below the header and fills the remaining height with its bottom aligned to the sidebar; six navigation entries in dashboard/version/subscription/configuration/panel/log order | App shell, responsive layout, shared controls | Desktop and narrow viewports, keyboard, internal scrolling without clipped controls | implemented |
| UI-02 | Shared 40px standard and 32px compact action buttons with 16px icons, matching square icon buttons and at least 44px touch targets; primary save, outlined secondary actions and semantic destructive actions; consistent hover/pressed/focus; centered dialogs; compact semantic-color Toast for all operation, loading and validation errors, without inserting error banners into page flow; repeated persistent failures reuse one notification and recovery closes it; all dropdown fields use the shared custom Select, including protocol, source and channel forms, with consistent popup styling and keyboard interaction | Shared UI and feedback | Real actions, focus return, Escape, visible validation, disabled options, no false success | implemented |
| UI-03 | Data tables and item lists use regular-weight names, body text, statuses and row actions; table headers use medium weight; dropdown options use regular weight, with existing compact metadata sizes preserved | Shared table typography and feature-owned list styles | Browser checks across versions, sources, keys, channels, configuration rows and logs | implemented |
| AUTH-01 | Compact token login; remove redundant brand/label/help; accessible input remains | Existing authenticated session, CSRF/origin/rate-limit boundaries | Login/logout/error tests, credentials never exposed | implemented |
| RUN-01 | One status badge: demo fixed, real running/stopped/failed accurately; transparent start/stop/restart icon buttons with clear spacing; theme/language/logout in sidebar footer share the same button and icon sizes without decorative borders or fills; keyboard focus remains visible; narrow header keeps uptime and transfer rates in one row of compact pills | Runtime observation and lifecycle, shared header | Unknown/stale never reported running; check failure keeps current process | implemented |
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
| NODE-04 | Listener vs published addresses; credentials masked/copyable; local edit links to configuration; hiding/deleting publication cannot delete inbound | Local-node provenance and configuration deep links | IPv6, unknown public IP, no listener/SNI/port mutation, real copy results | implemented |
| KEY-01 | Subscription keys replace user UI; expiry and successful-download quota shared across all channels; failed requests do not consume | Token storage and atomic delivery accounting; user-scoped access checks | Concurrent quota requests, expiry, failures, existing grants preserved | implemented |
| CHAN-01 | Continuous node grid, actual source badges; nodes/rules tabs; output/distribution/preview toolbar | Channel selected nodes and native output formats | Selection/order/hidden filtering and format change | implemented |
| CHAN-02 | Channel creation and editing open the strategy-group workspace directly; nodes are selected within each group; back/settings/preview/save actions grouped at the right; channel name and output client appear only in a cancelable settings draft, with no separate identity row; lightweight split workspace with independently scrolling group list and editor; group node/rule counts, card-level pencil actions with cancelable name dialogs; sidebar final-exit toggle beside delete; implicitly enabled groups; optional, exclusive final-exit badges; searchable candidate cards; automatic candidate exits, manual domain/IP/CIDR and remote references; rule dialogs omit exit and enablement fields; unmatched fallback | Typed channel configuration, native renderers | New group selects current eligible candidates; filtered selection preserves other members; edit/additional nodes preserve choice; valid exits | implemented |
| RULE-01 | Remote metadata only, client fetches; Mihomo YAML/YML/TEXT/MRS + behavior; MRS excludes classical; sing-box source JSON/binary SRS | Remote-reference validation and native output | No server content fetch/upload/conversion; incompatible format cleared and blocked without URL loss | implemented |
| RULE-02 | Group context inherited; smaller child modal returns to correct parent; missing format red border only; anchored menus | Modal state, accessible invalid controls | Nested save/cancel/focus; short viewport sticky actions and complete menus | implemented |
| RULE-03 | Single-line full URL; lightning toggles gh-proxy.com for eligible GitHub/raw/gist; unwrap known proxy, no duplicates; off restores origin | URL normalization and effective render URL | Empty/non-GitHub disabled; preview/delivery uses effective URL; no credential forwarding | implemented |
| CHAN-03 | Distribution: current template + new-node policy; node organizer owns prefix/exclusion/sort/dedup/incompatibility | Channel data separation | Renderer applies each saved option | implemented |
| TPL-01 | Per-channel native JSON/YAML template; true edit/dirty/validate/location/preview/save/cancel; generated nodes/auth/groups/rules/fallback reserved | New template storage/API/merge/validation; no shared library or DSL | Reject conflicts and invalid save; preserve other channel; preview equals delivery | implemented |
| SET-01 | Three categories: service/security, nodes/subscriptions, statistics/appearance; consistent rows, hover help, sticky save | Panel settings API/storage/bootstrap separation | Authenticated update, optimistic concurrency, invalid inputs, secrets redacted | implemented |
| SET-02 | Listen/address/domain, management token and optional GitHub token; blank token retains, explicit remove; server-only use | Bootstrap/security settings and catalog client | Restart semantics, session invalidation, no token in response/log/browser persistence | implemented |
| SET-03 | Public node host auto placeholder/custom override; unified identity name/key; protocol obfuscation stays protocol-specific | Publication settings and protocol identity | IPv4/IPv6/domain, failed auto detection cannot export bind/loopback, imported credentials unchanged | implemented |
| SET-04 | Traffic quota, language, theme, five presets/custom color picker with HEX and explicit apply/cancel after closing; default #6D4ED1, radius 0–32 default12 | Persisted preferences and preview transaction | Save/reload; unsaved category/route changes require confirmation; cancellation retains draft, confirmed departure restores saved; reset only color/radius | implemented |
| SET-05 | Theme links accent/background/border/focus/charts; R cards/dialogs, R/2 controls, min(32,7R/6) shell; badges/logo/status independent | Shared CSS tokens and accessible color derivation | Text contrast ≥4.5 for arbitrary light/dark colors, radius bounds, all component states | implemented |
| LOG-01 | Real-time core raw logs only; timestamp muted, full message matches level; all native levels, file/date/level selectors consistent and right aligned; one top-right status pill toggles live output and pause, without a separate icon or surrounding container | Core log file/tail API | Real file reading/stream/reconnect/pause/filter, independent scroll | implemented |
| LOG-02 | Panel log shows operation outcomes and runtime events; time, message, log level and source only | Unified event query with sanitized operation outcomes | Consistent displayed/filter levels; pagination/filter | implemented |

### Implementation and evidence map

| Requirements | Owning implementation and guide | Focused evidence |
| --- | --- | --- |
| UI-01/02, AUTH-01, RUN-01 | `web/src/components/app-shell`, shared `components/ui`, `pages/login-page`; existing HTTP session and origin protections | App-shell, route and login tests; 18 desktop layout cases; real centered dialogs, Toast, focus and return journeys |
| DASH-01/02 | `pages/dashboard-page`, `internal/hostmetrics`, `httpapi/metrics_stream.go`, persisted runtime transitions; [observability guide](../guides/subscriptions-and-observability.md) | Metrics sampling/stream tests, chart and timeline tests, browser missing-data and layout review |
| CORE-01/02 | `pages/cores-page`, application core commands and runtime preflight; [core guide](../guides/core-versions.md) | `httpapi/core_enable_test.go`, artifact verification and runtime tests; inline action and platform UI tests |
| CFG-01/02 | `application/configuration_file.go`, `store/configuration_file.go`, `server/configuration_runtime.go`, visual/JSON editors; [configuration guide](../guides/configuration-and-runtime.md) | File CAS and lossless tests; `server/configuration_runtime_test.go`; real HTTP browser invalid-text save/reload; schema form union/unknown-field round trips |
| SUB-01, NODE-01–04 | Source/node services, `singbox/inbound_convert.go`, `subscription/manual_node.go`, node editor/grid | Source refresh tests; `httpapi/subscription_nodes_test.go`, `manual_node_validation_test.go`, 1.14 inbound/TLS tests; browser hide/restore and conditional node form |
| KEY-01 | Store key accounting, public delivery and key panel; [subscription guide](../guides/subscriptions-and-observability.md) | 24-request concurrency tests, cross-channel quota HTTP tests, expiry/revocation/304/failure accounting, explicit secret-read authorization, persistence and UI tests |
| CHAN-01–03, RULE-01–03, TPL-01 | `subscription/channel_*`, channel policy API, channel workspace and nested editors | Native renderer and URL tests; `httpapi/subscription_channel_policy_test.go`; browser persisted group/reference reload, nested return, preview/copy and source visibility |
| SET-01–05 | `application/panel_settings.go`, `protocol_identity.go`, `store/panel_settings.go`, `pages/panel-settings-page`, `theme/appearance.ts` | CAS/redaction/validation/atomic identity tests; appearance contrast/bounds tests; browser preview, category retention, save/reload, leaving restores saved appearance |
| LOG-01/02 | `internal/corelogs`, runtime output observer, `store/panel_logs.go`, product log API and two log panels | File/rotation/custom-output tests; operation logs/cursors and authenticated file API tests; frontend pause/filter and log rendering tests |

Paths in this table are relative to `internal/` or `web/src/` where the context is
unambiguous. API details remain in `api/openapi.yaml`; the owning guides describe
persistence, security boundaries and runtime semantics.

### Persistence and preserved guarantees

The current `internal/store/schema.sql` creates the editable configuration,
immutable runtime evidence, subscription state and operational records directly.
Settings live in the selected `setting.json`; SQLite keeps only transaction
markers for interrupted settings/identity updates. Old storage formats require
a fresh development data directory and are never converted on startup.

Authenticated management, CSRF/origin controls, safe source acquisition,
immutable checked startup artifacts and exact core-version checks remain in place.
Core enable checks the deployed platform before selection; runtime preflight
checks the selected file before replacing the current process. Settings and
configuration writes use optimistic concurrency. Protocol identity updates and
the corresponding managed inbound credentials commit atomically.

The obsolete Web user, deployment and history components have been removed.
The CLI also removes configuration history, revision, diff and restore commands.
Internal immutable evidence remains for checked startup and runtime recovery. Unknown native configuration fields and numeric
lexemes survive editing. Neither hide nor delete-publication actions delete an
underlying core inbound.

### Verification workflows

- `make check` runs Go tests and vet, Web lint/tests/build, OpenAPI coverage,
  generated support checks, shell syntax checks and installer scenarios.
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
  settings persistence, notifications and observed runtime controls. Demo fixtures
  do not establish native process behavior.

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

Demo outcomes are explicitly separate from production results. Production
feedback follows completed operations and observed runtime state rather than hardcoded
Figma samples.

## HTTP API and security

The HTTP server delivers the embedded Web application, the authenticated
management API, and token-authenticated subscription output. Its default
listener is loopback-only.

### API contract and routing

The management API is rooted at `/api/v1`. The authoritative operation,
schema, status-code, and problem-detail contract is
[`api/openapi.yaml`](../../api/openapi.yaml); this guide describes its trust
boundaries without duplicating the endpoint inventory.

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

The shared settings file supplies the management token. Web replacements update
`auth.token` in that file; CLI or manual token edits are seen at the next
authentication boundary and invalidate existing sessions. API clients may send the
current token as a Bearer credential. Browser login exchanges it for an HttpOnly,
SameSite session cookie and a CSRF token.

Replacement tokens must contain 32–8192 UTF-8 bytes, without leading or trailing
Unicode whitespace or BOM, NUL, CR, or LF. Invalid replacements leave the current
credential and sessions intact. The login JSON body is bounded to 64 KiB so every
accepted token fits even when JSON encoding escapes its characters.

Cookie-authenticated state changes require both the session CSRF token and a
same-origin request. Login failures are rate-limited by the direct peer
address; forwarded-IP headers are not trusted. CORS is disabled by default.

The generated listener is `127.0.0.1:3000`. Before exposing the service beyond
loopback, place it behind a reviewed HTTPS reverse proxy, set
`server.external_origin` to its normalized public origin, and set
`auth.secure_cookie` so the browser session cookie is HTTPS-only. CSRF origin
checks use this explicit value and never trust `Forwarded` or
`X-Forwarded-*` headers. Do not treat the management token as a public
subscription token.

### Request and download protections

The server applies bounded request bodies, strict JSON decoding where the
contract requires it, constant-time token comparison, security response
headers, and secret-redacting event metadata.

Network downloads use explicit host and resolved-address checks, bounded
responses, timeouts, and restricted redirects. Browser core import accepts one
bounded multipart file and stages it in a
mode-0700 private data-directory location rather than accepting a server-local
path. Archive verification rejects path traversal, symbolic links, non-regular
entries, duplicate binaries, excessive expansion, and non-canonical gzip/tar
input. See [Core versions](../guides/core-versions.md#checks-and-recorded-hashes)
for the artifact trust boundary.

Third-party subscription refresh validates DNS and every redirect destination
against the configured source-network policy. Public subscription requests use
only persisted successful source versions and never fetch an upstream URL.

Keep the following data private:

- settings files and management tokens;
- exported sing-box configuration;
- subscription token plaintext and raw source versions; and
- diagnostic files that may contain paths or operator-provided values.

Use file or stdin inputs for secrets instead of command arguments.

### Concurrency and immutable evidence

Configuration writes use revision preconditions. The editable file API accepts
`{ revision, content }` in the `PUT /api/v1/config/file` JSON body. A stale
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
applied local startup artifact, current enabled source versions, channel and key.
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

See the [Web application reference](../../web/README.md) for frontend ownership
and build behavior.
