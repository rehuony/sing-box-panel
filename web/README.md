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

Navigation preloads route modules on sidebar hover, keyboard focus and pointer
press, sharing each pending download with navigation. A failed speculative import
does not interrupt the current page; the next unloaded route reloads the document
to recover Vite's shared CSS/module preload state. Configuration downloads the
reviewed Schema/validator modules in parallel with the authenticated Schema
contract; version, digest and full-schema checks
still gate structured editing. Subscription lists and their node catalog load
concurrently, and the version library reads runtime status only once on entry.
Runtime responses are published independently of artifact listings and discarded
when newer shared runtime evidence arrives while the request is pending.

The HTTP client owns a bounded, session-local memory cache for navigation reads:
system metadata (30 seconds), installed core lists (15 seconds), release catalogs
(60 seconds), subscription lists/node summaries (5 seconds), and artifact-bound
Schema contracts (5 minutes). Identical in-flight reads share a request; cancelling
one consumer does not cancel the others. Runtime status only shares in-flight
requests, while configuration files, credentials, settings and filesystem reads
are not retained by this cache. Writes invalidate reads before and after the
request, including failed writes; session changes and authorization failures also
clear it. Explicit installed-version and subscription-source/node refreshes also
invalidate it, including refreshes without remote subscription sources. Changes
made by another client become visible after the relevant short lifetime expires.

`src/api/api-client.ts` exposes the client interface and simple generated
transport types directly. Modules in `src/api/contracts/` own additional
domain-specific query filters and derived types. Routes are assembled in
`src/routes/app.routes.tsx`; complex component entries remain narrow boundaries
around their companion files. Each locale groups navigation, account, language,
and theme labels in `shell.ts`, with shared and startup messages in `common.ts`.

`components/server-path-input` owns the shared server filesystem selector.
Configuration and subscription-node forms use it through context-aware presentation
annotations in `configuration-path-fields`, including scalar/list representations
and nested dialogs. Filesystem reads go through the injected `ApiClient`; demo
mode uses a fixed in-memory directory tree. The chooser preserves hand-entered
values until confirmation and rechecks the selected path with the server.
See [server path selection](../docs/guides/configuration-and-runtime.md#selecting-server-paths)
for filesystem scope and path semantics.

Path pickers and log-file filters reuse the shared Input, InputGroup and Select
styles for control height, borders, spacing and focus feedback. Opening a Select
with the mouse keeps options visually neutral, including the selected item;
the checkmark still identifies the current value. Options highlight on hover or
keyboard-visible focus, without changing the value until an option is chosen.

Configuration contracts under `src/schemas/generated/` are exported offline
from the committed backend Schema assets by the Vite plugin. Its manifest binds
each exact sing-box version to its native/reviewed source kind, file and SHA-256 digest, and the plugin
precompiles the root validator as an Ajv 2020 module for the browser. Generated
validators contain no CommonJS `require`. A version without a committed Schema
remains available in the Advanced JSON editor; structured editing is enabled
only when the exact-version local Schema and served digest match. The
configuration version selector lists compatible installed core versions,
prefers the enabled version, and otherwise selects the highest installed exact
version. Without an installed core, Advanced JSON editing and saving remain
available while structured editing and validation direct the operator to
version management.

`pnpm run build` produces a deterministic SPA bundle in `web/dist/`. The Go
Web package embeds that directory and uses `index.html` for client-side
routes. `dist/` is generated output and is not a source of truth, but it must
exist before any Go package that embeds the Web application is loaded.
Content-hashed JavaScript and CSS assets are served with a one-year immutable
cache lifetime. HTML and SPA fallback responses remain `no-store`, so new builds
advertise their new asset hashes immediately.
`public/favicon.svg` is the single static icon source used by HTML, the React
logo, release bundles, and the repository README.

The HTML entry includes a `sing-box-panel-appearance` meta element containing
only the saved `theme`, `color`, and `radius`. The server reads this public visual
projection from the current settings for each document request, including login
and deep links; if reading fails, it uses the startup appearance so login remains
available. Full panel settings and credentials remain authenticated.
The global `ThemeProvider` applies these values before the first interface paint
and owns the shared color, contrast, and radius tokens for every route. Login
inherits the same page background and surface tokens as the panel. Authenticated
settings loads and saves update this shared state; unsaved appearance previews
are cleared when leaving settings or ending the session. The appearance selector
edits the server default in the settings draft; only saving settings writes it to
the backend. The sidebar shortcut immediately changes a separate browser preference
through a Zustand store with `persist`, using the existing `sing-box-panel.theme`
localStorage key and raw light/dark/system values. It makes no API requests and
never edits or dirties the settings draft. The store synchronously restores that
choice on page load, overriding the server theme even after settings load, save,
or discard. Missing, invalid, or inaccessible local values leave the server default
in effect; loading a server default does not write a browser override. Color and
radius still follow the server settings and their previews. An explicit local
system choice follows live OS changes. Without server metadata (Vite/demo), the
default palette provides the initial appearance beneath any local theme override.
Session checks, panel initialization, and route loading share `LoadingState`:
a centered circular breathing indicator and a status label, without a card or
skeleton bars. It uses the active theme and stays still under reduced motion.

At runtime the browser client uses same-origin `/api/v1` endpoints for the
session, live dashboard context, the single saved configuration, panel settings,
subscription publication, persistent logs and exact core-artifact operations. Cookie-backed writes retain
the session CSRF token, and saved-file writes include the numeric revision in the request.

Operations await the completed API resource; version/source actions update in
place and failures retain previous usable state. Runtime controls verify observed
process identity before reporting success. Version management displays the cached
catalog immediately, initializes a missing cache, and offers a forced refresh;
the running server handles periodic refresh using the configured interval.
Panel logs show five centered, single-line columns: occurrence time, level,
event summary, event source, and a details action. Known event names follow the
interface language; unknown events retain their original messages. The summary
contains only the event name, with overflowing text truncated. A read-only modal
shows the selected record's stable snapshot, recorded context, and collapsible
JSON, with a Copy log action. Basic information and context sit
side by side on desktop and stack on narrow screens; the header and actions stay
visible while the details scroll. Closing it preserves list state and
restores keyboard focus. Native process output and telemetry use authenticated
streams with bounded buffering and reconnect.

Dashboard reconnect feedback stays centered inside the shared runtime toolbar,
using the existing status Badge and Tooltip styles. Narrow toolbars temporarily
show the message in the metrics slot without resizing the header or page content;
the full explanation is available on hover, keyboard focus, or tap. Normal
authenticated stream rotation has a five-second reconnect grace period; transport
errors are reported immediately and the last snapshot remains visible until a
new one arrives. Reconnect feedback does not insert a row above the dashboard.

Version, subscription source, node, channel, key, and panel-log lists use the
shared `components/list-pagination` footer. It combines the page-size selector,
circular previous/next buttons, current-page input, and read-only total.
Enter or blur commits a page; Escape cancels an edit.
Page numbers are clamped to the filtered list, and changing the tab, search or
page size resets to the first page. The total follows the selected page size;
empty lists display 1 / 1 with navigation disabled.
Key and panel-log lists request server offsets and matching totals; arbitrary
page jumps do not load all records into the browser. Counts and list items are
read from the same database snapshot, and shrinking lists return to a valid page.
Panel-log search matches both Chinese and English event names by passing exact
`search_codes` alongside the original `search` text. The server unions those
matches before applying other filters, counting, and pagination. The demo client
uses the same semantics and includes runtime transitions with their saved context.
Enabled buttons, selectors, menu options, tabs, and choice controls use a pointer
cursor. Disabled controls retain unavailable feedback; text inputs remain editable.
Keyboard focus does not add accent-colored borders, outlines, or halos to controls
or content regions. Neutral field focus borders, menu highlights, selection states,
and validation feedback remain separate from this presentation rule; keyboard
navigation and focus restoration are preserved.

Tests inject an `ApiClient`, keeping pages independent from `fetch` while the
HTTP client has focused tests for base-path routing, CSRF, problem details, and
revision preconditions. The first visit selects Simplified Chinese for a
`zh-CN` browser preference and English otherwise; an explicit user selection
is persisted and takes precedence on later visits.

The configuration UI combines version-scoped RJSF controls with a lossless
sing-box JSON editor. It preserves unknown fields and unmodified large-number
lexemes, and never adds panel metadata to executable configuration. Versions
without a committed native or reviewed Schema use the Advanced editor only. The Web UI offers Save and Validate, with validation feedback in a Toast.
Visual fields, Advanced JSON and visual module navigation share one in-memory
draft for the authenticated session. Internal tab changes retain that draft;
only leaving the configuration route or unloading the page invokes the
unsaved-change guard.
Start/Restart validate saved bytes using the selected exact binary before
replacing the process. Immutable snapshots support runtime verification and recovery;
the editor has one current document with no historical selection or restoration.

Configuration modules are edited individually, with optional object settings
added on demand. Sections with child tabs keep collection actions in the toolbar
beside those tabs.
Standalone top-level collections show identifier, type, details and actions
columns after a small top inset. Inbound and outbound tables replace details with listen and
server addresses respectively, including configured ports and bracketed IPv6
hosts. Port-hopping ranges are retained; missing address parts display a dash.
A full-width dashed Add row is the final row
inside the table, including empty lists. Actions show Edit, up/down arrows and
Delete together, with identity repair available for malformed nodes. Up/down
buttons replace node dragging; node deletion retains its confirmation dialog.
Narrow screens scroll these tables horizontally within the list. List cells are centered and entry names
are display-only. Experimental tabs follow Clash API, V2Ray API, cache file and
Debug order, showing only groups available in the selected Schema. The version
selector contains its label and a compact empty-state value in the same control.
The visual editor scrolls at the workspace edge, with an inset keeping controls
clear of the scrollbar. The Add row has a small gap and its own dashed outline
without a doubled divider. Scalar field descriptions appear in an information
tooltip beside the label, available on hover or click/tap; validation
errors remain visible beside the control.
Chinese field names and help cover the committed schema inventory, including
list and optional-object headings. Help shows the description without a separate key heading and uses
context-specific wording for DNS, routing and protocol options. See the
[field-help contract and sources](../docs/guides/configuration-and-runtime.md#field-names-and-inline-help).
Panel settings use the same `InfoTooltip` and information icon for field help.
It reuses the existing `Tooltip` component, including its styling and shared
provider: opening another tooltip dismisses the previous one. Hover previews
open after 200 ms; click/tap opens immediately. Help icons are excluded from Tab
navigation and show no focus border or ring. Clicking an open help
trigger keeps its tooltip visible. Leaving the trigger and tooltip, losing
focus, Escape or outside press dismisses it. Action-button tooltips retain their
ordinary click-to-dismiss behavior.
Adding a record or choosing its Edit action opens a dialog; confirming updates
the draft and cancelling discards the pending changes. Simple entry dialogs use a
compact, content-height layout with each label, help icon and control on one row
when the form has enough space; narrower forms stack labels above their controls.
Node tag and protocol fields are stacked vertically.
Simple entry fields omit row dividers, and user entries place the username before
the password. Sections containing a single empty collection center their empty
message in the detail area without a trailing divider. User lists show the actual
username or name and the actual authentication credential values, with long values
wrapping within their column instead of showing irrelevant type and details columns.
Unnamed users are labeled explicitly. Optional settings keep
their title and divider in place; Configure switches to a red Remove settings
button in the same header position when the section is present.
Configured optional objects share the connection-options group's left divider
and inset, including nested settings; their headers stay outside this inset.
Routine instructions are kept available to screen readers without occupying visual
space, and tag guidance appears only when the value is invalid. Complex dialogs
use a wide layout with a fixed header and footer, a vertical section list on the
left, and detailed configuration on the right. This shared layout applies to node
creation/editing and all schema array entry dialogs, including DNS and route rules.
The sections group the
available fields into basic settings, authentication, TLS, transport, connection
and advanced settings; rule dialogs separate matching conditions from actions.
Fields use a single column even in complex sections, leaving enough space for long
labels beside their controls; narrow forms place labels above controls. A sole tab is hidden. On narrow screens
the navigation rail becomes slimmer and long labels wrap. Navigation and the active
configuration panel scroll independently. Up/down keys navigate sections. Switching tabs
keeps the same form mounted, preserving pending edits and optional-object state.
Editing a discriminated object's fields also commits the displayed branch's required
identity. Opening tabs alone does not enable optional settings. Existing branch
fields remain visible when a missing discriminator needs repair.
Scalar/list choices use Single value and List labels. Bare nested `anyOf` alternatives
share one representation selector, with a distinct Byte sequence option where supported;
existing strings, byte sequences and mixed lists retain their original representation.
Credential fields use plain text in configuration and subscription-node forms,
including private-key lists, consistently with the authenticated JSON editors.
Supported credentials have a dice action at the right of the input. The
presentation-only rules in `configuration-credentials` select the appropriate
password, UUID or Base64 key format. Shadowsocks method context follows nested
user and destination dialogs, including pending edits. Grouped subscription-node
forms retain the protocol as a hidden discriminator so generation uses the current
protocol and encryption method. See the
[configuration guide](../docs/guides/configuration-and-runtime.md#reviewed-configuration-schemas)
for the generated formats.
Managed entry editors include their lossless JSON preview in the same tab bar.
The preview follows the active theme, fills the available detail area and scrolls
internally. Its upper-right Copy button copies the current entry draft without
changing numeric precision. Nested lists separate rows without a trailing row
divider or a duplicate outer divider. Text selection uses the theme's primary
color and contrasting foreground in both content and form controls.
Node creation uses a searchable protocol input with keyboard completion and a
separate dropdown button matching the input height. Focusing or clicking the input does not open suggestions; typing
filters suggestions, and the dropdown button opens the full list. Clearing the
input closes suggestions. Only protocols in the selected schema can proceed. Its popup
always opens below the input, is capped at 18rem and the available viewport height,
and scrolls internally. Cancel uses a secondary color and Continue is primary;
both footer actions use text without icons.
Map fields keep keys separate from typed text, list or object
values, and referenced scalar lists are edited inline. Dialog content remains
mounted through the synchronized closing transition. Navigation to another route,
query or hash tab prompts before discarding unsaved edits, including incomplete
JSON. Keep editing preserves the current URL, draft and concurrency revision;
Discard changes resets the draft and continues to the requested destination.
Subscription areas return to their initial lists after leaving. Reloading or
closing the page uses the browser's native unsaved-changes prompt, and signing
out requires confirmation while edits are pending. No draft or embedded
configuration secrets are written to browser storage. Server startup persists an
empty configuration if none exists. The file API uses its own numeric compare-and-swap revision;
immutable canonical revisions remain internal runtime evidence.

The interface uses the saved accent color with floating frosted navigation and
runtime controls. New configurations default to the system color scheme. The
server embeds nonce-authorized background styles so the saved palette also
appears before JavaScript starts. Explicit light/dark preferences remain supported. Larger material surfaces use blur and translucent fills;
content stays on readable surfaces. Navigation selection uses a non-bouncing
spring, and controls respond on press. Reduced motion, reduced transparency,
and increased contrast preferences are respected.

The six main pages omit the visible page-title band while retaining a screen
reader heading. Their content fills the space below the runtime toolbar and
ends at the same bottom inset as the sidebar. Tables and forms scroll within
that space when needed, keeping panel actions reachable. The dashboard scrolls
in the full-height shell behind the floating runtime toolbar; the toolbar
surroundings remain transparent and the scrollbar sits at the viewport edge.

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
