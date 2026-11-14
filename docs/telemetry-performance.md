# Telemetry performance

Measurements of the panel's fleet reads, traffic writes and metrics history,
and what they do and do not show. The behavior is described in
[Subscriptions and observability](subscriptions-and-observability.md) and the
[panel and node architecture](panel-node-architecture.md#telemetry).

## What a fleet read costs

The fleet views read every node with its metrics from one snapshot. Reading
it is one query for the nodes (`store.Nodes`); each node's last state report is
decoded once, and its live evidence, current period and newest accepted
traffic sample come from the telemetry ledger in memory. The ledger reads the
store only the first time it needs a node's period or newest sample after the
panel starts, and again when a rollover or a changed reset day or period length
asks for another range. Every accepted sample updates it from the result of
its own transaction; a rejected one leaves it as it is.

The same read encodes each node's item once, for every reader. A node whose
item differs from its previous one only in when it was read keeps its previous
item and encoding, so an unchanged node is the same bytes in every snapshot.

`/fleet/stream` shares one snapshot among every client and reads a new one
every two seconds while anyone streams it. Each connection gets every node it
selects once, then only the nodes whose encoding changed and the IDs that went
away; a read with no change sends nothing. `/fleet/status` returns the shared
snapshot while it is at most two seconds old, without a query, and reads a new
one otherwise; a panel write that changes what the snapshot shows, such as a
rename or a command's result, makes the next read a new one. Its `ETag` is a
digest of the response, so a client holding the same page gets `304`.

Before this design, a fleet read made one query for the nodes and three more
per node (the node row again, its period's daily totals and its newest
accepted sample) and decoded each state report twice, and each of the
overview, the nodes view and the shell polled it on its own.

## Benchmarks

Commands, run on Apple M5, macOS arm64, Go 1.26, each three times with the
same result to within a few percent:

```sh
go test ./internal/sing-box-panel/fleet/ -run '^$' \
  -bench 'BenchmarkFleetStatus|BenchmarkRecordNodeSample' -benchtime=20x -benchmem
```

`BenchmarkFleetStatus` reads the fleet of 50, 200 or 1000 nodes that each
reported a state with a running core and two stored traffic samples, without a
stream, so every read is a new snapshot; one read before the timer fills what
the stored samples did not. The new read includes encoding every node, which
the HTTP layer did on every request before. `BenchmarkRecordNodeSample` stores
one sample of 200 such nodes, each ten seconds after the node's last.

| Benchmark | Before | After | Allocations before → after |
| --- | ---: | ---: | ---: |
| Fleet read, 50 nodes | 2.37 ms | 0.29 ms | 16,875 → 4,131 |
| Fleet read, 200 nodes | 10.97 ms | 1.02 ms | 67,402 → 16,290 |
| Fleet read, 1000 nodes | 94.8 ms | 4.89 ms | 337,839 → 81,106 |
| Stored sample | 155–182 µs | 151–181 µs | 307 → 315 |

A CPU profile of the 1000-node read, before encoding moved into it, put about
95% of it in the one `store.Nodes` query and about 3% in decoding state
reports, so the decoded reports are not cached. Storing a sample is unchanged:
its time is the transaction's commit, and the read of the node row it makes
for the period settings is about a fifth of its CPU. At the ten-second
baseline, 500 nodes store about 50 samples a second, under 10 ms of work per
second, so that read stays.

## Fleet frame size

The 1000-node benchmark fleet encodes to 1.34 MB, 1,331 bytes per node. Fleet
items leave out the newest stored sample and the period's counters, which only
a node's own views read; that removes about 460 bytes per node. Real nodes
report core checksums, artifact IDs and host metrics, so a node is closer to
2 KB, and the whole fleet passes the 1 MiB that bounds a node's stream frames
at about 500 nodes. Fleet stream frames are therefore bounded at 16 MiB on the
panel. Only a connection's first frame carries the whole
fleet; nodes report every ten seconds, so a later frame carries about a fifth
of the online nodes, and none while nothing changes. `limit` and `cursor` page
large fleets through `/fleet/status`.

## Live fleet

```sh
SBP_FLEET_SCALE=200,500 go test ./internal/sing-box-panel/fleet/ \
  -run TestFleetAtScale -v -timeout 30m
```

`TestFleetAtScale` is opt-in. It serves the fleet on a loopback listener and
connects simulated nodes over TLS, each on its own connection, through the real
enrollment, handshake and session code. Every node reports a running core,
then sends 30 metrics samples with traffic counters, one a second of wall time
and ten seconds apart in sample time, so every sample is stored, and a state
report every tenth sample. Every node changes in every two-second snapshot. One
client follows the shared snapshot the way `/fleet/stream` does. The test
fails if a node's period does not hold the traffic of every sample after its
first, or if a warm snapshot costs as much as reading one node row per node,
which any per-node query would. Single runs on Apple M5 (macOS arm64) and in a
Linux arm64 container on the same machine (10 cores, Go 1.26.10):

| Measure | 200 nodes, macOS | 200 nodes, Linux | 500 nodes, macOS | 500 nodes, Linux |
| --- | ---: | ---: | ---: | ---: |
| Enroll and connect every node | 0.62 s | 1.57 s | 1.54 s | 2.48 s |
| First snapshot, the ledger loading each node | 9.6 ms | 17.3 ms | 14.0 ms | 30.8 ms |
| Warm snapshot | 1.44 ms | 5.11 ms | 9.81 ms | 10.09 ms |
| One node row read per node, for comparison | 4.87 ms | 14.23 ms | 24.23 ms | 32.82 ms |
| First stream frame | 306 KB | 247 KB | 807 KB | 811 KB |
| Later frames, every node changed | 326 KB every 2 s | 330 KB every 2 s | 817 KB every 2 s | 826 KB every 2 s |
| Samples stored | 200 a second | 200 a second | 500 a second | 500 a second |
| Samples lost | none | none | none | none |
| Heap, panel and simulated nodes | 29.8 MiB | 31.7 MiB | 75.7 MiB | 73.8 MiB |

A node's item is 1.2–1.6 KB. The later frames carry every node because the
accelerated nodes sample faster than the snapshot refreshes; at the ten-second
baseline a frame carries about a fifth of the online nodes, so a 500-node fleet
streams about 160 KB every two seconds to each viewer. The heap figure includes
the simulated nodes' own connections and buffers; the panel's share is lower.

## Metrics history

```sh
go test ./internal/sing-box-panel/store -run '^$' \
  -bench '^BenchmarkMetricsHistoryNinetyDays' -benchtime=1x -benchmem
```

One node with one sample every ten seconds for 90 days, 777,600 samples,
measured three times per range on Apple M5; single observations, not a
statistical claim.

| Query range | Elapsed | Allocated | Allocations |
| --- | ---: | ---: | ---: |
| 1 hour | 93–94 ms | 30.8 KB | 1,158 |
| 24 hours | 137–138 ms | 120 KB | 5,704–5,710 |
| 90 days | 4.52–4.59 s | 228–231 KB | 13,242–13,248 |

The full range stays expensive. The dashboard stream shares one read among its
viewers and runs only while one is open, so the cost is per refresh, not per
viewer.

## Not measured

- Separate hosts: the live fleet runs in one process on loopback, so network
  latency, loss and many TLS connections across real network interfaces were
  not exercised; real nodes also send larger reports than the simulated ones.
- Proxies: the fleet stream and fleet command stream behind a reverse proxy
  were not exercised; they use the same server-sent event framing and
  heartbeats as the node streams.
- Linux hardware: the Linux figures come from a container on the macOS
  machine; SQLite commit times differ by file system and disk.
