# Core versions

Release discovery, installation, visual configuration and inbound conversion are
separate capabilities. A core without a reviewed Schema can still check and run
raw JSON. Capabilities are selected by exact `MAJOR.MINOR.PATCH`, never by the
nearest supported release.

- [Install and select](#install-and-select-a-version), [choose a build](#multiple-installations-of-one-version), or [import](#import-a-local-archive)
- [Catalog cache](#catalog-cache), [verification boundaries](#checks-and-recorded-hashes), and [runtime switching](#switching-and-runtime-state)
- [Exact configuration support](#exact-configuration-support) and [CLI / HTTP migration](#cli-and-http-migration)
- [Maintaining support](#maintaining-support) and [1.13 review evidence](#113-configuration-review)

## Install and select a version

```sh
sing-box-panel core refresh
sing-box-panel core catalog
sing-box-panel core install 1.13.21
sing-box-panel core list
sing-box-panel core show 1.13.21
sing-box-panel core enable 1.13.21
# Remove an unused installation when it is no longer needed:
sing-box-panel core remove 1.13.21
```

`catalog` lists cached official downloads; `list` lists local installations.
Copy a version from the `VERSION` column. `install`, `show`, `enable` and `remove`
accept that complete version, or its release tag such as `v1.13.21`. Values such
as `1.13`, `1.13.*`, `latest`, prereleases and internal IDs are not accepted.
The CLI normalizes `v1.13.21` to `1.13.21` without selecting another release.

The default architecture is the CLI machine's architecture and the build variant
is `musl`. Use `--arch amd64` or `--arch arm64` to explicitly select an
architecture, including when querying a catalog or installation list:

```sh
sing-box-panel core catalog --arch arm64 --core-version 1.13.21
sing-box-panel core install v1.13.21 --arch arm64
sing-box-panel core list --arch arm64 --core-version 1.13.21
```

Installation inspects the actual executable; the environment must be able to
execute the selected architecture. Runtime enablement requires the panel server's
Linux architecture. An architecture flag does not provide emulation.

Installation uses the stored catalog. It does not refresh implicitly. If no cache
exists, run `sing-box-panel core refresh` and `sing-box-panel core catalog`.
If a version is absent from an existing cache, run
`sing-box-panel core refresh --force` and query the catalog again. Error messages
include these commands. They do not substitute a newer or older version.

The installed list has `VERSION`, `ARCH`, `SOURCE`, `STATUS` and `BUILD` columns.
Status distinguishes an installed artifact from the selected artifact and its
observed runtime state. `show` prints labelled details. `-o json` and `-o jsonl`
retain full internal IDs for machine consumers.

## Multiple installations of one version

Official and imported builds of one exact version may coexist. If more than one
installation matches the version, architecture and musl variant, `show`, `enable`
and `remove` require `--build`. Copy the short installation number from the
`BUILD` column of `core list`. The ambiguity error also lists candidates and a
complete command for each one. For example, if the list shows `a17b34c56d78`:

```sh
sing-box-panel core show 1.13.21 --arch arm64 --build a17b34c56d78
sing-box-panel core enable 1.13.21 --arch arm64 --build a17b34c56d78
sing-box-panel core remove 1.13.21 --arch arm64 --build a17b34c56d78
```

This example number must be replaced with the value from your list. Short numbers
are prefixes derived from existing installation IDs, normally 12 characters.
Collisions extend the displayed prefix; an ambiguous prefix never selects a
candidate automatically. No database migration or new persistent identifier is
needed. The default list limit is 50; use version/source filters or `--limit 200`
when it reports more installations.

## Import a local archive

```sh
sing-box-panel core import \
  --file ./sing-box-1.13.21-linux-arm64-musl.tar.gz \
  --version 1.13.21 \
  --arch arm64
```

`--file` is the local archive path and `--version` is the version that the binary
must report. `--arch` defaults to the local machine; `--variant` only accepts
`musl`. An optional `--source` describes the archive's origin. There is no
`--sha256` argument.

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
cache expiration. Ordinary refreshes honor the interval; `--force` bypasses it.
The running server checks in the background without delaying HTTP readiness.

Opening Web version management reads the cache and initializes it only if missing.
“Check GitHub for updates” forces a refresh. Cached downloads and installed versions
remain visible during refresh. The server-side client honors proxy environment
variables and the optional GitHub token in panel settings; credentials do not
bypass rate limits. See [database compatibility](../getting-started.md#database-compatibility).

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
core file contents with those recorded values. Existing IDs, `sha256/...` paths
and database records continue to work. Hash folder names describe storage layout,
not a runtime authenticity guarantee. Reused executables still undergo platform
and exact-version inspection at installation.

Three other verification boundaries remain intact:

- Configuration snapshot digests bind the checked JSON to the bytes activated.
- Schema asset digests bind the exact reviewed backend and frontend contracts.
- Panel release signatures and updater checks authenticate the panel distribution.

Maintainers also keep size/SHA-256 pins for upstream artifacts used in Schema
regeneration and native compatibility CI. Those pins make review reproducible;
they are not prerequisites for an operator installing or running a core.
Official downloads rely on GitHub HTTPS and the fixed repository/release location.
Imported cores rely on the administrator's choice. Recorded hashes do not provide
publisher authenticity or ongoing tamper detection.

## Switching and runtime state

Web Enable and CLI `core enable VERSION` preserve the stopped/running state.
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

## Exact configuration support

| Exact releases | Configuration Schema source | Inbound family |
| --- | --- | --- |
| 1.13.19, 1.13.20, 1.13.21 | Project-reviewed `reviewed-1.13` source definition | 1.13 |
| 1.14.0, 1.14.1 | Official binary's native `schema` command | 1.14 |

Each exact release has a separate Schema asset, manifest entry and precompiled
browser validator. The three 1.13 option type definitions are identical and share
one reviewed semantic source. Field provenance, actual musl protocol coverage,
conditional checks and DNS patch behavior are documented in the
[1.13 review and field table](#113-configuration-review). Presentation annotations live
separately from semantic constraints. The 1.13 forms exclude 1.14-only fields.

The panel supplies the five upstream legacy DNS compatibility switches only for
these three reviewed 1.13 versions, during both check and run. This preserves old
DNS servers, FakeIP options, outbound DNS rules, domain strategy and missing
resolver behavior without changing JSON. Their deprecation warnings remain
visible. Before upgrading to 1.14, explicitly migrate those forms using the
[upstream migration guidance](https://sing-box.sagernet.org/migration/).

A version without an exact manifest entry uses Advanced JSON editing. Missing
Schema or inbound-conversion support does not prohibit raw runtime use. No nearby
version's Schema or converter is substituted. The existing
[1.14.1 review](https://github.com/SagerNet/sing-box/compare/v1.14.0...v1.14.1)
retains family 1.14 and its independently generated asset.

## CLI and HTTP migration

Replace `core install ASSET_ID` with `core install VERSION`. Replace
`core show/enable/remove ARTIFACT_ID` with the corresponding version and, when
needed, `--build` from `core list`. Remove `core catalog --installable` and local
import `--sha256` from scripts. Regenerate shell completions after upgrading;
version and build completion now come from the cached catalog and local records.

HTTP management endpoints continue to locate objects by internal asset or artifact
ID. Machine CLI output preserves those full IDs. The multipart import endpoint no
longer accepts a `sha256` field and the catalog endpoint no longer accepts an
`installable` query. Update API clients to the [OpenAPI contract](../../api/openapi.yaml).

## Maintaining support

`internal/singbox/catalog.json` locks exact tags, commits, module sums, dual
architecture release profiles, feature fingerprints and capability sources.
Add a version only after comparing its exact source, tag documentation, migration
notes and both Linux musl packages. Reuse an inbound family only after reviewing
its input and published client fields. Native Schema versions use the official
`schema` command; pre-1.14 support requires an explicit reviewed source definition,
never a copy of a neighboring native Schema.

`make support-generate` runs on Linux and regenerates source-derived catalog code,
each exact Schema and the reviewed field table. `make support-check` validates committed outputs
offline. `make web-build` exports them and compiles browser validators. Production
never fetches Schema definitions from the network. Native Linux amd64 and arm64
`make core-contract` runs the real binaries, including 1.13 positive and negative
configuration fixtures. `make release-smoke` exercises the panel release flow on
native Linux as a non-root user. Emulation is supplementary evidence.

The daily Core Version Monitor opens or updates one rolling issue when the latest
stable upstream release exceeds the highest supported catalog entry. It requests
manual review, never edits support code or backfills older release lines.

## 1.13 configuration review

This review covers only **1.13.19, 1.13.20 and 1.13.21** and their official Linux
amd64/arm64 musl packages. It does not assign capabilities to other 1.13 releases.

### Sources and patch comparison

| Release | Pinned commit | Configuration documentation | Source types and migration notes |
| --- | --- | --- | --- |
| 1.13.19 | `b5ebaa1fc0f2b94256180b95468e73ef53caa27d` | [tag documentation](https://github.com/SagerNet/sing-box/tree/v1.13.19/docs/configuration) | [option](https://github.com/SagerNet/sing-box/tree/v1.13.19/option), [migration](https://github.com/SagerNet/sing-box/blob/v1.13.19/docs/migration.md) |
| 1.13.20 | `56f91dfeabd6f4edbd437dfcc1e5b0ebc856b778` | [tag documentation](https://github.com/SagerNet/sing-box/tree/v1.13.20/docs/configuration) | [option](https://github.com/SagerNet/sing-box/tree/v1.13.20/option), [migration](https://github.com/SagerNet/sing-box/blob/v1.13.20/docs/migration.md) |
| 1.13.21 | `628cb31ffa79cffffd34c2f9cde6cae044e4fc12` | [tag documentation](https://github.com/SagerNet/sing-box/tree/v1.13.21/docs/configuration) | [option](https://github.com/SagerNet/sing-box/tree/v1.13.21/option), [migration](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/migration.md) |

All 50 `option/*.go` files are byte-identical across these three tags. The
[17-commit comparison](https://github.com/SagerNet/sing-box/compare/v1.13.19...v1.13.21)
adds or removes no configuration fields. The registries under `include/` are also
unchanged. This supports one shared semantic source definition, with independent
assets and manifest entries for each version. The current website is auxiliary:
its newer certificate providers, DNS response actions, protocols and endpoint
fields must not be copied into this definition.

The 1.13.20 patch fixes inverted DNS rules whose address conditions are deferred
until after lookup. It also corrects the DNS rule documentation: destination
address conditions (`ip_cidr`, `ip_is_private`, `ip_accept_any`) belong to the same
OR group as domain conditions; other condition groups combine with AND. The
[matching implementation and regression tests](https://github.com/SagerNet/sing-box/blob/v1.13.20/route/rule/rule_dns_address_filter_test.go)
change runtime semantics, not field shapes. 1.13.19 retains its original behavior;
selecting another patch version never rewrites a rule to emulate that fix.
The [1.13.21 changelog](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/changelog.md)
also records WebSocket/TUN fixes. Their behavior remains the exact core's responsibility.

### Field table and maintenance

The [complete field table](../../cmd/singbox-support/schema-sources/1.13-fields.csv) is generated from
[`schema-sources/1.13.json`](../../cmd/singbox-support/schema-sources/1.13.json).
It records configuration paths, JSON shapes and enum values, branch optionality,
constraints and exact source line links. Shared `$defs` expand once; later rows
refer to that definition. `[]` means an array element, `.*` a map value, and
`{oneOf:N}` / `{allOf:N}` identify union or composed branches. Follow the branch's
`type`, `action` or `provider` constant to identify a protocol. Source links use
1.13.21 because the reviewed option files are identical in all three tags.

Optional in this table means that the JSON decoder allows omission. It does not
promise that a runnable instance can omit credentials, a server, a referenced tag,
a certificate or a route action's effective options. Those contextual requirements
are summarized below and enforced by the real core. In particular, configuration
checking does not prove that an external ACME server, DNS resolver or tunnel peer
is reachable at runtime.

The reviewed 1.13 Schema also accepts explicit `null` for optional top-level
sections and plain log fields, matching the core's Go decoder. An empty or `null`
`log.level` uses the core's default level. These values remain unchanged
through validation and unrelated visual edits; custom nested types keep their
own decoding constraints. Shared fixtures exercise these cases in the backend,
browser validator and exact official core checks.

The source definition contains semantic JSON constraints and provenance. The
existing `schema_overlay.go` workflow separately adds `x-panel` presentation
annotations; it cannot widen accepted fields or values. `make support-generate`
regenerates the CSV and independent exact-version Schema assets. Offline
`make support-check` rejects source/output drift. `make web-build` produces each
version's standalone browser validator. These are reviewed schemas, not official
native-schema output: sing-box 1.13 has no `schema` command.

### Section and conditional-field review

| Section / fields | 1.13 shape and associated constraints | Exact reference |
| --- | --- | --- |
| `log` | Optional object; level is trace/debug/info/warn/error/fatal/panic. Output is a path; disabled/timestamp are booleans. | [log](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/log/index.md) |
| `dns.servers[]` | Typed transports and the legacy address format coexist. Missing/empty/legacy `type` selects the legacy decoder. Typed remote transports use server/server_port and their dial/TLS options; a domain-valued server requires an appropriate resolver relationship. | [DNS source](https://github.com/SagerNet/sing-box/blob/v1.13.21/option/dns.go), [legacy servers](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/dns/server/legacy.md) |
| `dns.rules[]` | Default or recursive logical rules; logical mode is and/or. Actions are route, route-options, reject, predefined. Route is the omitted action. Server/rule-set/outbound references must resolve. Predefined answers use textual or base64 DNS records; query type accepts numeric RR types or recognized uppercase names. | [DNS rules](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/dns/rule.md), [actions](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/configuration/dns/rule_action.md) |
| `dns.fakeip`, `independent_cache`, legacy `outbound` rule items | Retained 1.13 forms. FakeIP ranges must be valid IP prefixes. The five deprecated DNS switches are enabled by the panel only for these three exact versions; warnings remain visible. | [deprecated features](https://github.com/SagerNet/sing-box/blob/v1.13.21/experimental/deprecated/constants.go), [migration](https://github.com/SagerNet/sing-box/blob/v1.13.21/docs/migration.md) |
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

For both architectures all three releases report the same feature set:
`badlinkname`, `tfogo_checklinkname0`, `with_acme`, `with_ccm`, `with_clash_api`,
`with_dhcp`, `with_gvisor`, `with_musl`, `with_naive_outbound`, `with_ocm`,
`with_quic`, `with_tailscale`, `with_utls`, `with_wireguard`. They report Go 1.25.12,
CGO enabled and their respective pinned commits. Exact package names, URLs,
sizes and maintenance hashes are in the [support catalog](../../internal/singbox/catalog.json).
Feature presence does not make a protocol work without its required credentials,
network or operating-system permissions.

### Executable acceptance evidence

The shared [configuration fixtures](../../internal/singbox/testdata/configuration-1.13)
cover every top-level section through representative DNS actions/legacy servers,
TLS/inline ACME and transports, WireGuard and resolved service configurations.
They are checked against each exact Schema and each actual Linux binary.
TLS-disabled fixtures exercise option decoding without issuing certificates;
this is not a live ACME issuance test. Negative cases cover new protocols,
field types, port bounds and conflicting action options. Native-core checks are
supplemented by browser form edits/round trips and per-version inbound conversion
with server credential stripping. Unknown exact versions remain JSON-only.
