# Product design implementation contract

This document tracks the accepted product redesign and its implementation. It is
the product-behavior reference; OpenAPI and executable tests remain authoritative
for the implemented API. The matrix records the delivered behavior; the validation
record distinguishes automated and browser evidence from native-runtime limits.

## Accepted baseline and precedence

The accepted design is [sing-box-panel in Figma](https://www.figma.com/design/ppQe6qWogFBlM2gEhxAoEh/sing-box-panel).
The final review contains 15 formal screens, 402 components and 46 local overlays.
Earlier references to 16 screens, three log tabs, six settings categories, user
management, draft/history workflows or generic node-origin badges are superseded.
The two design conversations are `01a0a998-4f14-7243-b0fc-db7074ed390e` and
`01a0b44b-e100-7001-a96d-4950db6e476d`. The implementation authorization explicitly
includes frontend, backend, contracts, persistence and migration; it does not
authorize discarding data or weakening authentication/artifact guarantees.

The formal screens are login, dashboard, installed/available versions, sources,
manual nodes, source detail, subscription keys, channels, channel detail, visual
configuration, JSON configuration, panel settings, core logs and panel logs.
Detail tabs and feedback are local states, not duplicate screens.

## Requirement → screen → contract → implementation → verification

All rows below are implemented. Verification is scoped to the checks recorded
below; this does not claim a native Linux lifecycle or live Mihomo acceptance test.

| ID | Screen and accepted behavior | Contract / implementation boundary | Verification coverage | Status |
| --- | --- | --- | --- | --- |
| UI-01 | Floating 248px sidebar, 64px header; desktop title y=112/body y=168; aligned bottom; six navigation entries in dashboard/version/subscription/configuration/panel/log order | App shell, responsive layout, shared controls | 1280/1440/1920 widths and 720 height, keyboard, no clipping | implemented |
| UI-02 | Transparent 20px action icons in 40px targets; consistent hover/pressed/focus; centered dialogs; compact Toast; no duplicate headings/footers | Shared UI and feedback | Real actions, focus return, Escape, visible validation, no false success | implemented |
| AUTH-01 | Compact token login; remove redundant brand/label/help; accessible input remains | Existing authenticated session, CSRF/origin/rate-limit boundaries | Login/logout/error tests, credentials never exposed | implemented |
| RUN-01 | One status badge: demo fixed, real running/stopped/failed accurately; green start/red stop, restart; theme/language/logout in sidebar footer | Runtime observation and lifecycle, shared header | Unknown/stale never reported running; check failure keeps current process | implemented |
| DASH-01 | CPU/memory summary, transfer graph and one-hour active connections graph; compact external axes/units; no refresh/footer/extra core metric strip | Metrics history and real push updates | Real samples, reconnect, missing data, time window | implemented |
| DASH-02 | 24h runtime history in 48 equal segments; running/failed/stopped/unknown with legend, no yellow state | Persistent runtime history | Restart persistence and unknown periods | implemented |
| CORE-01 | Installed/available fill-height lists, inline enable/disable/download; source and capabilities retained; 5/10/50 pagination | Catalog, artifacts, runtime switch | Paging and scrolling; invalid switch retains runtime; artifact verification | implemented |
| CORE-02 | Read-only deployed OS/CPU architecture with hover help; matching assets only, never browser architecture or unknown→ARM64 | Platform detection, catalog/cache, imported artifact inspection | Cross-architecture assets and rate-limit/cache behavior | implemented |
| CFG-01 | One editable config.json, 13 native root sections and advanced JSON; preserve unknown fields and numbers | Canonical saved text, version-scoped schema, validated startup artifacts | Lossless round trip; invalid JSON can save, cannot start | implemented |
| CFG-02 | Save independent of running process; loaded/dirty/restart-required/start-pending states; start/restart checks saved file before replacement | Saved-file identity vs loaded immutable artifact; CAS and runtime transitions | Concurrent edits, failed validation preserves running process, restart loads saved bytes | implemented |
| SUB-01 | Sources/keys/channels only; source names single-line; toolbar refresh then add; URL-only source creation; edit/refresh row actions | Source API, SSRF-safe acquisition and source versions | Source CRUD/failure retains last success; no single-node source path | implemented |
| NODE-01 | Local publishable inbounds and manual nodes in one manual collection; real name, actual source badge, protocol/endpoint/TLS/Reality/SNI | Node catalog/provenance and inbound-to-client conversion | No automatic core outbound/TUN/redirect/tproxy publishing; no invented prefixes | implemented |
| NODE-02 | Eye controls visibility; hidden cards remain recoverable at source; channel filters before pagination without deleting membership/order | Independent visibility and channel membership | Hide/restore selected node, empty/page bounds, no ghost cards | implemented |
| NODE-03 | Native conditional protocol editor and node-scoped advanced JSON; parse share link then confirm; no unknown-field loss | Protocol capability/schema and references | TLS/QUIC exclusions, HY2 port modes, SS UoT/mux, SSH auth, detour cycles, version support | implemented |
| NODE-04 | Listener vs published addresses; credentials masked/copyable; local edit links to configuration; hiding/deleting publication cannot delete inbound | Local-node provenance and configuration deep links | IPv6, unknown public IP, no listener/SNI/port mutation, real copy results | implemented |
| KEY-01 | Subscription keys replace user UI; expiry and successful-download quota shared across all channels; failed requests do not consume | Token storage and atomic delivery accounting; legacy access migration | Concurrent quota requests, expiry, failures, existing grants preserved | implemented |
| CHAN-01 | Continuous node grid, actual source badges; nodes/rules tabs; output/distribution/preview toolbar | Channel selected nodes and native output formats | Selection/order/hidden filtering and format change | implemented |
| CHAN-02 | Ordered enabled rule groups, candidates, default exits, manual domain/IP/CIDR and remote references; unmatched fallback | Typed channel configuration, native renderers | New group selects current eligible candidates; edit/additional nodes preserve choice; valid exits | implemented |
| RULE-01 | Remote metadata only, client fetches; Mihomo YAML/YML/TEXT/MRS + behavior; MRS excludes classical; sing-box source JSON/binary SRS | Remote-reference validation and native output | No server content fetch/upload/conversion; incompatible format cleared and blocked without URL loss | implemented |
| RULE-02 | Group context inherited; smaller child modal returns to correct parent; missing format red border only; anchored menus | Modal state, accessible invalid controls | Nested save/cancel/focus; short viewport sticky actions and complete menus | implemented |
| RULE-03 | Single-line full URL; lightning toggles gh-proxy.com for eligible GitHub/raw/gist; unwrap known proxy, no duplicates; off restores origin | URL normalization and effective render URL | Empty/non-GitHub disabled; preview/delivery uses effective URL; no credential forwarding | implemented |
| CHAN-03 | Distribution: current template + new-node policy; node organizer owns prefix/exclusion/sort/dedup/incompatibility | Channel data separation with migration | Existing values preserved and renderer applies each option | implemented |
| TPL-01 | Per-channel native JSON/YAML template; true edit/dirty/validate/location/preview/save/cancel; generated nodes/auth/groups/rules/fallback reserved | New template storage/API/merge/validation; no shared library or DSL | Reject conflicts and invalid save; preserve other channel; preview equals delivery | implemented |
| SET-01 | Three categories: service/security, nodes/subscriptions, statistics/appearance; consistent rows, hover help, sticky save | Panel settings API/storage/bootstrap separation | Authenticated update, optimistic concurrency, invalid inputs, secrets redacted | implemented |
| SET-02 | Listen/address/domain, management token and optional GitHub token; blank token retains, explicit remove; server-only use | Bootstrap/security settings and catalog client | Restart semantics, session invalidation, no token in response/log/browser persistence | implemented |
| SET-03 | Public node host auto placeholder/custom override; unified identity name/key; protocol obfuscation stays protocol-specific | Publication settings and protocol identity | IPv4/IPv6/domain, failed auto detection cannot export bind/loopback, imported credentials unchanged | implemented |
| SET-04 | Traffic quota, language, theme, six colors/custom HEX; default #6D4ED1, radius 0–32 default24 | Persisted preferences and preview transaction | Save/reload, category switch retains draft, leaving restores saved; reset only color/radius | implemented |
| SET-05 | Theme links accent/background/border/focus/charts; R cards/dialogs, R/2 controls, min(32,7R/6) shell; badges/logo/status independent | Shared CSS tokens and accessible color derivation | Text contrast ≥4.5 for arbitrary light/dark colors, radius bounds, all component states | implemented |
| LOG-01 | Real-time core raw logs only; timestamp muted, full message matches level; all native levels, file/date/level selectors consistent and right aligned; LIVE/pause | Core log file/tail API | Real file reading/stream/reconnect/pause/filter, independent scroll | implemented |
| LOG-02 | Panel log combines tasks and events once; task ID in details only; statuses, failure details and permitted retry | Unified event query and task lifecycle correlation | No duplicate records; retry authorization/state; pagination/filter | implemented |

## Implementation and evidence map

| Requirements | Owning implementation and guide | Focused evidence |
| --- | --- | --- |
| UI-01/02, AUTH-01, RUN-01 | `web/src/components/app-shell`, shared `components/ui`, `pages/login-page`; existing HTTP session and origin protections | App-shell, route and login tests; 18 desktop layout cases; real centered dialogs, Toast, focus and return journeys |
| DASH-01/02 | `pages/dashboard-page`, `internal/hostmetrics`, `httpapi/metrics_stream.go`, persisted runtime transitions; [observability guide](subscriptions-and-observability.md) | Metrics sampling/stream tests, chart and timeline tests, browser missing-data and layout review |
| CORE-01/02 | `pages/cores-page`, application core commands and runtime preflight; [core guide](core-versions.md) | `httpapi/core_enable_test.go`, artifact verification and runtime tests; inline action and platform UI tests |
| CFG-01/02 | `application/configuration_file.go`, `store/configuration_file.go`, `server/configuration_runtime.go`, visual/JSON editors; [configuration guide](configuration-and-runtime.md) | File CAS and lossless tests; `server/configuration_runtime_test.go`; real HTTP browser invalid-text save/reload; schema form union/unknown-field round trips |
| SUB-01, NODE-01–04 | Source/node services, `singbox/inbound_convert.go`, `subscription/manual_node.go`, node editor/grid | Source refresh tests; `httpapi/subscription_nodes_test.go`, `manual_node_validation_test.go`, 1.14 inbound/TLS tests; browser hide/restore and conditional node form |
| KEY-01 | Store key migration/accounting, public delivery and key panel; [subscription guide](subscriptions-and-observability.md) | Migration/24-request concurrency tests, cross-channel quota HTTP tests, expiry/revocation/304/failure accounting, one-time secret UI tests |
| CHAN-01–03, RULE-01–03, TPL-01 | `subscription/channel_*`, channel policy API, channel workspace and nested editors | Native renderer and URL tests; `httpapi/subscription_channel_policy_test.go`; browser persisted group/reference reload, nested return, preview/copy and source visibility |
| SET-01–05 | `application/panel_settings.go`, `protocol_identity.go`, `store/panel_settings.go`, `pages/panel-settings-page`, `theme/appearance.ts` | CAS/redaction/validation/atomic identity tests; appearance contrast/bounds tests; browser preview, category retention, save/reload, leaving restores saved appearance |
| LOG-01/02 | `internal/corelogs`, runtime output observer, `store/panel_logs.go`, product log API and two log panels | File/rotation/custom-output tests; task deduplication/cursors, authenticated file API and retry-state tests; frontend pause/filter and log rendering tests |

Paths in this table are relative to `internal/` or `web/src/` where the context is
unambiguous. API details remain in `api/openapi.yaml`; the owning guides describe
persistence, security boundaries and runtime semantics.

## Persistence and preserved guarantees

SQLite migrations 0002–0006 add settings, the exact saved configuration file,
userless subscription keys, manual-node visibility and inherited public-host
policy. They preserve legacy IDs, grants, digests, usage and channel settings.
New key UI does not require users; legacy API clients retain their access scope.

Authenticated management, CSRF/origin controls, safe source acquisition,
immutable checked startup artifacts and verified core identities remain in place.
Core enable checks trust and deployed platform before queueing; runtime preflight
checks the selected file before replacing the current process. Settings and
configuration writes use optimistic concurrency. Protocol identity updates and
the corresponding managed inbound credentials commit atomically.

The obsolete Web user, deployment and history components have been removed.
The CLI also removes configuration history, revision, diff and restore commands.
Internal immutable evidence and existing API contracts remain for checked startup
and runtime recovery. Unknown native configuration fields and numeric
lexemes survive editing. Neither hide nor delete-publication actions delete an
underlying core inbound.

## Validation record — 2026-09-19

- Initial baseline passed Web type checking, focused Go application/store/HTTP/
  subscription tests and `git diff --check`. Pre-existing frontend and canonical
  draft-store changes were preserved.
- Final repository validation covers Go formatting, module tidy diff, vet, all Go
  tests, Web lint/typecheck/tests/build, OpenAPI coverage, generated core support,
  notices, shell syntax and installer tests. `go test -race ./...` also passed.
  Commands: `make check-go web-typecheck check-contracts`, and in `web`,
  `corepack pnpm run lint` plus `corepack pnpm run test` (34 files, 155 tests).
  Installer verification passed all six groups; `git diff --check` passed.
- Linux amd64 and arm64 binaries cross-compiled successfully. Cross-compilation
  establishes build compatibility, not execution of an installed core.
- The real HTTP/SQLite browser fixture verified settings save/reload and unsaved
  appearance restoration; invalid core JSON save/reload and blocked validation;
  saved channel groups/references, effective accelerated URLs, nested cancel,
  decoded native preview/copy and form actions in a 1280×720 viewport.
- Browser layout review covered the six main routes at 1280×720, 1440×1000 and
  1920×1000. Titles align at y=112, no horizontal document overflow or clipped
  visible controls was found, and content scrolls while save actions remain
  accessible. Node dialogs were also checked at the short viewport.
- Demo browser interaction verified hide/restore without a hidden channel card,
  the Hysteria2 port-range editor, theme/radius preview, category changes, saved
  appearance restoration on departure, reset/save and the management-token dialog.
  These screenshots use demo data, not live production traffic or lifecycle data.

## Deliberate distinctions and validation limits

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
- Linux process lifecycle cannot execute on this macOS development host. Tests
  verify preflight/fencing with controlled executors; the browser fixture uses real
  HTTP/SQLite and deliberately has no runtime executor. Native installed-core
  start/restart smoke testing requires a Linux environment.
- The build reports a large precompiled schema-validator chunk. It remains lazily
  loaded with configuration editing; the warning is not a build failure.

Demo outcomes are explicitly separate from production results. Production
feedback follows persisted operations and task completion rather than hardcoded
Figma samples. No Figma assets, Git history or deployed services were changed by
this code implementation.
