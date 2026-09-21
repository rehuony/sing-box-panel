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
only when the exact-version local Schema and served digest match. With no matching installed artifact, the configuration version selector supports authoring from the bundled reviewed schema; validation and execution still require the corresponding installed core.

`pnpm run build` produces a deterministic SPA bundle in `web/dist/`. The Go
Web package embeds that directory and uses `index.html` for client-side
routes. `dist/` is generated output and is not a source of truth, but it must
exist before any Go package that embeds the Web application is loaded.
`public/favicon.svg` is the single static icon source used by HTML, the React
logo, release bundles, and the repository README.

At runtime the browser client uses same-origin `/api/v1` endpoints for the
session, live dashboard context, the single saved configuration, panel settings,
subscription publication, persistent logs and exact core-artifact operations. Cookie-backed writes retain
the session CSRF token, and saved-file writes include the numeric revision in the request.

Operations await the completed API resource; version/source actions update in
place and failures retain previous usable state. Runtime controls verify observed
process identity before reporting success. Version management displays the cached
catalog immediately and automatically refreshes it using the configured TTL.
Panel logs display ordinary events with no operation tracking dialogs. Core logs
and telemetry use authenticated streams with bounded buffering and reconnect.

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
replacing the process. Immutable snapshots support runtime verification and recovery;
the editor has one current document with no historical selection or restoration.

Configuration modules are edited individually, with optional object settings
added on demand. Collection actions stay in a consistent, vertically centered
toolbar beside the section tabs where present, without item counts. List cells
are centered and entry names are display-only. Adding a record or choosing its
Edit action opens a dialog; confirming updates the draft and cancelling discards
the pending changes. Map fields keep keys separate from typed text, list or object
values, and referenced scalar lists are edited inline. Dialog content remains
mounted through the synchronized closing transition. Navigation to another route,
query or hash tab prompts before discarding unsaved edits, including incomplete
JSON. Keep editing preserves the current URL, draft and concurrency revision;
Discard changes resets the draft and continues to the requested destination.
Subscription areas return to their initial lists after leaving. Reloading or
closing the page uses the browser's native unsaved-changes prompt, and signing
out requires confirmation while edits are pending. No draft or embedded
configuration secrets are written to browser storage. An uninitialized file opens an empty editor and creates its first
file version. The file API uses its own numeric compare-and-swap revision;
immutable canonical revisions remain internal runtime evidence.

The interface retains its violet identity with floating frosted navigation and
runtime controls. Larger material surfaces use blur and translucent fills;
content stays on readable surfaces. Navigation selection uses a non-bouncing
spring, and controls respond on press. Reduced motion, reduced transparency,
and increased contrast preferences are respected.

The six main pages omit the visible page-title band while retaining a screen
reader heading. Their content fills the space below the runtime toolbar and
ends at the same bottom inset as the sidebar. Tables, forms, and the dashboard
scroll within that space when needed, keeping panel actions reachable.

Workspace tables share a 16px toolbar-to-header gap, 44px headers, and a 65px
minimum row height including the divider. Headers have no bottom divider;
dividers appear only below data rows. Rows expand when wrapped content
needs more space. These dimensions cover versions, sources, keys, channels,
and panel logs.

The six navigation entries host 15 review views using local tabs/dialogs instead
of feedback-page copies. Sources, manual nodes, subscription keys and native
channel rules share node provenance and independent visibility. Channel preview
and delivery have one server renderer; remote rule references are fetched only
by subscribing clients. Native editors preserve unknown fields and scalar/list
representations. Appearance is a saveable preview transaction shared by controls,
charts and overlays, with semantic status colors kept independent.

## Error feedback

Request failures and validation errors use the shared Toaster instead of in-flow
banners or field-error labels. Use `toast.add` for an action failure and
`ErrorNotice` for an error represented by component state. Repeated state errors
reuse one notification; recovery or unmount closes it. Keep invalid controls
marked with `aria-invalid`, and retain visually hidden descriptions where
`aria-describedby` needs them. Error feedback must not take focus away from the
current control or shift the surrounding layout. Service-unavailable screens
keep their retry action available after the notification is dismissed.

Management tabs use URL fragments: `#cores-installed` / `#cores-catalog`,
`#configuration-visual` / `#configuration-advanced` (visual sections append `/route`,
`/inbounds`, etc.), `#panel-security` / `#panel-nodes` / `#panel-appearance`, and
`#logs-core` / `#logs-panel`. Direct links, reloads and browser history restore the
selected tab; legacy log query links still open their requested view. Tab switches
preserve unrelated query parameters and route state. Subscription tabs retain
`#subscription-sources`, `#subscription-tokens`, and `#subscription-channels`.
