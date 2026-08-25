# Subscriptions and observability

Public subscription output and operational observations are bound to durable
panel state. The service does not expose uncommitted edits or invent metrics
when no collector evidence exists.

## Subscription resources

Channels, sources, and tokens can be managed through the CLI, management API,
and Web interface:

```sh
sing-box-panel subscription channel --help
sing-box-panel subscription source --help
sing-box-panel subscription token --help
```

A channel defines one public renderer and its filtering configuration. A source
stores an attached third-party snapshot. A token is either bound to one channel
or left unbound so the request must select a format. Tokens have explicit
rotation, revocation, and expiry state. Plaintext is returned only when a token
is created or rotated.

The public endpoint is:

```text
GET /sub/{token}?format=sing-box|mihomo|loon
```

`format` may be omitted only when the token is bound to exactly one channel.
The sing-box, Mihomo, and Loon renderers omit conversions that cannot be proven
safe and return positional diagnostics instead of guessing.

## Applied-snapshot publication

The public endpoint serves only data frozen into the currently applied
activation bundle. During bundle preparation, enabled source snapshots are
merged into a publication-only copy of the final startup document. Duplicate
tags, unresolved dependencies, channel filtering, and stable ordering are
validated before the bundle can be applied.

Publication input does not modify the runtime startup bytes. Configuration,
channel, and source edits stay private until a later apply succeeds. Refreshing
a source cannot mutate public output from an already applied bundle. Token
revocation and expiry, however, take effect immediately.

`subscription source refresh` currently accepts a strict snapshot through
`--file PATH` or `--file -`. Automatic remote-source retrieval and public
address detection are not implemented.

## Durable logs

The `log` commands and authenticated log API expose bounded, sanitized event
metadata for panel, core, task, and security events. Full configuration,
subscription bodies, tokens, and known secret fields are not stored as log
payloads.

```sh
sing-box-panel log list
sing-box-panel log show LOG_ID
sing-box-panel log tail --follow
sing-box-panel log clear --before TIMESTAMP
sing-box-panel log delete LOG_ID
```

The management API provides durable server-sent events at
`GET /api/v1/logs/stream`. Clients can reconnect with `Last-Event-ID` without
requiring an in-memory event buffer. Collection clearing is filtered and
bounded; single-record deletion is explicit.

`logs.retention_days` is enforced once during server startup and every 24
hours while the server remains active.

## Metrics and traffic

Metrics and traffic are collector-backed:

```sh
sing-box-panel metrics show
sing-box-panel metrics watch
sing-box-panel traffic status
sing-box-panel traffic period
```

Until a collector has stored evidence for the exact applied bundle, these
operations report an explicit unavailable reason instead of fabricated zero
counters. A traffic period's identity and time boundaries are immutable, and
its counter updates must be monotonic.

A live sing-box metrics and traffic collector is not currently connected to
the server. Treat unavailable output as missing evidence, not as zero usage.

See [Configuration and runtime](configuration-and-runtime.md) for activation
bundle semantics and [HTTP API and security](http-api-and-security.md) for
authentication of management and public routes.
