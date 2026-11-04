# Panel and node architecture

This guide is the target design for splitting the single-host binary into one
panel service and any number of node services. It owns service responsibilities,
the panel–node protocol, node identity, state ownership and failure behavior.
Package placement follows the [code layout](#code-layout) below. The
[architecture guide](architecture.md) describes the packages as they are today
and adopts this layout as the split lands.

- [Services](#services) and [topology](#topology)
- [Panel–node protocol](#panelnode-protocol) and [identity and trust](#identity-and-trust)
- [State ownership](#state-ownership), [commands and consistency](#commands-and-consistency) and [failure behavior](#failure-behavior)
- [Telemetry](#telemetry) and [subscriptions](#subscriptions)
- [Management API and Web UI](#management-api-and-web-ui)
- [Code layout](#code-layout), [packaging](#packaging) and [preserved guarantees](#preserved-guarantees)

## Services

| Service | Runs on | Responsibilities | Never does |
| --- | --- | --- | --- |
| Panel (`sing-box-panel`) | One host | Web UI and management API; node registry, enrollment and sessions; each node's editable configuration and revisions; the core version catalog and artifact cache; subscription sources, manual nodes, channels, keys, templates, rendering and `/sub` delivery; aggregated metrics, traffic, runtime history and panel logs | Run sing-box, read node hosts' files, fetch anything on a node's behalf except core artifacts |
| Node (`sing-box-node`) | Every proxy host | One sing-box runtime: verified core installs, configuration check and activation, process supervision and recovery, rollback bundles; host metrics, public IP, read-only filesystem browsing, core logs and traffic counters; reports its host, configuration and runtime state; carries out panel commands | Listen for management connections, serve a Web UI, hold subscription data or administrator credentials |

The panel host runs no sing-box. A panel host that should also carry proxy
traffic runs a node that connects to the panel over loopback, so every host is
managed through the same path.

## Topology

```text
   Browser (panel Web UI)                 Subscription clients
          │ HTTPS /api/v1                       │ HTTPS /sub/{token}/{channel}
          ▼                                     ▼
 ┌──────────────────────────── panel ────────────────────────────┐
 │ httpapi · fleet · nodeconfig · corecache · subscription ·       │
 │ telemetry · panel store (SQLite)                                │
 └───────────────▲──────────────────────▲──────────────────────────┘
   gRPC over TLS │ one session per node │  nodes dial out; panel never dials in
          ┌──────┴──────┐        ┌──────┴──────┐
          │   node A    │        │   node B    │   …
          │ agent ·     │        │             │
          │ runtime ·   │        │             │
          │ node store  │        │             │
          └──────┬──────┘        └─────────────┘
                 │ exec; Clash API on loopback
              sing-box
```

The panel serves the Web application and the node service on one port, with
the node service under the panel's base path. It tells connections apart by
their first bytes: a TLS handshake is a node dialing directly, the HTTP/2
cleartext preface from loopback is a reverse proxy on the panel host
forwarding nodes with `grpc_pass`, and anything else is the Web application.
Cleartext HTTP/2 from any other peer is closed. Behind a reverse proxy with a
publicly trusted certificate, nodes reach the panel through the proxy's HTTPS
origin, like browsers do.

## Panel–node protocol

The contract is a protobuf package, `sbp.node.v1`, in
`api/proto/sbp/node/v1/`; `api/proto/` is the buf module with its lint and
generation settings. The panel serves it with `google.golang.org/grpc`.
Generated Go code in `internal/nodeapi` is committed and regenerated with
`make generate` (`go tool buf generate`); `make check` fails when the generated
code differs from the contract.

| RPC | Shape | Purpose |
| --- | --- | --- |
| `Enroll` | Unary | Exchanges a one-time enrollment token and the node's public key for a node identity |
| `Connect` | Bidirectional stream | The control session: handshake, commands, results and events |
| `FetchArtifact` | Server stream | Streams one cached core artifact to a node, authorized by a grant issued in the session |

The session handshake runs inside `Connect` before any command:

1. The node sends `Hello` with its node ID, protocol version, build version and platform.
2. The panel answers `Challenge` with a random nonce and the panel ID.
3. The node answers `Proof`, an Ed25519 signature over a fixed context string, the panel ID, the node ID and the nonce.
4. The panel verifies it against the enrolled public key and answers `Welcome`, or closes the stream with a status code.

A new authenticated session for a node replaces the previous one. gRPC
keepalives detect dead sessions; the panel marks a node offline when its
session ends.

Frames are `oneof` envelopes:

| Direction | Frames |
| --- | --- |
| Panel → node | `Challenge`, `Welcome`, `Command { request_id, deadline, body }`, `Cancel { request_id }`, `TelemetryControl` |
| Node → panel | `Hello`, `Proof`, `Result { request_id, outcome or problem }`, `Event` (state report, runtime transition, metrics sample, traffic counters, log batch) |

The command bodies are check configuration, apply configuration, start, stop,
restart, roll back, install core, remove core, browse and resolve filesystem
paths, read core log files and follow core logs. A follow command emits events
tagged with its request ID until the panel cancels it.

Panel and node are released together. Within `v1` the contract changes only
additively, and `Hello` carries a minor protocol version. The panel accepts a
node with the same major version, records its build version, and rejects with a
clear problem any command the node's minor version does not support. A
breaking change moves to a new package.

## Identity and trust

**Panel identity.** On first use the panel generates a TLS key and a
self-signed certificate for direct node connections in its data directory, and
a stable panel ID in its database. Nodes that dial the panel directly pin the
certificate's public key. When the panel has an HTTPS external origin, such as
a reverse proxy with a publicly trusted certificate, the enrollment code names
that origin and selects system trust instead of a pin.

**Enrollment.** An administrator adds a node in the Web UI or with the panel
CLI. The panel stores the node record and a hash of a one-time enrollment token
that expires after one hour. It shows one command, `sing-box-node enroll --code <code>`,
where the code encodes the panel endpoint and base path, the trust mode or pin
and the token.
The node generates an Ed25519 key in its data directory (mode 0600) and calls
`Enroll` with the token, its public key and its platform. The panel stores the
public key and consumes the token. Enrolling again requires a new code and
replaces the key. Deleting a node removes its key and closes its session.

**Boundaries.**

- A node has no management listener. Its only local control surface is the sing-box Clash API on loopback.
- A node validates every command itself. Filesystem browsing stays read-only and non-recursive; artifacts must match the digest the panel supplies; configurations pass `sing-box check` with the exact selected core before activation.
- The panel sends a node only that node's configuration, artifacts and grants. Subscription tokens, source contents, administrator credentials and other nodes' data never leave the panel.
- The panel stores node public keys, never node private keys. A compromised node cannot impersonate another node or read panel data.

## State ownership

| State | Owner | The other side |
| --- | --- | --- |
| Editable configuration file and revisions, per node | Panel | The node receives exact bytes only to check or apply them |
| Selected core per node | Panel (desired) | The node reports the installed and running artifacts |
| Checked startup artifact, applied bundle and rollback bundle | Node, which keeps the bytes of the applied and previous bundles | The panel records their digests and the node's evidence |
| Installed core files | Node | The panel records which node has which artifact |
| Version catalog, official and imported artifacts | Panel, which caches and verifies them | Nodes fetch them from the panel |
| Runtime process, intents and recovery | Node | The panel shows the reported state |
| Host facts, metrics, public IP, filesystem | Node, live | The panel keeps bounded metrics history |
| Traffic counters | Node, with a durable cumulative checkpoint | The panel computes deltas, periods and quota |
| Core log files | Node, bounded and sanitized | The panel relays reads on demand and keeps no copy |
| Panel log and operation outcomes | Panel | Node events are recorded with their node ID |
| Subscriptions: sources, manual nodes, channels, keys, templates, order and visibility | Panel | — |
| Panel settings, administrator credentials and sessions | Panel | — |
| Node settings: panel endpoint, trust, data directory and identity key | Node | — |

The panel's configuration for a node is authoritative. On first connection a
node reports the configuration it already runs, if any, and that report seeds
the panel's editable file for the node. After that, changes flow only from the
panel. "Unapplied changes" means the panel's saved revision or selected core
differs from what the node reports as applied.

Both services use SQLite through `modernc.org/sqlite`, each with its own
baseline schema. Neither has an upgrade path from the single-host format.

## Commands and consistency

- Every command carries a request ID. The node runs runtime-changing commands through its prepare, perform and complete intent with a generation, so a repeated request ID returns the recorded result and an interrupted command recovers after a node restart.
- A management mutation for a node waits for that node's result within the command deadline and returns the completed result, as single-host mutations do today.
- An offline node rejects commands at once with a `node_offline` problem. Commands are never queued. Saving a node's configuration stays a panel-local write and works while the node is offline.
- When a session drops while a command is in flight, the API returns a `node_result_unknown` problem. The node's next state report settles the outcome. The UI shows reported state and never assumes success.
- Applying a configuration sends the revision, its digest and bytes, and the selected artifact identity. The node checks that the artifact is installed and verified, runs `sing-box check`, replaces the process through the existing preflight, keeps the previous bundle for rollback, and reports the new bundle. A failed check leaves the running process untouched.
- Installing a core sends the artifact identity, its digest and a short-lived fetch grant. The node streams the artifact with `FetchArtifact`, verifies it and publishes it atomically.

## Failure behavior

| Situation | Behavior |
| --- | --- |
| Panel unreachable | Nodes keep running their applied bundles. A node that restarts, or whose host reboots, starts its last applied bundle without the panel. Subscriptions stop updating until the panel returns |
| Node unreachable | The panel marks it offline with its last-seen time and keeps its last reported values visible, marked stale. Its inbounds stay published from the last applied revision, because a node that lost the panel may still carry traffic |
| Reconnect | The node sends a full state report first, then buffered events. The panel reconciles observed state before accepting commands for the node |
| Lost telemetry | Traffic stays exact through cumulative counters and the node checkpoint. Runtime transitions wait in a bounded node outbox, and an overflow is reported as a dropped count. Metrics samples are not buffered beyond a short window |

## Telemetry

A node sends a state report when its state changes and a metrics sample at a
low baseline interval. While a browser views a node's live metrics, the panel
sends `TelemetryControl` to raise that node to one sample per second and lowers
it again when the last viewer leaves. The fleet overview uses baseline samples.
Traffic counters ride on metrics samples. The panel stores metrics history,
traffic periods and runtime history per node, with the existing retention
settings.

## Subscriptions

Rendering and `/sub` delivery stay in the panel and never contact a node.
Local inbounds come from each node's applied revision, converted on the panel.
A published endpoint uses the node's public host override or, without one, the
public IP the node reports. Each local inbound's provenance names its node, and
channel membership refers to node-scoped stable IDs. Third-party sources and
manual nodes behave as today.

## Management API and Web UI

The management API stays at `/api/v1` and is redesigned in place without
compatibility, because the product has no stable release yet.

| Scope | Resources |
| --- | --- |
| Fleet | `/nodes` (list, add with enrollment code, rename, delete), fleet dashboard, core catalog and artifact cache, subscriptions, panel settings, panel logs, authentication |
| One node | `/nodes/{node_id}/…`: status, cores, configuration file and check, runtime controls, rollback, filesystem, core logs, metrics, traffic and runtime history |

OpenAPI remains the authoritative HTTP contract. The Web UI follows
[`DESIGN.md`](../DESIGN.md)'s node-first shell: a fleet overview, a node
workspace (dashboard, cores, configuration, logs), subscriptions and settings.

## Code layout

`cmd/` holds only each binary's `main.go`. Each service's packages live in
`internal/<binary>/`, named after the binary that runs them, and packages that
both binaries or the maintainer tools use sit directly in `internal/`, the
repository's only `internal` directory. No Go file sits outside a package
directory. The services meet only through `internal/nodeapi`.

```text
api/
  openapi.yaml           browser ↔ panel HTTP contract
  proto/                 panel ↔ node buf module (buf.yaml, buf.gen.yaml)
    sbp/node/v1/         package sbp.node.v1
cmd/
  sing-box-panel/main.go panel entry: CLI, server and embedded Web UI
  sing-box-node/main.go  node entry: CLI and agent
internal/
  sing-box-panel/        panel service: imports shared packages, never the node
    app/                 the one composition of the panel's services
    fleet/               node registry, enrollment, gRPC sessions, command routing, presence
    nodeconfig/          per-node editable file, revisions, check and apply through fleet
    corecache/           upstream catalog discovery, artifact cache and distribution
    schema/              exact-version Schema manifest and assets for the editor
    subscription/        sources, manual nodes, inbound conversion, channels, keys, templates, renderers
    telemetry/           metrics, traffic, runtime history and log relay
    auth/ settings/
    store/               panel SQLite
    httpapi/             management API, Web delivery and /sub
    cli/ server/ testutil/
  sing-box-node/         node service: imports shared packages, never the panel
    agent/               session, handshake, reconnect, command dispatch, telemetry outbox
    artifacts/           verified core installs
    runtime/             process supervision, intents, recovery, Clash API monitor
    corelogs/            bounded native process output
    host/                host metrics, public IP, read-only filesystem
    store/               node SQLite: bundles, intents, recovery, counters
    settings/ cli/ server/

  Infrastructure: knows nothing of sing-box, subscriptions or the fleet
  atomicfile/ buildinfo/ console/ jsonstrict/ release/ selfupdate/
  procctl/               local service control and lease
  database/              SQLite opening, pragmas and data-directory locks shared by both stores
  logredact/             log redaction applied by both services
  installation/ systemd/

  Shared domain: both services use these and never import a service
  configuration/         strict, lossless sing-box JSON
  coreartifact/          artifact identities and platform assets
  singbox/               exact-version support catalog and runtime compatibility
  nodeapi/               generated protocol code, protocol version and session proof

  layout/                the dependency-rule test over go list
tools/                   maintainer commands; singbox-support maintains panel Schema assets
web/                     panel React application
```

`internal/layout` enforces the rules over `go list`: `cmd/` holds only the two
`main` packages, shared packages depend on neither service, each service
imports the other only through `internal/nodeapi`, and `cmd/sing-box-node`
embeds neither the Web UI nor Schema assets. Placements
that exist only until the panel stops running sing-box in process, and until
each service has its own installer, are listed in the
[architecture guide](architecture.md#package-layout).

## Packaging

- One module and one release build both binaries for `linux/amd64` and `linux/arm64`; the signed `SHA256SUMS` covers both.
- The installer takes `--component panel|node` and installs only that binary.
- Each service has its own systemd unit. The node unit carries the privileges sing-box needs; the panel unit drops them because it no longer runs sing-box.
- A node reports its build version. The panel flags nodes whose version differs from its own and can tell a node to self-update to the panel's release through the existing signed update path.

## Preserved guarantees

The split keeps the documented single-host guarantees and moves each to its
owner: authenticated management with CSRF and origin checks and login limits on
the panel; SSRF-safe source acquisition on the panel; exact-version artifact
verification on both sides of distribution; immutable checked startup artifacts
and `sing-box check` before activation on the node; optimistic concurrency for
configuration and settings writes on the panel; and completed-result responses
for management mutations. Changing any of these still needs an explicit product
decision.
