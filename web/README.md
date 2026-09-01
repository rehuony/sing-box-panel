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

Run `pnpm demo` to start the complete interface with an in-memory API client.
Demo mode never contacts the backend, so it is suitable for working on pages,
responsive states, themes, and interaction styles without a running panel. Its
state lives only in the browser process and resets when the development server
or page is reloaded; use `pnpm dev` when backend integration is required.

Transport types under `src/api/generated/` are build artifacts generated from
`../api/openapi.yaml`. Development, typecheck, test, and build commands refresh
them automatically; they are not committed or edited by hand. The custom HTTP
client remains hand-written. `pnpm run api:generate` performs only this OpenAPI
generation; configuration Schema export belongs to the Vite plugin.

Configuration contracts under `src/schemas/generated/` are exported offline
from the committed backend Schema assets by the Vite plugin. Its manifest binds
each exact sing-box version to one file and SHA-256 digest, and the plugin
precompiles the root validator as an Ajv 2020 module for the browser. Generated
validators contain no CommonJS `require`. A version without a native Schema
remains available in the Advanced JSON editor; structured editing is enabled
only when the exact-version local Schema and served digest match.

`pnpm run build` produces a deterministic SPA bundle in `web/dist/`. The Go
Web package embeds that directory and uses `index.html` for client-side
routes. `dist/` is generated output and is not a source of truth, but it must
exist before any Go package that embeds the Web application is loaded.
`public/favicon.svg` is the single static icon source used by HTML, the React
logo, release bundles, and the repository README.

At runtime the browser client uses same-origin `/api/v1` endpoints for the
session, dashboard context, the global JSON configuration and revisions,
durable tasks, and exact core-artifact operations. Cookie-backed writes retain
the session CSRF token, and configuration writes include the current
revision in `If-Match`.

Except for Start, Stop, and Restart—which the shared telemetry banner follows
to a terminal runtime result—an asynchronous action reports only that its
durable task was accepted, shows the task ID, and links to the Tasks page.
Feature pages do not duplicate task polling or infer completion from the
initial queued response.

Tests inject an `ApiClient`, keeping pages independent from `fetch` while the
HTTP client has focused tests for base-path routing, CSRF, problem details, and
revision preconditions. The first visit selects Simplified Chinese for a
`zh-CN` browser preference and English otherwise; an explicit user selection
is persisted and takes precedence on later visits.

The configuration UI combines version-scoped RJSF controls with a lossless
sing-box JSON editor. It preserves unknown fields and unmodified large-number
lexemes, and never adds panel metadata to executable configuration. Versions
before native Schema support use the Advanced editor only. Compile, binary
check, Apply, evidence-bound rollback, and startup artifact history all stay on
the Deploy surface; the selected exact binary remains the final authority.
