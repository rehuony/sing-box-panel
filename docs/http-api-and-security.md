# HTTP API and security

The HTTP server delivers the embedded Web application, the authenticated
management API, and token-authenticated subscription output. Its default
listener is loopback-only.

## API contract and routing

The management API is rooted at `/api/v1`. The authoritative operation,
schema, status-code, and problem-detail contract is
[`api/openapi.yaml`](../api/openapi.yaml); this guide does not duplicate its
endpoint inventory.

`server.base_path` prefixes the Web application, management API, and `/sub`
routes. It must be empty or a normalized path without a trailing slash. Browser
code uses same-origin paths and does not depend on backend implementation
packages.

`GET /api/v1/system/status` resolves capability state from the OS-verified live
exact version. When the child is stopped, it may use the applied bundle's exact
identity. With neither source, the state is `unresolved`; it never assumes the
newest catalog release. A quarantined exact pin is reported as
`quarantined_manual_json`.

## Management authentication

The settings file contains one management token. API clients may send it as a
Bearer credential. Browser login exchanges it for an HttpOnly, SameSite
session cookie and a CSRF token.

Cookie-authenticated state changes require both the session CSRF token and a
same-origin request. Login failures are rate-limited by the direct peer
address; forwarded-IP headers are not trusted. CORS is disabled by default.

The generated listener is `127.0.0.1:3000`. Before exposing the service beyond
loopback, place it behind a reviewed HTTPS reverse proxy, set
`server.external_origin` to its normalized public origin, and set
`auth.secure_cookie` so the browser session cookie is HTTPS-only. CSRF origin
checks use this explicit value and never trust `Forwarded` or
`X-Forwarded-*` headers. Do not treat the management token as a public
subscription token.

## Request and download protections

The server applies bounded request bodies, strict JSON decoding where the
contract requires it, constant-time token comparison, security response
headers, and secret-redacting event metadata.

Network downloads use explicit host and resolved-address checks, bounded
responses, timeouts, and restricted redirects. Archive import rejects path
traversal, symbolic links, non-regular entries, duplicate binaries, excessive
expansion, and non-canonical gzip/tar input. These controls reduce common SSRF,
archive, and resource-exhaustion risks but do not replace the artifact trust
model described in [Core versions and capabilities](core-versions-and-capabilities.md#artifact-trust-boundary).

Keep the following data private:

- settings files and management tokens;
- exported canonical or manual configuration;
- subscription token plaintext and source snapshots; and
- diagnostic files that may contain paths or operator-provided values.

Use file or stdin inputs for secrets instead of command arguments.

## Revision and version boundaries

Canonical writes use revision preconditions. HTTP clients send the current
revision through `If-Match`; a stale value receives `412 Precondition Failed`
and must be reviewed rather than overwritten automatically.

Version-scoped operations that allow an omitted `core_version` resolve it from
the live verified process. Collection operations whose OpenAPI description
defines `core_version` as a filter simply omit filtering when it is absent.

## Web presentation boundary

An exact capability pin may expose semantic facts and inert descriptors only.
The React application maps those descriptors onto built-in group, text,
number, boolean, select, and JSON controls. It does not load manifest-provided
scripts, components, templates, or remote resources.

Compatible structured support requires explicit acceptance. Changing the
viewed exact version discards the previous presentation synchronously so
controls cannot cross version boundaries.

Canonical snapshots include authoritative raw `document_json`. The generic
editor and manifest controls use a lossless JSON codec, and the versioned form
sends only edited JSON Pointer values as JSON text in one bounded `If-Match`
patch. Editing another field therefore cannot rewrite a large integer, large
exponent, or decimal lexeme through a JavaScript `number`. A `412` response
preserves the local draft for review.

See the [Web application reference](../web/README.md) for frontend ownership
and build behavior.
