# Sing-Box Panel web shell

This directory owns the React/Vite frontend. It deliberately has no dependency
on backend implementation packages.

```sh
corepack enable pnpm
pnpm install --frozen-lockfile --ignore-scripts --verify-store-integrity
pnpm run lint
pnpm test
pnpm run build
```

Run `pnpm run lint:fix` to apply the configured TypeScript, React, CSS, and HTML
formatting rules before checking the remaining code-quality diagnostics.

Transport types under `src/api/generated/` are build artifacts generated from
`../api/openapi.yaml`. Development, typecheck, test, and build commands refresh
them automatically; they are not committed or edited by hand. The custom HTTP
client remains hand-written.

`pnpm run build` produces a deterministic SPA bundle in `web/dist/`. The Go
delivery adapter should embed that directory and use `index.html` as the
fallback for non-API routes. `dist/` is generated output and is not a source of
truth; the backend may copy it into a package-local embed directory during its
generation step if Go's package boundary requires that layout.

At runtime the browser client uses same-origin `/api/v1` endpoints for the
session, dashboard context, the global canonical configuration and revisions,
durable tasks, and exact core-artifact/adapter operations. Cookie-backed writes
retain the session CSRF token, and canonical writes include the current
revision in `If-Match`.

Tests inject an `ApiClient`, keeping pages independent from `fetch` while the
HTTP client has focused tests for base-path routing, CSRF, problem details, and
revision preconditions. The configuration UI is structured-only: it preserves
unshown global fields, previews exact adapter diagnostics, requires the current
ignored-field digest, and exposes compilation, Apply, and lifecycle operations
without a raw startup-JSON editor.
