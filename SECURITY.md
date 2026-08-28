# Security Policy

## Supported Versions

Security fixes are provided for the latest published stable release only.

| Version | Supported |
| --- | --- |
| Latest stable release | Yes |
| Earlier releases | No |
| Unreleased `main` branch | Reports are welcome, but no support guarantee applies |

Until the first stable release is published, there is no supported release.
Reports against the development branch are still welcome so vulnerabilities
can be addressed before publication.

## Reporting a Vulnerability

Report vulnerabilities through [GitHub private vulnerability
reporting](https://github.com/rehuony/sing-box-panel/security/advisories/new).

Do not open a public issue, pull request, or discussion containing vulnerability
details. Include the affected version or commit, deployment and exposure
conditions, reproduction steps, expected impact, and any relevant sanitized
logs. Remove management tokens, subscription tokens, configuration secrets,
private paths, personal data, and signing material.

The maintainer will acknowledge a private report within seven calendar days and
begin validating its reachability and impact. Remediation and disclosure
timelines depend on severity and complexity and will be coordinated through the
private advisory. The project does not currently operate a paid bug bounty.

When testing, use systems and data you own or are authorized to access. Avoid
service disruption, persistence, destructive actions, privacy violations, and
access to unrelated data. Give the maintainer a reasonable opportunity to
investigate and release a fix before public disclosure.

## System and Scope

sing-box-panel is a local-first Linux management service and CLI with an
embedded Web application. It manages configuration history, sing-box artifacts,
runtime lifecycle, public subscription output, persistent state, systemd
integration, installation, and authenticated self-update.

Security review covers the Go backend and CLI, Web application, OpenAPI
boundary, persistence and runtime management, installer and systemd paths,
release build and signing workflow, and project-owned adapters and subscription
rendering.

Important attacker-controlled inputs include HTTP requests, browser uploads,
subscription source responses, GitHub release metadata and assets, imported
archives and binaries, configuration documents, public subscription tokens,
filesystem state, and subprocess output.

## Security Invariants

The following properties must hold:

- Management reads and mutations require the documented authentication
  boundary; cookie-authenticated mutations also require CSRF and origin checks.
- Management tokens, subscription plaintext tokens, configuration secrets,
  signing keys, and secret-bearing diagnostics must not be exposed through
  logs, API responses, generated artifacts, command arguments, or repository
  content.
- Network fetches, redirects, uploads, archives, JSON documents, command output,
  and decompressed content remain bounded and fail closed on malformed or
  ambiguous input.
- Imported core artifacts preserve exact version, architecture, feature,
  checksum, and immutable binary identity; unsupported profiles must not fall
  back to a nearby adapter.
- Installer and self-update paths verify the project Ed25519 signature and the
  selected binary checksum before replacing an executable.
- Filesystem writes and executable replacement reject unsafe symbolic links,
  path traversal, non-regular targets, and partial updates.
- Runtime and systemd operations must not cross the selected user or system
  privilege boundary or silently broaden service privileges.
- Concurrent configuration, artifact, and activation changes must not bypass
  revision, digest, or immutable-evidence checks.
- Public subscription access remains default-deny and cannot expose nodes,
  sources, or configuration outside the token user's current grants.

A reachable violation of one of these properties is normally reportable.

## Severity Context

High-impact findings include remote code execution, authentication or
authorization bypass, management or subscription secret disclosure, release
signature or checksum bypass, arbitrary file write or executable replacement,
sandbox or privilege-boundary escape, and persistent corruption that causes an
unreviewed configuration or binary to run.

Availability, resource-exhaustion, and information-disclosure reports should
describe realistic attacker access, deployment conditions, and impact.
Dependency-version or best-practice findings should demonstrate
project-relevant reachability rather than relying only on a scanner result.

## Out of Scope

The following are not project vulnerabilities by themselves:

- vulnerabilities in upstream sing-box that do not arise from or become more
  exploitable through sing-box-panel;
- unsupported historical panel releases without evidence that the latest
  stable release is affected;
- social engineering, phishing, physical access, or compromised administrator
  credentials without a bypass of a project control;
- deployment choices that explicitly ignore documented loopback, HTTPS,
  cookie, filesystem-permission, or systemd-hardening requirements;
- reports that only identify an outdated dependency without a reachable impact
  in this project; and
- denial-of-service testing against systems without explicit authorization.

These exclusions do not suppress reports showing that a documented control is
missing, misleading, or bypassable.

## Known Limitations

The default management listener is loopback-only. Exposing it beyond loopback
requires a reviewed HTTPS reverse proxy, an explicit external origin, secure
cookies, and appropriate host firewall policy.

The project authenticates its own release artifacts with an embedded Ed25519
trust root. It does not claim TUF-style metadata or project-owned signatures
for upstream sing-box artifacts. User-supplied core imports rely on an expected
digest obtained through an independently trusted channel.

Existing installations cannot trust a replacement release-signing key without
an explicit transition design. Signing-key rotation therefore requires a
reviewed migration rather than an in-place key replacement.

## Coordinated Disclosure

Validated vulnerabilities will be handled through a private repository
security advisory when appropriate. Credit will be offered to reporters who
want it, subject to coordinated disclosure and the accuracy of the published
advisory.
