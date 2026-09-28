# Data and interaction refactor verification

## Scope

The refactor preserves REST/SSE routes, persisted traffic accounting and the
existing visual system. Ordinary reads use a session-local TanStack Query cache;
runtime, metrics and dashboard history use separate Zustand subscriptions.
Live collection runs every two seconds, accounting checkpoints remain ten
seconds, and subscribed clients share each history snapshot. The chart's live
tail is display-only and is replaced at the history persistence watermark.

The behavior and proxy configuration are documented in
[Subscriptions and observability](../guides/subscriptions-and-observability.md)
and the [Web application](../../web/README.md).

## Automated verification, 2026-09-27

- `make check`: Go formatting, module consistency, vet, all Go tests,
  TypeScript, ESLint, 194 logic tests, 456 React tests, production build,
  8 build-contract tests, third-party notices, support generation checks,
  installer tests and OpenAPI validation.
- `go test -race ./internal/application ./internal/httpapi ./internal/publicip
  ./internal/runtime ./internal/server ./internal/store`: passed on macOS arm64
  and in the existing Linux arm64 contract container. Linux coverage includes
  cancellation/shutdown with an open metrics stream.
- `bash scripts/test/core-contract.sh`: passed in the Linux arm64 container
  against pinned official binaries for 1.13.19, 1.13.20, 1.13.21, 1.14.0,
  1.14.1 and 1.14.2. Artifact checksums passed.
- `git diff --check`: passed.

New regression coverage owns request deduplication, cancellation, targeted
invalidation, ambiguous write outcomes, session changes and late responses;
three-worker subscription refresh; retained settings drafts and conflict
revisions; two-second live samples with ten-second accounting, restart,
regression, collection failure and UTC-period boundaries; shared snapshots and
slow readers; half-open streams, visibility and dashboard subscription lifetime;
normal renewal, recovery grace and terminal 401; and history/live chart gaps and
watermark replacement. The reconnect test advances **virtual** time by 30 minutes;
it is not a real-network soak result.

## History benchmark

Command:

```sh
go test ./internal/store -run '^$' \
  -bench '^BenchmarkMetricsHistoryNinetyDays' -benchtime=1x -benchmem
```

Apple M5, macOS arm64, 777,600 samples covering 90 days. Each range was measured
once; these observations are not a statistical speedup claim.

| Query range | Elapsed | Allocated | Allocations |
| --- | ---: | ---: | ---: |
| 1 hour | 162.18 ms | 30,496 B | 1,156 |
| 24 hours | 206.74 ms | 125,472 B | 5,709 |
| 90 days | 4.583 s | 230,640 B | 13,243 |

The pre-refactor 90-day baseline was approximately 4.791 s. The full-range
query remains expensive. The principal improvement here is sharing aggregation
between clients and stopping it outside the dashboard, rather than changing
historical coverage/accounting semantics. These are database query timings,
not dashboard page-load timings.

## Browser and proxy checks

The real HTTP server and production web build were served against the opt-in,
isolated browser-review database:

```sh
SBP_BROWSER_REVIEW=1 SBP_BROWSER_NODES=500 \
  go test ./internal/server -run '^TestBrowserReview$' -v -timeout 30m
```

Verified in the Codex browser at desktop size and 390×844: 500-node source
search, pagination, refresh retaining page two, node editor opening and Escape
close, light/dark presentation, candidate selection and F2/arrow-key reordering
with saved order. The narrow source page had equal viewport and scroll widths
(390 px), and its node dialog remained within the viewport. Browser logs had
no warnings or errors during this check. This fixture has **no runtime executor**;
its empty metrics are not evidence of live Linux collection latency.

A separate production demo build verified the 1-hour/24-hour charts, the
live tail, and keyboard cursor tooltips in a real browser, with no console
warnings or errors. Those values are synthetic demo data, not a measurement of
the runtime collector.

An isolated Nginx 1.31.6 instance proxied the fixture over loopback with
`proxy_buffering off` and a 75-second read timeout. Metrics and dashboard were
read concurrently through two consecutive connections each (about 118 seconds):

| Stream | Events | Heartbeats | Largest event gap | Renewal times |
| --- | ---: | ---: | ---: | --- |
| Metrics | 61 | 10 | 2.008 s | 59.019 s, 59.004 s |
| Dashboard | 5 | 10 | 30.005 s | 59.018 s, 59.006 s |

Initial response headers arrived in 16–18 ms, and renewed connections in 3–4 ms.
Both streams received the control event before closure. This verifies a real
local Nginx hop; it does not verify a deployed TLS/CDN/remote proxy chain.

## Outstanding deployment acceptance

No claim is made yet for interaction feedback p95 ≤100 ms, cached page usability
≤200 ms, or sample-to-visible p95 ≤3 s under **150 ms RTT plus 4× CPU throttling**.
Those conditions were not available through the browser control interface.
A continuous 30-minute real-network browser soak and live Linux collection
profiling also remain unverified; unit/race/core-contract results do not replace
them. External downloads and subscription-source timings were not benchmarked.

For deployment acceptance, keep one production build and the same 500-node,
90-day dataset for both revisions, apply the stated RTT/CPU settings, and record
browser input-to-paint, cached navigation-to-usable, and live-sample timestamp
to visible paint distributions. Check network traces across at least 30 renewal
cycles, offline/recovery, revoked authentication, hidden/visible tabs and a slow
second client. Record external-source transfer times separately from panel work.
