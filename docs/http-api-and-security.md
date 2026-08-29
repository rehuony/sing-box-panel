# HTTP API and security

The HTTP server delivers the embedded Web application, the authenticated
management API, and token-authenticated subscription output. Its default
listener is loopback-only.

## API contract and routing

The management API is rooted at `/api/v1`. The authoritative operation,
schema, status-code, and problem-detail contract is
[`api/openapi.yaml`](../api/openapi.yaml); this guide describes its trust
boundaries without duplicating the endpoint inventory.

`server.base_path` prefixes the Web application, management API, and `/sub`
routes. It must be empty or a normalized path without a trailing slash. Browser
code uses same-origin paths and depends only on the HTTP contract.

Configuration operations address one global canonical history.
Preview and compilation bind an immutable installed core artifact, not a
naked version string. The server derives the artifact's verified version, OS,
architecture, variant, and feature fingerprint, then requires one exact
compiled adapter. Unsupported profiles remain inspectable but fail closed for
projection and runtime work.

`GET /api/v1/system/status` reports the adapter evidence associated with the
live artifact or, when stopped, the applied bundle. It never selects the newest
catalog version or a nearby adapter.

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
responses, timeouts, and restricted redirects. Browser core import accepts one
bounded multipart file, verifies its declared SHA-256, and stages it in a
mode-0700 private data-directory location rather than accepting a server-local
path. Archive verification rejects path traversal, symbolic links, non-regular
entries, duplicate binaries, excessive expansion, and non-canonical gzip/tar
input. See [Core versions and adapters](core-versions-and-adapters.md#artifact-trust-boundary)
for the artifact trust boundary.

Third-party subscription refresh validates DNS and every redirect destination
against the configured source-network policy. Public subscription requests use
only persisted successful source versions and never fetch an upstream URL.

Keep the following data private:

- settings files and management tokens;
- exported canonical configuration;
- subscription token plaintext and raw source versions; and
- diagnostic files that may contain paths or operator-provided values.

Use file or stdin inputs for secrets instead of command arguments.

## Concurrency and immutable evidence

Canonical writes use revision preconditions. HTTP clients send the current
revision through `If-Match`; a stale value receives `412 Precondition Failed`
and must be reviewed rather than overwritten automatically. Channel, source,
user-grant, and token mutations use their documented compare-and-swap or
lifecycle preconditions.

Compilation stores projected bytes together with the canonical revision,
immutable core artifact, exact adapter ID/revision, diagnostics, and ignored
diagnostic digest. Apply and lifecycle operations revalidate that evidence;
there is no editable startup-JSON bypass.

Public subscription responses are rendered from one consistency read of the
applied local startup artifact, current enabled source versions, channel,
token user, and that user's exact grants. Response bodies are not frozen into
activation bundles.

## Web presentation boundary

The React application contains its own trusted structured controls. It does
not load adapter-provided scripts, components, templates, or remote resources.
Changing the selected artifact clears the previous preview before requesting
the new exact adapter result.

The canonical response includes authoritative `document_json`. The Web editor
uses a lossless codec, preserves unshown global fields, and replaces the whole
document with the current `If-Match` revision. It offers no editable raw
startup JSON. An ignored-field acknowledgement is tied to the current stable
digest, and a `412` response preserves the local draft for review.

See the [Web application reference](../web/README.md) for frontend ownership
and build behavior.
