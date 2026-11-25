# Sing-Box Panel web shell

This directory owns the React/Vite frontends as one pnpm workspace. They
deliberately have no dependency on backend implementation packages.

Install the [development toolchains](../.github/CONTRIBUTING.md#development-setup) first.
Schema preparation requires Go for offline export, including when running frontend tests.

## Workspace

| Package | Path | Owns |
| --- | --- | --- |
| `@sing-box-panel/panel` | `apps/panel` | The panel's application, embedded by `sing-box-panel` through the Go package in `apps/panel` |
| `@sing-box-panel/node` | `apps/node` | The node console's application, embedded by `sing-box-node` through the Go package in `apps/node` |
| `@sing-box-panel/shared` | `packages/shared` | What both applications use: the `components/ui` controls, `lib/utils`, `hooks/use-mobile`, the styles and tokens, the theme preference primitives, language detection and i18n setup, and the favicon |

Each application imports its own code through `@/` and the shared package
through `@shared/`, both mapped in its `tsconfig.json`. Shared code imports
neither application, and neither application imports the other; ESLint
enforces both, because each binary embeds only its own application. The
workspace root holds the toolchain: the lockfile, the development dependencies,
the ESLint configuration, and scripts that run each application's own.

## Development

Frontend work runs against the real services: start the service an
application talks to, then start that application's Vite development server,
which proxies `/api` to the service on this host.

```sh
corepack enable pnpm
pnpm install --frozen-lockfile --ignore-scripts --verify-store-integrity
```

The panel application expects a panel listening on `127.0.0.1:3000`. From the
repository root, build it, [initialize its settings](../docs/getting-started.md#initialize-settings-and-storage)
and start it:

```sh
make build-panel
./bin/sing-box-panel init --config ./setting.json
./bin/sing-box-panel server start --config ./setting.json
```

Then start the application from this directory and sign in with the
administrator account that `init` printed:

```sh
pnpm --filter @sing-box-panel/panel run dev
```

Keep the panel's `server.base_path` and `server.external_origin` empty while
developing: the proxy forwards `/api` unchanged and keeps the browser's `Host`,
so the panel's origin and CSRF checks accept the development server. Pages that
show a node's cores, configuration and telemetry need an enrolled node; register
one with `sing-box-panel node add` and run `sing-box-node` as described in
[Nodes](../docs/cli.md#nodes).

The node console application expects a node whose console listens on
`127.0.0.1:8000`; its first start prints the console password. Start the
application with `pnpm --filter @sing-box-panel/node run dev`. The node runtime
requires Linux, so on another system run the node in a Linux container that
shares the host's network.

`.claude/launch.json` starts all four: `.claude/dev/panel.sh` runs the panel,
`.claude/dev/node.sh` runs a node enrolled with it in a Debian container on the
host network, and each application's development server runs on `5173` (panel)
or `5174` (node console). Their binaries, settings and data stay in the ignored
`.claude/dev/state/`; the first start prints the credentials once.

Use `make check-frontend` from the repository root for a production build, types,
lint, all tests, Schema consistency and notices checks. From this directory,
`pnpm run build` builds both applications and `pnpm test` runs every package's
tests. For a focused panel check, run `pnpm exec vitest run --project logic`,
`--project react`, or `--project contracts` from `apps/panel`.

Run `pnpm run lint:fix` to apply the configured TypeScript, React, CSS, and HTML
formatting rules before checking the remaining code-quality diagnostics.
Test environments, timeout bounds and coverage ownership are documented in
[Testing](../docs/testing.md).

## Node console

`apps/node` is the node console's application, served by `sing-box-node server
start` on the console's address. It signs in with the console password and
shows the node, the panel it is enrolled with and the state of its session,
refreshed every five seconds; the last status stays visible, marked stale,
when the node stops answering. Its transport types under `src/api/generated/`
are generated from `../../../api/node-openapi.yaml`. It follows the system's
light or dark appearance.

## Source layout

The sections below describe the panel's application; its paths are relative
to `apps/panel/` and shared modules are named with `@shared/`.

`src/api/api-client.ts` exposes the client interface and simple generated
transport types directly. Modules in `src/api/contracts/` own additional
domain-specific query filters and derived types. Routes are assembled in
`src/routes/app.routes.tsx`; complex component entries remain narrow boundaries
around their companion files. Each locale groups navigation, account, language,
and theme labels in `shell.ts`, with shared and startup messages in `common.ts`.

The first visit selects Simplified Chinese for a `zh-CN` browser preference and
English otherwise; an explicit user selection is persisted and takes precedence
on later visits.

`components/server-path-input` owns the shared server filesystem selector.
Configuration and subscription-node forms use it through context-aware presentation
annotations in `configuration-path-fields`, including scalar/list representations
and nested dialogs. Filesystem reads go through the injected `ApiClient`. The
chooser preserves hand-entered values until confirmation and rechecks the
selected path with the server.
See [server path selection](../docs/configuration-and-runtime.md#selecting-server-paths)
for filesystem scope and path semantics.

## Generated inputs

Transport types under `src/api/generated/` are build artifacts generated from
`../../../api/panel-openapi.yaml`. Development, typecheck, test, and build commands refresh
them automatically; they are not committed or edited by hand. The custom HTTP
client remains hand-written. `pnpm run generate` prepares the transport types;
ordinary development and validation commands include this step, so no separate
preparation command is needed.

The application bundles no configuration schema. The visual editor renders the
schema the panel serves for the selected core, after checking that it names that
core's version and is an object; it does not validate against it, and its form
validator only matches values to schema branches. The panel answers
`configuration_schema_unavailable` for a version outside every
[release line](../docs/core-versions.md#release-lines), which keeps the visual
editor tab disabled; any other failure shows a notice with a retry, and the
Advanced JSON editor stays available either way. The configuration version
selector lists compatible installed core versions, prefers the enabled version,
and otherwise selects the highest installed exact version. Without an installed
core, Advanced JSON editing and saving remain available while structured editing
and validation direct the operator to version management. The form library is
imported from its `lib/` modules, because its package entry also loads a test
registry that needs the Ajv validator.

## Build output and embedding

`pnpm run build` produces a deterministic SPA bundle for each application in
`apps/panel/dist/` and `apps/node/dist/`. Each application's Go package embeds
its directory, and its binary uses `index.html` for client-side routes. `dist/`
is generated output and is not a source of truth, but it must exist before the
Go package that embeds it is loaded.
Content-hashed JavaScript and CSS assets are served with a one-year immutable
cache lifetime. HTML and SPA fallback responses remain `no-store`, so new builds
advertise their new asset hashes immediately.
`packages/shared/public/favicon.svg` is the single static icon source used by
both applications' HTML, the React logo, release bundles, and the repository
README.

## HTTP client and authentication

At runtime the browser client uses same-origin `/api/v1` endpoints for the
session, the fleet and node enrollment, panel settings, subscription
publication, panel logs and the core catalog. Everything about one node goes
through `/api/v1/nodes/{nodeId}/…`: the node workspace gives its pages a client
scoped to that node, so its cores, configuration, runtime, logs, telemetry and
settings never reach another node. Cookie-backed writes retain the session CSRF
token, and saved-file writes include the numeric revision in the request.

Production login lazily collects a FingerprintJS visitor ID with the locally
bundled SDK and sends it only in `X-Client-Fingerprint`. Collection is bounded to
1.5 seconds, SDK monitoring is disabled, and the ID is kept only in page memory.
Collection failure falls back to login without a fingerprint; server peer/global
limits always apply. Fingerprints do not authenticate a user. See the
[authentication contract](../docs/architecture.md#management-authentication)
for limits, proxy behavior and privacy boundaries.

## Data loading and caching

The status line's windows, on the desktop line and the phone tab bar, preload
route modules on hover and keyboard focus, and the command line preloads the
window it opens, so navigation shares each pending download. A failed speculative import
does not interrupt the current page; the next unloaded route reloads the document
to recover Vite's shared CSS/module preload state. A view that fails, such as a
route module that fails to load without a preload, shows one line in its place
inside the shell with a reload action, and opening another window leaves it; a
failure outside the shell shows the router's root fallback with the same
action. Subscription lists and their node catalog load
concurrently, and the version library reads runtime status only once on entry.
Runtime responses are published independently of artifact listings and discarded
when newer shared runtime evidence arrives while the request is pending.

TanStack Query v5 owns session-local query state: the fleet status, fed by the
fleet stream, the fleet's node list for the configuration copy dialog (15
seconds), installed core lists (15 seconds), release catalogs (60
seconds), subscription summaries and node catalogs (5 seconds), and resolved
artifact-bound Schema contracts (5 minutes). Node-scoped reads carry the node in
their key, so nodes never share cached data. Inactive queries expire after 5 minutes. Configuration,
credentials and file content are read on demand; settings are retained only by
an active provider. No query data is persisted in browser storage; it holds only
the theme and language preferences (localStorage) and the marker that allows one
reload per panel version after an update (sessionStorage). Shared query keys
coalesce pending reads and preserve unchanged object references. Background
refresh retains content, pagination, selection and scroll position. GET requests
have a 15-second total budget and retry a network/temporary service failure once;
writes are never automatically retried. Writes invalidate related resources,
apply returned evidence, and reconcile ambiguous failures. A fleet batch streams
its results, so its reader invalidates once the batch ends what its single-node
commands change: the fleet, and the touched nodes' runtime status and installed
cores. Session changes cancel
queries, clear the cache and reject responses from earlier sessions.

The shell renders after authentication while settings and page queries load
independently. An open node's workspace reads its node from the fleet status,
as the status line and the panes do, so they never disagree on its state; a
failed read keeps the last report, the workspace and drafts, and the workspace's
views and drafts stay mounted while the node goes offline or comes back.
Sources poll every 15 seconds only while their page and browser tab are visible;
sources and channels share the node catalog. Batch source refresh uses at most
three concurrent operations and one final list refresh. The backend also limits
remote refresh concurrency to three across callers. Drafts retain their revision
until explicitly saved/restored so background reads cannot bypass conflict checks.

Operations await the completed API resource; version/source actions update in
place and failures retain previous usable state. Runtime controls verify observed
process identity before reporting success. Version management displays the cached
catalog immediately, initializes a missing cache, and offers a forced refresh;
the running server handles periodic refresh using the configured interval.

## Telemetry

The shell follows the panel's fleet stream while the page is visible and
applies each event to the fleet status query: a reset event replaces the fleet,
later events replace the nodes that changed and drop the removed ones, in the
panel's order. The nodes home, the status line, the command line and the node
workspaces read one live fleet without polling. While the stream drives the
query, a status read never replaces it: returning to the tab restarts the
stream, whose first event carries every node, instead of reading the status,
and a read that an event overtakes keeps the event's fleet. A stream that stays
down for its five-second grace hands over to one read of the status, whose
failure marks the last reading as no longer updating.

Each node workspace owns one telemetry store for that node, a stable Zustand
store with separate subscriptions for runtime, metrics, history and connection
health. Its streams run only while the node is online; going offline drops its
readings. History is subscribed only by the node's overview; core logs only by its
visible logs tab. The live metrics stream stays
connected across browser tab switches so received samples remain continuous.
Hidden tabs pause history, logs and polling until they become visible again.
Normal server rotation sends a control event and reconnects immediately. Other
failures use jittered exponential backoff (at most 30 seconds) and a five-second
notification grace period. A 25-second idle watchdog detects half-open streams.
Live snapshots become stale after 10 seconds, history after 45 seconds. History
delays stay in the overview and do not trigger the global live-connection Toast.
Period traffic reads the recorded monthly ledger independently of live sampling:
empty periods show zero, stopped/stale collection retains usage and quota status,
and no period-completeness label is displayed.
Chart updates reuse uPlot instances and batch cursor feedback per animation frame.
The default Live traffic chart retains the last 120 seconds of received samples,
independently of the historical persistence watermark. One-hour and 24-hour
traffic views show historical one-minute and five-minute averages. The collector
supplies rates from adjacent observations on a one-second sampling cadence;
the browser does not derive rates from stream delivery intervals. Missing rates
or samples older than three seconds show `—`, while measured zero remains zero.
Live chart samples preserve gaps and never contribute to accounted traffic totals.

The runtime timeline keeps 48 half-hour segments with time marks anchored to the
overview snapshot, including dates at both endpoints. Narrow cards retain the
start, midpoint and end marks. Segment tooltips expose the complete interval and
observed state on hover or keyboard focus. Recent activity keeps two compact rows
with the same localized event names and severity badges as panel logs. Each row
opens the panel logs page; its tooltip exposes the full name and occurrence time.
Activity times include dates for earlier days and years for earlier years, and
update at local midnight independently of delayed runtime snapshots.

## Shell, theme and loading

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
the backend. The status line's theme action immediately changes a separate browser preference
through a Zustand store with `persist`, using the existing `sing-box-panel.theme`
localStorage key and raw light/dark/system values. It makes no API requests and
never edits or dirties the settings draft. The store synchronously restores that
choice on page load, overriding the server theme even after settings load, save,
or discard. Missing, invalid, or inaccessible local values leave the server default
in effect; loading a server default does not write a browser override. Color and
radius still follow the server settings and their previews. An explicit local
system choice follows live OS changes. Without server metadata (the Vite
development server), the default palette provides the initial appearance beneath any local theme override.
Session checks, route loading, and initial page/Tab data loading share
`LoadingState`, whose appearance follows the loading-region pattern in
[`DESIGN.md`](../DESIGN.md). Its brief delayed fade-in avoids flashes
for fast loads without delaying ready content. Loading stays within the pending
content region, retaining navigation, toolbars and any usable data. Reduced motion
shows the indicator immediately and keeps it still.

Colours, type, surfaces, radius and motion come from the tokens described in
[`DESIGN.md`](../DESIGN.md); `src/styles/global.css` imports `tokens.css`,
`base.css` and `components.css`. New configurations default to the system color
scheme. The server embeds nonce-authorized background styles so the saved palette
also appears before JavaScript starts. Explicit light/dark preferences remain
supported. Reduced motion, reduced transparency, and increased contrast
preferences are respected.

The shell is one scrolling content region with a status line below it. The
status line leads back to the fleet (`fleet`, the same in every scope), lists
the scope's windows as numbered tabs (`1 nodes` to
`4 settings`, or a node's `1 overview` to `5 settings`), shows the fleet
readout and the panel's update, and carries the theme cycle, language menu and
sign-out; below 640px it is a tab bar along the window's foot with a more
menu. Window names are command words in every language; their tooltips and
accessible names are translated. Pages omit a visible title band while
retaining a screen reader heading: every view starts with one title strip
holding its toolbar in flat controls grouped by dividers, or in a node the
previous and next nodes, its name, state, labelled readings and runtime
controls. Tables and forms scroll within the space above
the status line, keeping their actions reachable.

Ctrl or Cmd+K, `:` outside fields and dialogs, or the status line's prompt
action opens the command line (`components/status-line`). It parses commands
in `utils/panel-commands.ts` (`help`, `clear`, `logout`, window numbers,
`open`, `goto`, `filter`, and `start`, `stop`, `restart` and `update` on a name,
a glob, `tag:<tag>` or `all`), completes the word under the caret, and prints to
one output panel above the line that stays until closed. Batches from the
command line or from selected panes confirm first, run one at a time, stream
each node's result from `/fleet/commands` into output that keeps room for every
node's line, and keep running when the line closes, ending in a toast then,
which also says when a batch stopped before its summary. The output, a pending
confirmation, the command history, the home screen's filters, selection mode
and selection, and the enrollment dialog end with the signed-in session; a batch still running then finishes on the
panel without printing into the next session. The terminal parts are shared
with the node console from `packages/shared/src/components/terminal`.

The nodes home renders every node as a memoized pane that redraws only when
its own fleet entry changes or the shared 30-second clock moves; panes off
screen skip layout and paint through `content-visibility` with their fixed
height. Selection mode lives in `stores/fleet-view.store.ts`; a filter or search
that hides selected panes drops them from the selection, so a batch from it
names only panes on screen, while command-line targets address the whole fleet. The shell holds
the one enrollment dialog (`components/enrollment-dialog.tsx`, opened through
`stores/enrollment-dialog.store.ts`), so a view that remounts while a host
enrolls keeps it; the dialog reads the fleet status the stream feeds, polls
while the stream is down, and discards an unused code when it closes or the
page goes away. The browser review harness seeds a fleet with
`SBP_BROWSER_FLEET=<n>` (`panel-review` in `.claude/launch.json` runs 200),
half of the nodes online with live readings; without a seeded fleet
(`panel-review-empty`) nodes can enroll with `sing-box-node enroll` on the
same port.

## Login

Login uses one responsive frame for session checking, sign-in, and service
unavailability. Its decorative geometric characters respond to pointer and form
focus. All three remain front-facing and close their eyes while the password
field or visibility toggle is focused, or while the password is visible. They
reopen their eyes when the password is hidden and focus leaves that field. The
scene receives presentation state only, never credentials. Motion stops at rest;
when their eyes are open, each character's pupils track a fine mouse pointer
across the page while staying inside fixed eye whites. Reduced motion
switches poses without animation. Form focus, error notifications, and
authentication behavior remain independent of the scene.

## Shared controls and lists

Path pickers and log-file filters reuse the shared Input, InputGroup and Select
components, so their sizes and states follow the field and single-choice patterns
in [`DESIGN.md`](../DESIGN.md). Options never change the value until one is chosen.

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
matches before applying other filters, counting, and pagination.
Overview activity links and runtime segments use explicit keyboard focus outlines
for inspection; keyboard navigation and focus restoration are preserved everywhere.

Workspace tables for versions, sources, keys, channels and panel logs use the data
table pattern and table measurements in [`DESIGN.md`](../DESIGN.md).

## Pages and navigation

The four navigation entries and a node's five tabs host every view; details
use local tabs and dialogs instead of separate pages. Sources, manual nodes, subscription keys and native
channel rules share node provenance and independent visibility. Channel preview
and delivery have one server renderer; remote rule references are fetched only
by subscribing clients. Native editors preserve unknown fields and scalar/list
representations. Appearance is a saveable preview transaction shared by controls,
charts and overlays, with semantic status colors kept independent.

Management tabs use URL fragments: `#cores-installed` / `#cores-catalog`,
`#configuration-visual` / `#configuration-advanced` (visual sections append `/route`,
`/inbounds`, etc.) inside a node, and `#panel-service`, `#panel-traffic`,
`#panel-logs`, `#panel-interface`, `#panel-maintenance` and `#panel-backup` in
panel settings. Direct links, reloads and browser history restore the selected
tab. Tab switches preserve query parameters and route state. Subscription tabs retain
`#subscription-sources`, `#subscription-tokens`, and `#subscription-channels`.

## Panel logs

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

## Configuration editor

### Drafts and validation

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

### Modules and field help

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
[field-help contract and sources](../docs/configuration-and-runtime.md#field-names-and-inline-help).
Panel settings use the same `InfoTooltip` and information icon for field help.
It reuses the existing `Tooltip` component, including its styling and shared
provider: opening another tooltip dismisses the previous one. Hover previews
open after 200 ms; click/tap opens immediately. Help icons are excluded from Tab
navigation and show no focus border or ring. Clicking an open help
trigger keeps its tooltip visible. Leaving the trigger and tooltip, losing
focus, Escape or outside press dismisses it. Action-button tooltips retain their
ordinary click-to-dismiss behavior.
### Entry dialogs

Adding a record or choosing its Edit action opens a dialog; confirming updates
the draft and cancelling discards the pending changes. Simple entry dialogs use a
compact, content-height layout with each label, help icon and control on one row
when the form has enough space; narrower forms stack labels above their controls.
Dialog footer actions align right, with an outline Cancel first, auxiliary
actions next and the primary action last. Destructive actions retain their danger
style. Cancel only dismisses the dialog or its pending edits; Apply updates a
parent draft, while Save retains its existing persistence behavior. Channel
settings order Channel name, Client, New-node policy and Config template, with
each label and control on one row even on narrow screens.
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
the section list becomes slimmer and long labels wrap. The section list and the active
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
user and destination dialogs, including pending edits. Subscription-node forms
use the same vertical section navigation and single-column field rows as configuration
entry dialogs, with fixed headers and footers. The top-level Visual editor,
Advanced JSON and Import share link modes remain separate. Protocol and endpoint
choices appear in Basic settings, SSH authentication in Authentication, and detour
selection in Connection options. One mounted schema form preserves pending fields
across section changes and retains the protocol as a hidden discriminator so
generation uses the current protocol and encryption method. See the
[configuration guide](../docs/configuration-and-runtime.md#configuration-schemas)
for the generated formats.
### Previews and unsaved changes

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
and scrolls internally. Cancel uses an outline style and Continue is primary;
both footer actions use text without icons.
Map fields keep keys separate from typed text, list or object
values, and referenced scalar lists are edited inline. Dialog content remains
mounted through the synchronized closing transition. Navigation to another route,
query or hash tab prompts before discarding unsaved edits, including incomplete
JSON. Cancel preserves the current URL, draft and concurrency revision;
Discard changes resets the draft and continues to the requested destination.
Subscription areas return to their initial lists after leaving. Reloading or
closing the page uses the browser's native unsaved-changes prompt, and signing
out requires confirmation while edits are pending. No draft or embedded
configuration secrets are written to browser storage. Adding a node initializes
its configuration to `{}`. The file API uses its own numeric compare-and-swap revision;
immutable canonical revisions remain internal runtime evidence.

## Error feedback

Request failures and validation errors use the shared Toaster instead of in-flow
banners or field-error labels. Use `toast.add` for an action failure and
`ErrorNotice` for an error represented by component state. Repeated state errors
reuse one notification; recovery or unmount closes it. Keep invalid controls
marked with `aria-invalid`, and retain visually hidden descriptions where
`aria-describedby` needs them. Error feedback must not take focus away from the
current control or shift the surrounding layout. Service-unavailable screens
keep their retry action available after the notification is dismissed.

## Testing

Tests inject an `ApiClient`, keeping pages independent from `fetch` while the
HTTP client has focused tests for base-path routing, CSRF, problem details, and
revision preconditions.
