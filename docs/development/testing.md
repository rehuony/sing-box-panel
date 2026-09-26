# Testing

Tests verify behavior at its owning boundary. Page tests connect controls to
actions; they do not repeat every domain case through a complete editing journey.

## Coverage ownership

| Behavior formerly repeated in page journeys | Owning checks |
| --- | --- |
| Configuration save/reload, incomplete JSON, CAS conflict, failed save, draft reset, exact numbers | `use-canonical-configuration.test.tsx`; HTTP configuration and application tests verify real persistence |
| Exact Schema versions, integrity and generated validators | `resolve-reviewed-schema.test.ts`, `generated-validator-browser.test.ts`, production build and schema regeneration job |
| Credential formats and Hysteria2 obfuscation | `configuration-credentials.test.tsx`, `managed-collections-editor.test.tsx` |
| Version selection, unavailable versions, stale responses and retained session choice | `configuration-session.store.test.ts`, `use-configuration-schema.test.tsx` |
| Navigation cancellation and discard | `use-unsaved-changes.test.tsx`; configuration Hook reset checks |
| Log rotation, cursor recovery, clear races and cancellation | `use-core-logs.test.tsx`; Go core-log and HTTP tests |
| Policy transitions, format restrictions, selection and save payload | `channel-policy.test.ts`, `use-channel-draft.test.tsx`; channel submit smoke |
| Candidate membership, filtered ordering and source visibility | `node-order.test.ts`, `channel-policy.test.ts`; custom F2/Escape/drag integration remains in component tests |
| Source load ordering, polling, fresh revisions and refresh failures | `use-subscription-sources.test.tsx`; API cache tests and source submit smoke |
| Settings payload, token validation and backup parsing | `use-panel-settings-draft.test.tsx`, `settings-validation.test.ts`; provider checks and submit/restore smoke |
| Real release, process ownership and systemd behavior | Existing Go, Linux packaging and release jobs |

### Removed duplicate checks

| Removed interaction or assertion | Remaining owner / reason |
| --- | --- |
| Channel member selection followed by another complete save | `use-channel-draft.test.tsx` owns membership/exclusions and CAS payload; picker tests own confirmation |
| Unsupported channel strategies and repeated URL acceleration toggles | `channel-policy.test.ts` owns format restrictions and URL normalization |
| Source refresh through three equivalent toolbar paths; missing/invalid recorded starts repeated in the page | `use-subscription-sources.test.tsx` owns refresh/revision/start behavior; HTTP cache tests own invalidation |
| Settings invalid-category matrices, hash navigation and repeated save journeys | Draft Hook owns validation/category/payload; `use-hash-tab.test.tsx` owns navigation; page retains focus and submission wiring |
| Log toolbar visibility, repeated streaming cursor/filter journeys | `use-core-logs.test.tsx` owns stream state/cursors; page retains selection, live toggle and confirmed deletion |
| Animated icon states, language-menu glyphs and exact chart tooltip coordinates | Decorative implementation details; removed without replacement |
| Select hover/Home/Escape behavior | Third-party select behavior; application value conversion and accessibility forwarding remain |
| Every translated diagnostic rendered through a full dialog | Existing i18n key checks plus representative log-detail/error wiring |
| Toast-manager timeouts, dismissal/type matrices and update behavior | Third-party implementation; `error-notice.test.tsx` retains application deduplication, recovery and accessible descriptions |
| Standalone theme-button cycling journey | Settings submit smoke already checks the sidebar preference stays independent of server defaults |
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

`pnpm test` runs all three Vitest projects. Use `pnpm test:logic`,
`pnpm test:react`, or `pnpm test:contracts` from `web` to run one boundary.
Logic uses Node; React Hooks and component smoke use jsdom; build contracts
use Node with an explicit jsdom override for CSS preload recovery.

Tests read committed Schema assets. The production Vite plugin checks their
generation in CI; ordinary tests do not regenerate them or invoke Go export.
Keep exact-version integrity coverage even when equivalent form behavior is
tested using one representative Schema.

Use controlled promises for asynchronous outcomes and virtual time for timers,
expiry and retries. Unmount before restoring timers; abort streams and remove
listeners on cleanup. Timeouts bound hangs, not acceptable application latency:
logic has 5 seconds, React 10 seconds, and build contracts 30 seconds. Schema
imports belong in suite preparation. Retries remain disabled.

CI bounds workers to two and runs build contracts sequentially. Test isolation
remains enabled. `make check-web` runs static checks, logic, React, production
build, build contracts and third-party notices; `make check` also runs Go and
API/installer contracts. Linux-only guarantees remain in their existing jobs.
Each web phase reports its duration and exits immediately on failure.

When changing the test layout, use `pnpm exec vitest list --json` to compare
discovered files against `src/tests/**/*.test.{ts,tsx}`. Every file must belong
to exactly one project. Before landing a broad suite refactor, run the complete
suite independently with CI concurrency, `--sequence.shuffle --sequence.seed=28`,
and `--maxWorkers=1`. A later passing run does not dismiss an earlier failure.
