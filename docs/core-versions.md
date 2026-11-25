# Core versions

Release discovery, installation, visual configuration and inbound conversion are
separate capabilities. Visual configuration and inbound conversion follow the
core's release line; a core outside every line can still check and run raw JSON.

- [Install and select](#install-and-select-a-version), [choose a build](#multiple-installations-of-one-version), or [import](#import-a-local-archive)
- [Catalog cache](#catalog-cache), [verification boundaries](#checks-and-recorded-hashes), and [runtime switching](#switching-and-runtime-state)
- [Release lines](#release-lines) and [CLI / HTTP migration](#cli-and-http-migration)
- [The reviewed 1.13 schema](#reviewed-113-schema)

## Install and select a version

Open version management in the Web UI. The available list shows cached official
downloads; the installed list shows local artifacts. Use “Check GitHub for updates”
to refresh available versions, Download to install an exact release, and Enable
on an installed artifact to select it. Remove unused installations there when
they are no longer needed.

Installation uses the stored catalog and never substitutes a newer or older
release. Opening version management initializes the catalog if it is missing;
use the explicit refresh action when a release is absent from an existing cache.
The Web client stops waiting for a refresh after one minute and shows the timeout
with Try again in place of the list, as it does for any list it cannot read.
The selected release must have a complete `MAJOR.MINOR.PATCH` version.

Official packages use the static musl variant for Linux amd64 or arm64.
Installation inspects the actual executable; the environment must be able to
execute its architecture. Runtime enablement requires the node's Linux
architecture. Selecting an architecture does not provide emulation.

## Multiple installations of one version

Official and imported builds of one exact version may coexist. Select the intended
installed artifact in version management; its source and installation identity
distinguish it from another build of the same version. HTTP operations address
that exact artifact by its existing internal ID. Version text alone does not
change which artifact a management request selects.

## Import a local archive

The Web import dialog accepts one `.tar.gz`/`.tgz` file and an exact version.
Recognized filenames suggest an editable version and reject a clearly incompatible
architecture or non-musl package. Custom filenames require the operator to specify
the version. Uploads do not calculate or submit an expected checksum.
The persisted source name `user_verified` means an administrator-imported archive;
it is retained for existing records and does not mean publisher authentication.

## Catalog cache

Discovery reads stable releases from the fixed `SagerNet/sing-box` GitHub
repository. Only `sing-box-VERSION-linux-{amd64,arm64}-musl.tar.gz` packages enter
the catalog, with at most one per version and architecture. Drafts, prereleases,
invalid names and unrelated assets are filtered. There is no plain/glibc fallback.
Missing, malformed or conflicting digest metadata does not hide a package or
prevent installation.

Discovery reads pages of 50, up to 100 pages, with a 32 MiB page limit, a 256 MiB
aggregate limit and a three-minute deadline. JSON depth and duplicate keys are
checked. A successful complete catalog and its HTTP validator remain in SQLite
until another complete refresh succeeds. Failed refreshes retain the last usable
cache. `github.catalog_refresh_interval_hours` controls refresh scheduling, not
cache expiration. Scheduled refreshes honor the interval; the explicit Web
refresh action bypasses it.
The running server checks in the background without delaying HTTP readiness.

Opening Web version management reads the cache and initializes it only if missing.
“Check GitHub for updates” forces a refresh. Cached downloads and installed versions
remain visible during refresh. The server-side client honors proxy environment
variables and the optional GitHub token in panel settings; credentials do not
bypass rate limits. See [database compatibility](getting-started.md#database-compatibility).

## Checks and recorded hashes

Installation still enforces download/file size bounds, safe archive extraction,
ordinary files and trusted paths, Linux ELF architecture, static musl linking and
the reported `with_musl` feature. `sing-box version` must report the requested exact
version. Configuration eligibility is determined by the real selected core's
`sing-box check`.

Archive and binary SHA-256 values are calculated during installation for content
paths, deduplication and existing installation identifiers. They are recorded
metadata. Official installation does not require or compare an upstream digest;
local import does not accept an expected digest. Check, enable, start, restart,
rollback, recovery, execution copies and current-link reconciliation do not compare
core file contents with those recorded values. Hash folder names describe storage layout,
not a runtime authenticity guarantee. Reused executables still undergo platform
and exact-version inspection at installation.

Three other verification boundaries remain intact:

- Configuration snapshot digests bind the checked JSON to the bytes activated.
- A configuration schema's SHA-256 identifies the exact schema the panel serves
  and checks configurations with.
- Panel release signatures and updater checks authenticate the panel distribution.

Official downloads rely on GitHub HTTPS and the fixed repository/release location.
Imported cores rely on the administrator's choice. Recorded hashes do not provide
publisher authenticity or ongoing tamper detection.

## Installed file layout and upgrades

Installed archives and executables share one content directory:

```text
artifacts/
  current -> sha256/<archive-sha256>/sing-box
  sha256/
    <archive-sha256>/
      artifact.tar.gz
      sing-box
```

The full archive digest remains the content address; there is no two-character
prefix directory. Different builds of the same version retain distinct installation
identities, and identical archives reuse their content directory.

The former `artifacts/sha256/<first-two-characters>/<archive-sha256>/` layout is
unsupported. Opening or installing into such a store fails with an instruction to
reinitialize the instance data directory and reinstall cores. This is an explicit
incompatible storage change: there is no automatic migration, fallback lookup,
or deletion of old files. Before upgrading an instance with that layout, export
the configuration and any other state you need to retain, stop the panel, and
reinitialize its data directory using the documented [instance cleanup](cli.md#instance-files-and-cleanup)
and initialization workflows. Reinitialization discards the previous database
history; merely moving the core files leaves stale paths in the database.

## Switching and runtime state

Web Enable preserves the stopped/running state.
Panel startup initializes an absent sing-box configuration to `{}`, so the first
installed version can be enabled without manually saving an empty document.
Existing saved configuration is preserved. Installation and version selection
do not create or rewrite that document. An unexpectedly absent configuration
returns `409 configuration_not_saved`; the Web notification links to Configuration.
Selecting while stopped does not start a process. Selecting while running performs
preflight before replacing the process. A failed preflight retains the old
selection, running process and saved JSON. Switching never migrates, fills or
rewrites configuration fields. Start, restart and rollback also check the exact
version and the configuration snapshot before execution.

Stop retains the selection. Web Disable stops the selected artifact and clears
selection only after success. Removal rejects artifacts still referenced by
startup records or activation bundles. A committed activation records the selected
installation; `<data_dir>/artifacts/current` is a relative convenience symlink
reconciled from that record after startup or recovery. Execution uses the stored
binary path and safe-file checks, not the convenience link as an authority.

## Release lines

A core's exact `MAJOR.MINOR.PATCH` version belongs to the newest line whose first
version is not above it, so later patch and minor releases need no panel release.

| Line | First version | Configuration schema | Inbound family |
| --- | --- | --- | --- |
| 1.13 | 1.13.19 | [Reviewed schema](#reviewed-113-schema) embedded in the panel | 1.13 |
| 1.14 | 1.14.0 | The installed binary's `sing-box schema` output | 1.14 |

A version below 1.13.19 has no line: it uses Advanced JSON editing, and its
node's inbounds are not published to subscriptions. Raw checking and running do
not depend on a line.

For the 1.14 line the panel asks the node for the schema its installed core
prints, once per archive SHA-256, and keeps up to 16 of them in memory. The node
must be online and run protocol 1.2 or later for that first request. The panel
adds its `x-panel` presentation annotations to either schema; they never widen
accepted fields or values. Before a configuration reaches a node, the panel checks
it against the core's schema when it can obtain one; the core's own
`sing-box check` remains authoritative. The browser renders the schema and does
not validate against it.

The panel supplies the five upstream legacy DNS compatibility switches only for
the 1.13 line, during both check and run. This preserves old DNS servers, FakeIP
options, outbound DNS rules, domain strategy and missing resolver behavior without
changing JSON. Their deprecation warnings remain visible. Before upgrading to
1.14, explicitly migrate those forms using the
[upstream migration guidance](https://sing-box.sagernet.org/migration/).

## CLI and HTTP migration

The `core` CLI group has been removed. Move catalog refresh, installation,
import, selection, removal, and core lifecycle operations to the Web UI.
Existing automation should use authenticated HTTP management endpoints.
Regenerate shell completions after upgrading; removed commands have no aliases.

HTTP management endpoints continue to locate objects by internal asset or artifact
ID. The multipart import endpoint no
longer accepts a `sha256` field and the catalog endpoint no longer accepts an
`installable` query. Update API clients to the [OpenAPI contract](../api/panel-openapi.yaml).

## Reviewed 1.13 schema

sing-box 1.13 has no `schema` command, so the panel embeds a reviewed source
definition, [`1.13.json`](../internal/sing-box-panel/schema/1.13.json), for the
whole 1.13 line. It follows the option types of the tags below. Their
configuration fields are identical; later 1.13 patch releases use the same
schema, and their runtime fixes remain the core's responsibility.

### Sources and patch comparison

| Release | Pinned commit | Configuration documentation | Source types and migration notes |
| --- | --- | --- | --- |
| 1.13.19 | `b5ebaa1fc0f2b94256180b95468e73ef53caa27d` | [tag documentation](https://github.com/SagerNet/sing-box/tree/v1.13.19/docs/configuration) | [option](https://github.com/SagerNet/sing-box/tree/v1.13.19/option), [migration](https://github.com/SagerNet/sing-box/blob/v1.13.19/docs/migration.md) |
| 1.13.20 | `56f91dfeabd6f4edbd437dfcc1e5b0ebc856b778` | [tag documentation](https://github.com/SagerNet/sing-box/tree/v1.13.20/docs/configuration) | [option](https://github.com/SagerNet/sing-box/tree/v1.13.20/option), [migration](https://github.com/SagerNet/sing-box/blob/v1.13.20/docs/migration.md) |
| 1.13.21 | `628cb31ffa79cffffd34c2f9cde6cae044e4fc12` | [tag documentation](https://github.com/SagerNet/sing-box/tree/v1.13.21/docs/configuration) | [option](https://github.com/SagerNet/sing-box/tree/v1.13.21/option), [migration](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/migration.md) |

All 50 `option/*.go` files are byte-identical across these three tags. The
[17-commit comparison](https://github.com/SagerNet/sing-box/compare/v1.13.19...v1.13.21)
adds or removes no configuration fields. The registries under `include/` are also
unchanged. The current website is auxiliary: its newer certificate providers, DNS
response actions, protocols and endpoint fields must not be copied into this
definition.

The 1.13.20 patch fixes inverted DNS rules whose address conditions are deferred
until after lookup. It also corrects the DNS rule documentation: destination
address conditions (`ip_cidr`, `ip_is_private`, `ip_accept_any`) belong to the same
OR group as domain conditions; other condition groups combine with AND. The
[matching implementation and regression tests](https://github.com/SagerNet/sing-box/blob/v1.13.20/route/rule/rule_dns_address_filter_test.go)
change runtime semantics, not field shapes. 1.13.19 retains its original behavior;
selecting another patch version never rewrites a rule to emulate that fix.
The [1.13.21 changelog](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/changelog.md)
also records WebSocket/TUN fixes. Their behavior remains the exact core's responsibility.

### Constraints and presentation

Optional in this schema means that the JSON decoder allows omission. It does not
promise that a runnable instance can omit credentials, a server, a referenced tag,
a certificate or a route action's effective options. Those contextual requirements
are summarized below and enforced by the real core. In particular, configuration
checking does not prove that an external ACME server, DNS resolver or tunnel peer
is reachable at runtime.

The schema also accepts explicit `null` for optional top-level sections and plain
log fields, matching the core's Go decoder. An empty or `null` `log.level` uses
the core's default level. These values remain unchanged through validation and
unrelated visual edits; custom nested types keep their own decoding constraints.

The source definition contains semantic JSON constraints and provenance. The panel
adds its `x-panel` presentation annotations when it prepares the schema; the
result's SHA-256 is the schema's identity.

### Section and conditional-field review

| Section / fields | 1.13 shape and associated constraints | Exact reference |
| --- | --- | --- |
| `log` | Optional object; level is trace/debug/info/warn/error/fatal/panic. Output is a path; disabled/timestamp are booleans. | [log](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/log/index.md) |
| `dns.servers[]` | Typed transports and the legacy address format coexist. Missing/empty/legacy `type` selects the legacy decoder. Typed remote transports use server/server_port and their dial/TLS options; a domain-valued server requires an appropriate resolver relationship. | [DNS source](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/dns.go), [legacy servers](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/dns/server/legacy.md) |
| `dns.rules[]` | Default or recursive logical rules; logical mode is and/or. Actions are route, route-options, reject, predefined. Route is the omitted action. Server/rule-set/outbound references must resolve. Predefined answers use textual or base64 DNS records; query type accepts numeric RR types or recognized uppercase names. | [DNS rules](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/dns/rule.md), [actions](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/dns/rule_action.md) |
| `dns.fakeip`, `independent_cache`, legacy `outbound` rule items | Retained 1.13 forms. FakeIP ranges must be valid IP prefixes. The five deprecated DNS switches are enabled by the panel for the 1.13 line; warnings remain visible. | [deprecated features](https://github.com/SagerNet/sing-box/blob/v1.13.21/experimental/deprecated/constants.go), [migration](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/migration.md) |
| `route.rules[]`, `route.rule_set[]` | Default/logical matching and route, route-options, direct, bypass, reject, hijack-dns, sniff, resolve actions. Inline/local/remote rule sets have a nonempty tag. Logical rules require rules and mode. Non-inline formats are source/binary, inferred from path/URL where supported. Empty route-options are rejected by core. | [route actions](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/rule_action.go), [rule sets](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/rule_set.go) |
| Reject / TLS fragmentation actions | Reject method is default/drop/reply; drop and no_drop=true cannot combine. `tls_fragment` and `tls_record_fragment` cannot both be true. Dial settings in a direct action do not include detour. | [action decoder](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/rule_action.go) |
| Shared listen/dial fields | Listen ports are uint16. Listable fields accept one value or an array. Network is tcp/udp. `domain_resolver` accepts a tag string or object with required server. Domain strategy permits as_is/prefer_ipv4/prefer_ipv6/ipv4_only/ipv6_only. Durations, addresses, marks, interfaces and detour/resolver references require core parsing and environment checks. | [listen](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/shared/listen.md), [dial](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/shared/dial.md) |
| Inbound/outbound `tls` | Separate server/client objects retain inline certificate/key, client-auth, ECH, Reality, ALPN, TLS versions, uTLS and fragmentation as applicable. Server private keys and ACME credentials never become subscription client fields. TLS versions are 1.0–1.3; enabled TLS requires protocol-appropriate certificates or Reality credentials. Reality handshake/private_key are server fields; public_key is client-side. ECH and kTLS have transport/kernel restrictions checked by the core. | [TLS documentation](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/shared/tls.md), [types](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/tls.go) |
| Inbound `tls.acme` | Inline ACME domains, challenge controls, EAB and DNS-01 options remain supported. DNS provider discriminator selects alidns/cloudflare/acmedns with provider-specific credentials. Challenge ports must be forwarded where required. No 1.14 certificate-provider tag is introduced. | [ACME types](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/tls_acme.go), [TLS/ACME documentation](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/shared/tls.md#acme-fields) |
| V2Ray transport / multiplex / UDP-over-TCP | Transport selects http/ws/quic/grpc/httpupgrade, with protocol-specific fields. Multiplex protocol is smux/yamux/h2mux. UDP-over-TCP accepts boolean or its option object; exact protocol support and credential combinations remain core checks. | [transport](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/v2ray_transport.go), [multiplex](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/multiplex.go) |
| WireGuard / Tailscale endpoints | WireGuard retains addresses, private key, peers, allowed IPs and reserved bytes; bytes accept base64 or integer arrays. Tailscale retains its authentication/control/state options and hostname/exit-node settings. Keys, network prefixes and references need core validation. | [WireGuard](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/wireguard.go), [Tailscale](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/tailscale.go) |
| `certificate`, `ntp`, `services`, `experimental` | Certificate stores/files/directories, NTP server/dial fields, registered service unions, Clash/V2Ray API and cache-file sections are covered. Service tags and TLS/listen/endpoint references are validated by core. HTTP API exposure depends on the actual configured address and authentication. | [certificate](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/certificate.go), [NTP](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/ntp.go), [services](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/service.go), [experimental](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/experimental.go) |

The source still declares some removed fields solely to reject them at decoding.
The visual contract excludes legacy inbound sniff/domain options, proxy_protocol
listen fields, old split TUN address fields, gso, endpoint_independent_nat,
legacy outbound destination overrides, GeoIP/Geosite and the misspelled
`rule_set_ipcidr_match_source`. Advanced JSON preserves existing contents; checking
reports unsupported fields instead of silently deleting them.

The 1.13 certificate custom decoder silently ignores some unknown properties.
The reviewed Schema deliberately rejects the 1.14 `certificate.providers` field:
passing an ignored setting through `sing-box check` would not implement providers.
Other unsupported new protocol discriminators are rejected by both Schema and
core. TLS settings are evaluated only when TLS is enabled.

### Reviewed musl registries

These discriminators follow the exact `include/` registries and official feature
reports, not the set of structs alone. Removed compatibility stubs such as
ShadowsocksR, DNS outbound and WireGuard outbound are excluded.

| Collection | Supported types |
| --- | --- |
| Inbounds | tun, redirect, tproxy, direct, socks, http, mixed, shadowsocks, vmess, trojan, naive, shadowtls, vless, anytls, hysteria, tuic, hysteria2 |
| Outbounds | direct, block, selector, urltest, socks, http, shadowsocks, vmess, trojan, naive, tor, ssh, shadowtls, vless, anytls, hysteria, tuic, hysteria2 |
| DNS servers | legacy, udp, tcp, tls, quic, https, h3, hosts, local, fakeip, dhcp, tailscale, resolved |
| Endpoints | wireguard, tailscale |
| Services | resolved, ssm-api, derp, ccm, ocm, oom-killer |

For both architectures 1.13.19, 1.13.20 and 1.13.21 report the same feature set:
`badlinkname`, `tfogo_checklinkname0`, `with_acme`, `with_ccm`, `with_clash_api`,
`with_dhcp`, `with_gvisor`, `with_musl`, `with_naive_outbound`, `with_ocm`,
`with_quic`, `with_tailscale`, `with_utls`, `with_wireguard`. They report Go 1.25.12
and CGO enabled. Feature presence does not make a protocol work without its
required credentials, network or operating-system permissions.

### Acceptance fixtures

The shared [configuration fixtures](../internal/sing-box-panel/schema/testdata/configuration-1.13)
cover every top-level section through representative DNS actions/legacy servers,
TLS/inline ACME and transports, WireGuard and resolved service configurations.
Go tests validate them against the reviewed schema, together with negative cases
for new protocols, field types, port bounds and conflicting action options. TLS-disabled
fixtures exercise option decoding without issuing certificates; this is not a live
ACME issuance test. Web tests edit and round-trip them in the visual editor.
