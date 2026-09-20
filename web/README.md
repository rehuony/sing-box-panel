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

`src/api/api-client.ts` exposes the client interface and simple generated
transport types directly. Modules in `src/api/contracts/` own additional
domain-specific query filters and derived types. Routes are assembled in
`src/routes/app.routes.tsx`; complex component entries remain narrow boundaries
around their companion files. Each locale groups navigation, account, language,
and theme labels in `shell.ts`, with shared and startup messages in `common.ts`.

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
session, live dashboard context, the single saved configuration, panel settings,
subscription publication, durable logs/tasks and exact core-artifact operations. Cookie-backed writes retain
the session CSRF token, and saved-file writes include the numeric revision in the request; legacy
canonical endpoints retain `If-Match`.

Asynchronous operations use shared task tracking and report completion only after
a terminal API result. Version/source actions update in place; failures retain
previous usable state. The panel-log detail view follows pending tasks, stops
polling at a terminal state and aborts tracking when closed. Task detail links use
`/observability?tab=panel&task=<id>`. Core logs and telemetry use authenticated streams
with bounded buffering, reconnect and polling recovery.

Tests inject an `ApiClient`, keeping pages independent from `fetch` while the
HTTP client has focused tests for base-path routing, CSRF, problem details, and
revision preconditions. The first visit selects Simplified Chinese for a
`zh-CN` browser preference and English otherwise; an explicit user selection
is persisted and takes precedence on later visits.

The configuration UI combines version-scoped RJSF controls with a lossless
sing-box JSON editor. It preserves unknown fields and unmodified large-number
lexemes, and never adds panel metadata to executable configuration. Versions
before native Schema support use the Advanced editor only. The Web UI offers Save and Validate, with validation feedback in a Toast.
Start/Restart validate saved bytes using the selected exact binary before
replacing the process; immutable history and rollback remain internal/legacy
API and CLI contracts rather than a second deployment UI.

Configuration modules are edited individually, with optional object settings
added on demand. Collection actions stay in a consistent, vertically centered
toolbar beside the section tabs where present, without item counts. List cells
are centered and entry names are display-only. Adding a record or choosing its
Edit action opens a dialog; confirming updates the draft and cancelling discards
the pending changes. Map fields keep keys separate from typed text, list or object
values, and referenced scalar lists are edited inline. Dialog content remains
mounted through the synchronized closing transition. Drafts, including incomplete JSON, survive route changes in
memory for the current authenticated session. Reloading or closing the page
prompts when there are unsaved changes; signing out clears the draft. No draft
or embedded configuration secrets are written to browser storage. The saved file revision remains the concurrency base after navigating away
and returning. An uninitialized file opens an empty editor and creates its first
file version. The file API uses its own numeric compare-and-swap revision;
immutable canonical revisions remain internal runtime evidence.

The interface retains its violet identity with floating frosted navigation and
runtime controls. Larger material surfaces use blur and translucent fills;
content stays on readable surfaces. Navigation selection uses a non-bouncing
spring, and controls respond on press. Reduced motion, reduced transparency,
and increased contrast preferences are respected.


The six navigation entries host 15 review views using local tabs/dialogs instead
of feedback-page copies. Sources, manual nodes, subscription keys and native
channel rules share node provenance and independent visibility. Channel preview
and delivery have one server renderer; remote rule references are fetched only
by subscribing clients. Native editors preserve unknown fields and scalar/list
representations. Appearance is a saveable preview transaction shared by controls,
charts and overlays, with semantic status colors kept independent.
