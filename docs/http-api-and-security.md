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

Configuration operations address one saved JSON document. Compilation binds an
immutable installed core artifact, not a naked version string. The server
derives and rechecks the artifact's verified identity, then asks that exact
binary to validate the immutable configuration bytes. JSON Schema and inbound
conversion availability are independent optional capabilities.

`GET /api/v1/system/status` reports the configuration revision and binary
evidence associated with the live artifact or, when stopped, the applied
bundle. It never selects the newest catalog version or a nearby release.

## Management authentication

The shared settings file supplies the management token. Web replacements update
`auth.token` in that file; CLI or manual token edits are seen at the next
authentication boundary and invalidate existing sessions. API clients may send the
current token as a Bearer credential. Browser login exchanges it for an HttpOnly,
SameSite session cookie and a CSRF token.

Replacement tokens must contain 32–8192 UTF-8 bytes, without leading or trailing
Unicode whitespace or BOM, NUL, CR, or LF. Invalid replacements leave the current
credential and sessions intact. The login JSON body is bounded to 64 KiB so every
accepted token fits even when JSON encoding escapes its characters.

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
input. See [Core versions](core-versions.md#artifact-trust-boundary)
for the artifact trust boundary.

Third-party subscription refresh validates DNS and every redirect destination
against the configured source-network policy. Public subscription requests use
only persisted successful source versions and never fetch an upstream URL.

Keep the following data private:

- settings files and management tokens;
- exported sing-box configuration;
- subscription token plaintext and raw source versions; and
- diagnostic files that may contain paths or operator-provided values.

Use file or stdin inputs for secrets instead of command arguments.

## Concurrency and immutable evidence

Configuration writes use revision preconditions. The editable file API accepts
`{ revision, content }` in the `PUT /api/v1/config/file` JSON body. A stale
revision receives `412 Precondition Failed`
and must be reviewed rather than overwritten automatically. Channel, source,
user-grant, and token mutations use their documented compare-and-swap or
lifecycle preconditions.

Compilation stores the raw JSON bytes together with the configuration revision,
immutable core artifact, exact version, and both content digests. Apply and
lifecycle operations revalidate that evidence; a candidate that fails the
selected binary's `sing-box check` never becomes ready.

Public subscription responses are rendered from one consistency read of the
applied local startup artifact, current enabled source versions, channel and key.
User-bound keys additionally require an enabled user and exact grants;
independent keys use channel publication policies. Response bodies are not frozen
into activation bundles.

Management mutations return HTTP 200 after completion, with the resulting core
artifact, catalog summary, refreshed source version, checked startup artifact or
runtime status. The browser keeps controls pending until that response and verifies
process identity for runtime changes. Errors use problem details and leave the
previous usable state intact where the operation has not committed.

Integrations consume the completed resource response directly. Runtime snapshots
are internal evidence; there is no separate configuration-history write or restore API.

## Web presentation boundary

HTML responses generate a unique style nonce, place it in the page metadata and
include it in `style-src`. CodeMirror uses that nonce for its generated styles.
HTML is served with `Cache-Control: no-store`; script policy remains self-only,
without unsafe-inline or unsafe-eval. Ordinary notifications dismiss after three
seconds (hover/focus pauses the accessible toast timer).

The React application contains its own trusted structured controls. It does
not load Schema-provided scripts, components, templates, or remote resources.
Changing the selected artifact clears the previous Schema state before
resolving the new exact version.

The editable configuration response includes authoritative `content` text,
`revision` and `syntax_valid`. The Web editor uses a lossless codec, preserves
unshown fields, and saves the complete text with the current body revision.
The version selector can use a bundled reviewed schema for authoring before core installation. Exact versions without a native schema use the Advanced editor and show the reason; selecting another version never changes the installed core.
A `412` response preserves the local draft for review.

See the [Web application reference](../web/README.md) for frontend ownership
and build behavior.
