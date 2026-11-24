# Panel and node architecture

This guide describes the product's two services: one panel and any number of
nodes. It owns service responsibilities, the panel–node protocol, node identity,
state ownership and failure behavior. Package placement follows the
[code layout](#code-layout) below; the [architecture guide](architecture.md)
lists each package's responsibility and the dependency direction.

- [Services](#services) and [topology](#topology)
- [Panel–node protocol](#panelnode-protocol) and [identity and trust](#identity-and-trust)
- [State ownership](#state-ownership), [commands and consistency](#commands-and-consistency) and [failure behavior](#failure-behavior)
- [Telemetry](#telemetry) and [subscriptions](#subscriptions)
- [Management API and Web UI](#management-api-and-web-ui) and [node console](#node-console)
- [Code layout](#code-layout), [packaging](#packaging) and [preserved guarantees](#preserved-guarantees)

## Services

| Service | Runs on | Responsibilities | Never does |
| --- | --- | --- | --- |
| Panel (`sing-box-panel`) | One host | Web UI and management API; node registry, enrollment and sessions; each node's editable configuration and revisions; the core version catalog and artifact cache; the node release of its own version that nodes update from; the subscription node pool, sources, channels, keys, templates, rendering and `/sub` delivery; aggregated metrics, traffic, runtime history and panel logs | Run sing-box, read node hosts' files, fetch anything on a node's behalf except core artifacts and its node release |
| Node (`sing-box-node`) | Every proxy host | One sing-box runtime: verified core installs, configuration check and activation, process supervision and recovery, rollback bundles; host metrics, public IP, read-only filesystem browsing, core logs and traffic counters; reports its host, configuration and runtime state; carries out panel commands; an optional loopback console for its enrollment | Accept connections from its panel or from other hosts, serve the panel's Web UI, hold subscription data or the panel's administrator credentials |

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
| `FetchRelease` | Bidirectional stream | Streams the panel's node release for the node's platform after its own signed handshake, to a running node or to the node's `update` command |

The session handshake runs inside `Connect` before any command:

1. The node sends `Hello` with its node ID, protocol version, build version, platform and instance ID, the random identity of its database.
2. The panel answers `Challenge` with a random nonce and the panel ID.
3. The node answers `Proof`, an Ed25519 signature over a fixed context string, the panel ID, the node ID and the nonce.
4. The panel verifies it against the enrolled public key and answers `Welcome` with the baseline metrics interval and the newest runtime transition it holds for that instance, or closes the stream with a status code.
5. The node sends a full `StateReport`, then the runtime transitions after the one the panel named. The panel accepts commands for the node once that report is recorded. A session whose first report does not arrive within 30 seconds, or cannot be recorded, ends, and the node reconnects with a new one.

A new authenticated session for a node replaces the previous one. The panel
records the new session and replaces the previous one in one step that cannot
fail between them, so a handshake that fails never leaves a live session
recorded as ended. gRPC keepalives detect dead sessions; the panel marks a node
offline when its session ends.

Frames are `oneof` envelopes:

| Direction | Frames |
| --- | --- |
| Panel → node | `Challenge`, `Welcome`, `Command { request_id, deadline, body }`, `Cancel { request_id }`, `TelemetryControl` |
| Node → panel | `Hello`, `Proof`, `Result { request_id, outcome or problem }`, `Event` (state report, runtime transition, metrics sample with traffic counters) |

The command bodies are typed: check configuration; enable and disable a core;
check and activate a startup artifact; start, stop, restart and roll back;
install, remove and list cores; list startup artifacts; read the runtime
status; browse and resolve filesystem paths; list, read, delete and clear
core log files; and update the node to the panel's release. A command that uses the configuration carries the node's saved
revision: its ID, file revision, SHA-256 and bytes. The panel follows a core log
by reading it again each second, so the protocol has no follow command.

Panel and node are released together. Within `v1` the contract changes only
additively, and `Hello` carries a minor protocol version. The panel accepts a
node with the same major version and records its build version and minor
version. Each command belongs to the minor version that added it, and the
panel refuses to send a command to a node whose session names an older minor
version, answering `node_upgrade_required` without contacting the node. The
contract is at minor version 1, which adds `FetchRelease`, `UpdateNode` and the
state report's last update attempt; every other command is in minor version 0.
A breaking change moves to a new package.

`FetchRelease` authenticates on its own, apart from `Connect`, so the node's
`update` command can use it while the node's service holds the session:

1. The node sends `Hello` with its node ID, protocol version and platform.
2. The panel answers `Challenge` with a random nonce and the panel ID.
3. The node answers `Proof`, an Ed25519 signature over a release context string, the panel ID, the node ID and the nonce; the context differs from the session's, so neither proof is accepted by the other handshake.
4. The panel sends `ReleaseHeader`, the release version, the binary's size and SHA-256 and the release's signed `SHA256SUMS` with `SHA256SUMS.sig`, then the binary for the node's platform in 64 KiB chunks.

## Identity and trust

**Panel identity.** On first use the panel generates a TLS key and a
self-signed certificate for direct node connections in its data directory, and
a stable panel ID in its database. Nodes that dial the panel directly pin the
certificate's public key. When the panel has an HTTPS external origin, such as
a reverse proxy with a publicly trusted certificate, the enrollment code names
that origin and selects system trust instead of a pin.

**Enrollment.** An administrator adds a node in the Web UI or with the panel
CLI by naming it. The panel issues a one-time enrollment token for a new node:
it stores the token's hash with a preassigned node ID, the name and an expiry
one hour away, and shows one command, `sing-box-node enroll --code <code>`,
where the code encodes the panel endpoint and base path, the trust mode or pin
and the token. No node exists yet: a node appears in lists, status and
commands only once a host enrolls. A new code for a name replaces an unused
one, a name an enrolled node uses is refused, an unused code can be canceled,
and an expired one is never accepted.
The node generates an Ed25519 key in its data directory (mode 0600) and calls
`Enroll` with the token, its public key and its platform. In one transaction
the panel consumes the token and creates the node with the preassigned ID, the
name, the key and the empty configuration; if another node took the name in
the meantime, enrollment fails and creates nothing. Enrolling again requires a
new code for the node and replaces the key. Deleting a node removes its key
and closes its session. The node's [console](#node-console) can exchange a code
too, while the node runs.

**Boundaries.**

- A node accepts no connection from its panel or from other hosts. Its local control surfaces are the sing-box Clash API and, when enabled, its console, both on loopback only.
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
| Version catalog, official and imported artifacts | Panel, which caches and verifies them, and removes official archives that no node reports installed and no install asked for in the last ten minutes | Nodes fetch them from the panel |
| Node release of the panel's version | Panel, which downloads both platforms' node binaries with the release's signed checksums into `release-cache/VERSION/`, verifies them with its embedded key and removes other versions | Nodes fetch it with `FetchRelease` and verify it again with their own embedded key |
| Executables and their last update attempt | Each service, which replaces its own binary when it may or stages a release for its root update helper | The node reports its last attempt in its state report |
| Runtime process, intents and recovery | Node | The panel shows the reported state |
| Host facts, metrics, public IP, filesystem | Node, live | The panel keeps bounded metrics history |
| Traffic counters | Node, cumulative across core restarts in a durable checkpoint | The panel computes each node's deltas, its periods from its own reset day, and its usage against its own quota |
| Core log files | Node, bounded and sanitized | The panel relays reads on demand and keeps no copy |
| Panel log and operation outcomes | Panel | Node events are recorded with their node ID |
| Subscriptions: node pool and its order, sources, channels, keys, templates | Panel | — |
| Panel settings, administrator credentials and sessions | Panel | — |
| Node settings: panel endpoint, trust, data directory and identity key; console settings and password hash | Node | — |

The panel's configuration for a node is authoritative. The first state report
of each session carries the configuration of the bundle the node runs, if any;
while the node's editable file is still the empty document it was created with,
that report seeds it, so a node enrolled again keeps its configuration. The seed
keeps the revision ID the node reports unless the panel already uses that ID,
so the node's inbounds stay published without a restart. After that, changes
flow only from the panel, and the node records each revision under the panel's
ID even when its document matches the one it runs. "Unapplied changes" means the panel's saved revision or selected core
differs from what the node reports as applied.

Both services use SQLite through `modernc.org/sqlite`, each with its own
numbered migrations from a shared baseline format; neither converts a database
written before that baseline.

## Commands and consistency

- Every command carries a request ID. The node records each result under its request ID for a day, so a repeated request returns the recorded result, and it runs runtime-changing commands through its prepare, perform and complete intent with a generation, so an interrupted command recovers after a node restart.
- The panel checks a revision against the exact Schema of the core it is for before sending it: the selected core for enable and check, and the core the node reports enabled for start and restart. The node accepts a revision ID only with the same bytes and rejects bytes that do not match the digest.
- A command that can change the node's state keeps only its deadline once sent: a browser that disconnects does not cancel a change the node already started.
- A management mutation for a node waits for that node's result within the command deadline and returns the completed result.
- An offline node rejects commands at once with a `node_offline` problem. Commands are never queued. A command without a deadline of its own has two minutes, batches included. A command to a node whose session has not yet reported its state waits for that report within its deadline, follows a session that replaces it, and fails as `node_offline` when the session ends first. Saving a node's configuration stays a panel-local write and works while the node is offline.
- When a session drops while a command is in flight, the API returns a `node_result_unknown` problem. The node's next state report settles the outcome. The UI shows reported state and never assumes success.
- A node reports the state a state-changing command left before it sends the command's result, so the panel's reported state is current when the API answers.
- Applying a configuration sends the revision, its digest and bytes, and the selected artifact identity. The node checks that the artifact is installed and verified, runs `sing-box check`, replaces the process through the existing preflight, keeps the previous bundle for rollback, and reports the new bundle. A failed check leaves the running process untouched.
- Installing a core first caches the archive on the panel, then sends its identity, size and a fetch grant that lasts only while the command runs. The node streams the archive with `FetchArtifact`, verifies its digest and the exact version the binary reports, publishes it atomically and records it under the same artifact ID as the panel.
- Updating a node first caches and verifies the panel's node release, then sends `UpdateNode` with the panel's version. The node fetches the release with `FetchRelease`, verifies its signature with its own embedded key, the binary's checksum, its build identity and platform and a version newer than its own, and answers how it left itself: up to date, restarting into the new version, staged for its root update helper, or installed and waiting for a manual restart. Restarting a node restarts its core, so its proxy is briefly interrupted. See [Packaging](#packaging) for how each kind of installation replaces its binary.

## Failure behavior

| Situation | Behavior |
| --- | --- |
| Panel unreachable | Nodes keep running their applied bundles. A node that restarts, or whose host reboots, starts its last applied bundle without the panel. Subscriptions stop updating until the panel returns |
| Node unreachable | The panel marks it offline with its last-seen time and keeps its last reported values visible, marked stale. Its inbounds stay published from the last applied revision, because a node that lost the panel may still carry traffic |
| Reconnect | The node sends a full state report first, then the runtime transitions the panel lacks. The panel records the report before accepting commands for the node |
| Lost telemetry | Traffic stays exact through cumulative counters and the node checkpoint: the first sample after a gap proves the traffic of the whole gap. Runtime transitions are the node's durable history and are sent again from where the panel's copy ends. Metrics samples are not buffered |
| Recreated node database | The node's new instance ID starts a new traffic segment and a new history sequence; the panel keeps what it recorded before |
| Failed update | A release that does not verify leaves the binary untouched. A system service that does not stay active after its helper installed a release is put back on the previous binary and restarted; the node reports the failed attempt with its code once it runs again, and a batch update stops there. A staged release whose helper never takes it is withdrawn after two minutes and reported as `helper_unavailable` |

## Telemetry

A node sends a state report when its state changes: after a state-changing
command, after a new runtime transition, and when a periodic check finds a
change such as a new public IP. It sends a metrics sample every ten seconds;
the panel keeps the sample's time unless it lies ahead of the panel's clock or
more than 20 seconds behind it, and then uses its own.
While a browser views a node's live metrics, the panel sends `TelemetryControl`
to raise that node to one sample per second and lowers it again when the last
viewer leaves. The nodes home and the status line read every node's baseline samples at once,
with rates averaged over ten seconds, from one fleet snapshot: `/fleet/stream`
shares it among every client and refreshes it every two seconds while anyone
streams it, sending each client every node once and then only the nodes that
changed, and `/fleet/status` answers from it while it is at most two seconds
old, or reads it anew and shares that read for two seconds. A panel write that
changes what the snapshot shows, such as a rename, saved node settings or a
command's result, makes the next read a new one, even while a read that began
before the write is still running; node reports reach readers with the next
shared read. Streaming the fleet does not
change how often nodes sample. Traffic counters ride
on metrics samples. The panel stores each node's last state report, keeping it
visible while the node is offline, and stores metrics history, traffic periods
and runtime history per node, with the existing retention settings. It keeps
each node's latest metrics sample, current period and newest accepted traffic sample
in memory, updated by every accepted sample, so a fleet snapshot is one query for
the nodes whatever their number; a period the panel does not hold, after a
rollover or a changed period setting, is read from the daily ledger once. The
subscription header reports the summed usage of every node's own current
period and no total, because quotas belong to nodes.

## Subscriptions

Rendering and `/sub` delivery stay in the panel and never contact a node.
Local inbounds come from each node's applied revision, converted on the panel:
the node reports the bundle it runs, and the panel publishes the revision it
names only when its bytes match the digest the node reported. A published
endpoint uses the node's public host override or, without one, the public IP the
node reports; a node with neither, or whose revision cannot be published,
publishes nothing and is named in a diagnostic while the other nodes publish.
Each inbound's pool ID and default display name include its node, and channel
membership refers to these stable IDs. The subscription traffic
header sums the usage of the nodes a subscription carries. Third-party sources belong to the panel and depend on no node.

## Management API and Web UI

The management API is rooted at `/api/v1`. Fleet resources sit at its root and
each node's resources under that node.

| Scope | Resources |
| --- | --- |
| Fleet | `/fleet/status` (the nodes with their metrics, filtered by state, tag and text, and paged) and `/fleet/stream` (the same nodes, then their changes), `/fleet/commands` (start, stop, restart, install a core version or update on listed nodes or every node with a tag, streaming each node's outcome), `/nodes` (list with each node's reported state, tags, whether an update is available and its last update attempt, a new node's enrollment code, rename, delete) and `/nodes/{node_id}/enrollment` (a new code for an enrolled node, and canceling an unused code), fleet traffic status with each node's usage and quota, core catalog and artifact cache, `/system/update` (the panel's version, the newest release, a check and the panel's own update), subscriptions, panel settings, panel logs, authentication |
| One node | `/nodes/{node_id}/…`: status, cores, configuration file and check, runtime controls, rollback, filesystem, core logs, metrics and their stream and history, dashboard stream, settings (public host, traffic quota and period, tags), update to the panel's release, traffic periods and runtime history |

A fleet command runs the single-node operation on each selected node, at most
four at once, records each as its own operation and streams each node's outcome
as it finishes, then a summary, so progress and partial success are visible; an
unknown node ID is that node's failure. An update runs one node at a time
instead: each node must come back reporting the panel's version within five
minutes, or report why its update failed, before the next starts, and the first
failure stops the batch, reporting the remaining nodes as skipped. It keeps running when its client
disconnects, so every selected node receives the command that was confirmed.
Installing a core version picks, for each node, the official catalog archive of
that version for the node's recorded platform, and caches each platform's
archive once, when its first node needs it; a node without a recorded platform,
a platform the version has no archive for, and a platform whose archive cannot
be cached fail on their own.

OpenAPI remains the authoritative HTTP contract. The Web UI follows
[`DESIGN.md`](../DESIGN.md)'s node-first shell: a status line that lists the
scope's windows, leads back to the fleet and, in a node's windows, to its
neighbours, and reads commands; the nodes home with the fleet's key figures and
one pane per node, a node's windows (overview, cores, configuration, logs,
settings) under a title strip that names the node, subscriptions, logs and
settings.

## Node console

A node's console is how an operator on the node host enrolls it without the
panel CLI's command line: it exchanges an enrollment code, re-enrolls the node
with a new code, unenrolls it, and shows the node's link with its panel. It
also shows read-only readings of the node itself: its core's state and version,
the host's CPU, load, memory and disk, its cumulative core traffic as last
counted, live connections while a core with a loopback Clash API runs, and its
detected public address.
It cannot edit the node's configuration or control its runtime; the panel does
that. [`api/node-openapi.yaml`](../api/node-openapi.yaml) is its HTTP contract.
The console's Web application, `web/apps/node`, is embedded only in the node
binary and served on the same loopback address.

- **Off by default.** `sing-box-node console enable` turns it on and generates a password when there is none; the settings and the password's Argon2id hash sit in `console.json` (mode 0600) in the node's data directory. An enabled console starts with `sing-box-node run`, and a node with an enabled console starts unenrolled and waits to be enrolled from it.
- **Loopback only.** The console listens only on a loopback IP address, and answers only requests whose `Host` names a loopback host, so a page under another name that resolves to the node cannot reach it. Operators on other machines use an SSH tunnel.
- **Same protections as the panel.** Login, its rate limits and concurrency bound, origin checks and the CSRF token come from the code the panel uses (`internal/webauth`). With no external origin to configure, the console always requires the request's own origin. Sessions live in memory for 12 hours; changing the password ends all of them, and so does restarting the node.
- **Settings apply at once.** Each request reads the console settings: a new password applies to the next login and a disabled console refuses every request; the listener closes when the node next starts.
- **Enrollment changes end the session.** The node ends its session, saves the new enrollment and connects to the panel it names; a failed exchange leaves the current enrollment in place. Unenrolling removes the enrollment; the panel shows the node offline until an administrator removes it, and the node's runtime keeps its applied configuration either way.

## Code layout

`cmd/` holds only each binary's `main.go`. Each service's packages live in
`internal/<binary>/`, named after the binary that runs them, and packages that
both binaries or the maintainer tools use sit directly in `internal/`, the
repository's only `internal` directory. No Go file sits outside a package
directory. The services meet only through `internal/nodeapi`.

```text
api/
  panel-openapi.yaml     browser ↔ panel HTTP contract
  node-openapi.yaml      browser ↔ node console HTTP contract
  proto/                 panel ↔ node buf module (buf.yaml, buf.gen.yaml)
    sbp/node/v1/         package sbp.node.v1
cmd/
  sing-box-panel/main.go panel entry: CLI, server and embedded Web UI
  sing-box-node/main.go  node entry: CLI, agent and console
internal/
  sing-box-panel/        panel service: imports shared packages, never the node
    app/                 the one composition of the panel's services
    fleet/               node registry, enrollment, gRPC sessions, command routing, presence, node updates
    releases/            the node release cache of the panel's version and the panel's own updates
    nodeconfig/          per-node editable file, revisions, check and apply through fleet
    corecache/           upstream catalog discovery, artifact cache and distribution
    schema/              exact-version Schema manifest and assets for the editor
    subscription/        node pool, sources and parsers, inbound conversion, channels, keys, templates, renderers
    telemetry/           metrics, traffic, runtime history and log relay
    auth/ settings/
    systemd/             the panel's unit templates and settings-file state for the shared lifecycle
    installation/        inspection, data moves and removal of the panel's files
    store/               panel SQLite
    httpapi/             management API, Web delivery and /sub
    cli/ server/ testutil/
  sing-box-node/         node service: imports shared packages, never the panel
    agent/               enrollment link, session, handshake, reconnect, command dispatch, state and telemetry reports
    httpapi/             loopback console API: login, enrollment and the link's status
    artifacts/           verified core installs
    runtime/             process supervision, intents, recovery, Clash API monitor
    corelogs/            bounded native process output
    host/                host metrics, public IP, read-only filesystem
    service/             the node's use cases: installs, checks, activation, supervision, recovery, core logs, filesystem
    store/               node SQLite: revisions, bundles, intents, recovery, command results
    systemd/             the node's unit templates and data-directory state for the shared lifecycle
    settings/ cli/

  Infrastructure: knows nothing of sing-box, subscriptions or the fleet
  atomicfile/ buildinfo/ console/ jsonstrict/ release/ selfupdate/
  procctl/               local service control and lease
  netguard/              which addresses count as public for outbound fetches and reported host addresses
  database/              SQLite opening, pragmas and data-directory locks shared by both stores
  logredact/             log redaction applied by both services
  webauth/               password hashing, login limits, origin and CSRF checks of both management APIs
  systemd/               one service's systemd lifecycle: units, dedicated account, status, control
  pathguard/             symlink-safe containment and shared-directory checks for service directories

  Shared domain: both services use these and never import a service
  configuration/         strict, lossless sing-box JSON
  coreartifact/          artifact identities and platform assets
  singbox/               exact-version support catalog and runtime compatibility
  nodeapi/               generated protocol code, protocol version and session proof

  layout/                the dependency-rule test over go list
tools/                   maintainer commands; singbox-support maintains panel Schema assets
web/                     pnpm workspace: apps/panel, apps/node (each embedded by its binary), packages/shared
```

`internal/layout` enforces the rules over `go list`: `cmd/` holds only the two
`main` packages, shared packages depend on neither service, each service
imports the other only through `internal/nodeapi`, and `cmd/sing-box-node`
embeds neither the panel's Web UI nor Schema assets, and `cmd/sing-box-panel`
does not embed the node console's. Maintainer tools are programs that nothing
imports and may use any package.

## Packaging

- One module and one release build both binaries for `linux/amd64` and `linux/arm64`; the signed `SHA256SUMS` covers both.
- The installer takes `--component panel|node`, or asks which when neither is given, reports what is already installed, and installs only that binary.
- Each service has its own systemd unit and dedicated account, installed by its own binary's `systemd` commands over one shared lifecycle. The node unit carries the privileges sing-box needs; the panel unit drops them because it no longer runs sing-box.
- A node reports its build version in each session; the panel records it and returns it with the node in the management API, with whether it differs from the panel's release.
- Both binaries embed the release key and update themselves only to a signed release newer than the running version. The panel updates from the latest stable GitHub release, from its settings or with `sing-box-panel update`; nodes update to the panel's own release, from the panel or with `sing-box-node update`, and never contact GitHub.
- How a binary is replaced depends on how it runs. A process that may write its executable, a user service or one in the foreground, verifies and installs the release itself; a user service then restarts its unit, and a foreground process keeps running the old version until it is restarted. A system service runs as its dedicated account with a read-only system, so it stages the verified release in `updates/VERSION/` of its data directory and writes `updates/request` naming the version. Its root update helper, the `NAME-update.path` and `NAME-update.service` units, copies the staged files beside the executable before reading them, verifies them with the installed binary's key, refuses anything older than or equal to the installed version, installs, restarts the service and, when it does not stay active, restores the previous binary and restarts again. The helper writes `updates/result` before each restart, and a refused release without one; the service reads it when it starts or, while it runs, within seconds, and reports it. See the [systemd packaging guide](systemd.md#update-helper).

## Preserved guarantees

Each documented guarantee sits with the service that owns it: authenticated
management with CSRF and origin checks and login limits on the panel and the
node console; SSRF-safe source acquisition on the panel; exact-version artifact
verification on both sides of distribution; immutable checked startup artifacts
and `sing-box check` before activation on the node; optimistic concurrency for
configuration and settings writes on the panel; and completed-result responses
for management mutations. Changing any of these needs an explicit product
decision.
