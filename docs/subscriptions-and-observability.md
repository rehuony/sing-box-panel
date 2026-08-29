# Subscriptions and observability

Public subscription output combines immutable applied runtime state with live,
administrator-managed authorization. Metrics are exposed only when a real
collector sample exists.

## Users, tokens, and default-deny grants

A subscription user is an administrative profile, not a panel login. Each
token belongs to one user, carries an operator label, and may be disabled,
revoked, expired, rotated, or deleted. Plaintext is returned only when a token
is created or rotated.

Users receive no nodes by default. Grants store exact node keys. Selecting an
entire source in the Web permission matrix expands only its current nodes;
future source nodes remain denied until explicitly granted. All of a user's
tokens inherit grant changes immediately.

The public endpoint remains:

```text
GET /sub/{token}/{channelId}
```

A valid token with no grants receives a format-correct empty subscription.
Disabled users and disabled, revoked, expired, deleted, or unknown tokens share
one public access-denied response. Disabling a token cannot remove credentials
already downloaded or invalidate a client's cache.

Responses have an ETag. Both `200` and `304` increment successful request
counts; only a response with a body increments body count and served bytes.
The panel stores last-use time but not client IP, User-Agent, or the full URL.

## Applied local nodes and versioned sources

Local nodes are derived only from the immutable startup bytes referenced by
the currently applied activation bundle. Draft, failed, and unapplied
configuration is never published. Rollback changes the applied bundle pointer
and therefore restores the matching local-node input without re-projecting the
current revision.

The inbound registry accepts only the exact reviewed releases `1.11.15`,
`1.12.25`, and `1.13.19`; other versions fail closed. Each converter publishes
only the client-usable inbound types available in that release and reports
stable diagnostics for server-only or unsupported types. Multi-user inbounds
become separate grantable credentials. A channel's required `public_host`
combines with each inbound `listen_port`; server certificate private keys,
ACME configuration, and listen-side fields are never copied.

The current exact inbound contracts are:

| Core | Convertible local inbound types |
| --- | --- |
| `1.11.15` | `mixed`, `socks`, `http`, `shadowsocks`, `vmess`, `trojan`, `hysteria`, `shadowtls`, `vless`, `tuic`, `hysteria2` |
| `1.12.25` | All 1.11.15 types plus `anytls` |
| `1.13.19` | All 1.12.25 types plus `naive` |

For all three versions, `direct`, `tun`, `redirect`, `tproxy`, and
`cloudflared` are explicitly unpublishable. Any other inbound type currently
produces an unsupported-type diagnostic; it is not guessed from a nearby
version.

Third-party sources accept sing-box JSON, Mihomo YAML, or plaintext/Base64
share-link lists. URI lists strictly recognize `ss`, `socks`/`socks5`,
`http`/`https`, `vmess`, `vless`, `trojan`, `hysteria`, `hy2`/`hysteria2`,
`tuic`, and `anytls`. Successful versions are append-only and store a raw
digest, normalized nodes, detected format, fetch time, and diagnostics. An
older successful version can be restored as current. Parse or fetch failure
never replaces the current version.

Remote refresh is a durable maintenance task. It is disabled on a schedule by
default; a configured interval is at least 15 minutes. Fetches allow at most
4 MiB, 20 seconds, and five redirects. DNS and every redirect target are
checked against the SSRF policy. Loopback, link-local, and private addresses
are denied unless `subscription.private_source_cidrs` explicitly allows them.
Public subscription requests never fetch upstream data.

Channel policy, user grants, token state, and source current-version pointers
take effect immediately. The public handler reads these values, the applied
startup artifact, and enabled source versions in one consistent SQLite read.

## Target renderers

The sing-box renderer preserves validated publishable nodes in sing-box JSON.
The cross-format renderers convert only their explicit current contracts:

| Target | Explicitly converted node types |
| --- | --- |
| Mihomo | `shadowsocks`, `socks`, `http`, `vmess`, `vless`, `trojan`, `hysteria2`, `tuic`, `anytls` |
| Loon | `shadowsocks`, `socks`, `http`, `vmess`, `vless`, `trojan`, `hysteria2`, `anytls` |

Unsupported types, transports, TLS shapes, networks, dependencies, or options
are omitted with stable positional diagnostics. Renderers never infer a field
mapping that is not implemented and tested.

## Durable logs

The log CLI and authenticated API expose bounded, sanitized metadata for
panel, core, task, and security events:

```sh
sing-box-panel log list
sing-box-panel log show LOG_ID
sing-box-panel log tail --follow
sing-box-panel log clear --before TIMESTAMP
sing-box-panel log delete LOG_ID
```

`GET /api/v1/logs/stream` is a durable server-sent event stream and accepts
`Last-Event-ID` for reconnection. `logs.retention_days` is enforced at server
startup and every 24 hours. Configuration bytes, subscription bodies, token
plaintext, URL credentials, and known secret fields are not stored as log
payloads.

## Limited monitoring and traffic

`process_only` reports child-process health without counters. `limited`
requires the final configuration to enable a
Clash API on a numeric loopback address with a non-empty secret. The panel
reads that configuration and never modifies startup bytes.

Apply, start, and restart first pass process health, then wait up to five
seconds for `/version` to report the exact selected core version. While the
process runs, the panel reads `/connections` every ten seconds and persists
memory, active connections, `uploadTotal`, and `downloadTotal`. A sample older
than 30 seconds is stale.

Counters are checkpointed by PID and OS start token. A restart opens a new
segment and preserves the UTC natural-month period total. A decrease inside
one process is stored as rejected diagnostic evidence and cannot lower totals.
Periods span `traffic.period_months`; `traffic.quota_gib=0` is unlimited.
Current periods aggregate across activation bundles while individual samples
retain bundle evidence.

```sh
sing-box-panel metrics show
sing-box-panel metrics watch
sing-box-panel traffic status
sing-box-panel traffic period list
sing-box-panel traffic period show PERIOD_ID
```

## Management surfaces

The Web interface and OpenAPI expose user profiles, grant matrices, token
statistics and lifecycle, channel preview as a selected user, source refresh
and version history, runtime operations, tasks, and traffic evidence. The
browser core import uses bounded multipart upload and a private staging
directory; it never asks a browser to submit a server-local path.

See [Configuration and runtime](configuration-and-runtime.md) for activation
semantics and [HTTP API and security](http-api-and-security.md) for management
authentication and request boundaries.
