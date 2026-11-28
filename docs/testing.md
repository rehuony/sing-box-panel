# Testing

Tests verify behavior at its owning boundary. Page tests connect controls to
actions; they do not repeat every domain case through a complete editing journey.

## Coverage ownership

| Behavior formerly repeated in page journeys | Owning checks |
| --- | --- |
| Configuration save/reload, incomplete JSON, CAS conflict, failed save, draft reset, exact numbers | `use-canonical-configuration.test.tsx`; HTTP configuration and application tests verify real persistence |
| Release lines, schema preparation, the reviewed 1.13 contract and its Web fixture | Go `singbox/line_test.go`, `schema/configuration_schema_test.go`, `schema/configuration_1_13_test.go`, `nodeconfig/schema_test.go` |
| Schema the node's core prints | Go `sing-box-node/agent/commands_test.go` with a fake core binary |
| Credential formats and Hysteria2 obfuscation | `configuration-credentials.test.tsx`, `managed-collections-editor.test.tsx` |
| Version selection, schema availability and errors, stale responses and retained session choice | `configuration-session.store.test.ts`, `use-configuration-schema.test.tsx` |
| Navigation cancellation and discard | `use-unsaved-changes.test.tsx`; configuration Hook reset checks |
| Log rotation, cursor recovery, clear races and cancellation | `use-core-logs.test.tsx`; Go core-log and HTTP tests |
| Group members, format restrictions and save payload | `channel-policy.test.ts`, `use-channel-draft.test.tsx`, `channel-node-picker.test.tsx`; channel submit smoke |
| Exit-rule order, rule set expansion, same-named remotes and group matches | Go `channel_policy_test.go` owns rendering and validation; `httpapi/subscription_routes_test.go` owns rule set CRUD, icons and order, client refusals and the cascade that advances channel versions |
| Pool order, filtered drag saves and stale orders | `node-order.test.ts`, `subscription-node-panel.test.tsx`; Go `pool_test.go` owns synchronization and its cleanup |
| Source creation with a preview, server refusals, one save for settings and content, and deletion | `subscription-source-panel.test.tsx`; Go `source_parse_test.go` and `service_sources_test.go` own parsing, filters and refresh failures |
| Settings payload, token validation and backup requests | `use-panel-settings-draft.test.tsx`, `settings-validation.test.ts`, `panel-settings.test.ts`; Go `backup_test.go` owns archive checks; provider checks and submit/restore smoke |
| Real release, process ownership and systemd behavior | Existing Go, Linux packaging and release jobs |

### Removed duplicate checks

| Removed interaction or assertion | Remaining owner / reason |
| --- | --- |
| Channel member selection followed by another complete save | `use-channel-draft.test.tsx` owns membership and the If-Match payload; picker tests own confirmation |
| Unsupported channel strategies and repeated URL acceleration toggles | `channel-policy.test.ts` owns format restrictions and URL normalization |
| Settings invalid-category matrices, hash navigation and repeated save journeys | Draft Hook owns validation/category/payload; `use-hash-tab.test.tsx` owns navigation; page retains focus and submission wiring |
| Log toolbar visibility, repeated streaming cursor/filter journeys | `use-core-logs.test.tsx` owns stream state/cursors; page retains selection, live toggle and confirmed deletion |
| Animated icon states, language-menu glyphs and exact chart tooltip coordinates | Decorative implementation details; removed without replacement |
| Select hover/Home/Escape behavior | Third-party select behavior; application value conversion and accessibility forwarding remain |
| Every translated diagnostic rendered through a full dialog | Existing i18n key checks plus representative log-detail/error wiring |
| Toast-manager timeouts, dismissal/type matrices and update behavior | Third-party implementation; `error-notice.test.tsx` retains application deduplication, recovery and accessible descriptions |
| Standalone theme-button cycling journey | Settings submit smoke already checks the status line's theme preference stays independent of server defaults |
| Repeated pagination keyboard/boundary matrices on the cores page | `list-pagination.test.tsx` owns custom entry/cancel/bounds; core page retains filtering/page reset wiring |
| Schema help hover mechanics, closing animations, repeated map editing and fixed tab/field order | `info-tooltip.test.tsx` owns custom help behavior; `schema-map-fields.test.tsx` owns map editing; staged cancel checks remain; decorative order/animation assertions removed |
| Dashboard tab mechanics and exact timeline segment count | Third-party tabs and presentation detail; `runtime-timeline.test.ts` retains evidence/gap logic |

Configuration fixture projection lives in `schema-ui.test.ts`;
the representative editor checks only user edits, null preservation and staged
confirmation. Hover/focus route wiring is checked in `app.routes.test.tsx`;
preload caching and failure branches have one owner, `page-loaders.test.ts`,
plus the real Vite CSS recovery contract. Tooltip tests retain custom click,
touch, keyboard and stale-hover behavior, rather than repeating the UI library's
ordinary hover contract. Fixed column order and decorative icon assertions are
removed without replacing them with implementation snapshots.
Color confirmation, canceled drafts and the injected stylesheet's CSP nonce
belong to `appearance-color-picker.test.tsx`. Establish the page nonce before
any picker mounts: the third-party library caches styles per document, and a
test must not simulate changing the CSP nonce halfway through that lifetime.

Preserve distinct failures in separate tests. Decorative classes, icon choices,
incidental copy and third-party widget mechanics do not need duplicate page
assertions. A security or data-integrity boundary must retain a direct check.

## Commands and environments

`pnpm test` in `web` runs every package's tests, the panel's three Vitest
projects among them. Use `pnpm exec vitest run --project logic`, `--project react`,
or `--project contracts` from `web/apps/panel` to run
one boundary.
Logic uses Node; React Hooks and component smoke use jsdom; build contracts
use Node with an explicit jsdom override for CSS preload recovery.

Test entry points run `pnpm run generate`, which writes the OpenAPI types, before
Vitest loads them. Direct `pnpm exec vitest` or `pnpm exec tsc` commands bypass
that preparation; first run `pnpm run generate` in a clean checkout.

Editor tests use the two schemas in `web/apps/panel/src/tests/fixtures/`: the
reviewed 1.13 schema, which a Go test keeps equal to the panel's, and one a 1.14
core printed. Equivalent form behavior can use one of them.

Use controlled promises for asynchronous outcomes and virtual time for timers,
expiry and retries. Unmount before restoring timers; abort streams and remove
listeners on cleanup. Timeouts bound hangs, not acceptable application latency:
logic has 5 seconds, React 10 seconds, and build contracts 30 seconds. Retries
remain disabled.

CI bounds workers to two and runs build contracts sequentially. Test isolation
remains enabled. `make check-frontend` builds the Web application once, including
generation and TypeScript checking, then runs lint, all Vitest projects and the
third-party notices check. `make check` shares that
build with Go and API/installer checks. Linux-only guarantees remain in their
existing jobs.

When changing the test layout, use `pnpm exec vitest list --json` to compare
discovered files against `src/tests/**/*.test.{ts,tsx}`. Every file must belong
to exactly one project. Before landing a broad suite refactor, run the complete
suite independently with CI concurrency, `--sequence.shuffle --sequence.seed=28`,
and `--maxWorkers=1`. A later passing run does not dismiss an earlier failure.
