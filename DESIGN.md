# sing-box-panel design system

The design system of the sing-box-panel web interface: roles, scales, formulas, patterns and rules. It extends the global design guide and repeats nothing from it. Every value is the one the tokens in code use; a change to one changes the other in the same work.

The panel interface serves two scopes. The panel scope covers the fleet: the overview, the nodes, subscriptions, the panel log and panel settings. The node scope covers one node: its overview, core versions, configuration, logs and settings. The node console is a separate interface that one node serves on its own host for its enrollment with a panel and a read-only view of its state; it uses the same tokens, type and components.

## 1. Character

| Property | Declaration |
| --- | --- |
| Mood | Calm and technical; the interface reports state and stays out of the way |
| Colour temperature | The terminal's cool neutral greys, the same under every accent |
| Density | Dense working screens; data at 12–14px, one toolbar row per view |
| Type | One bundled variable sans at three weights; one bundled monospace for machine text |
| Surface material | Cards and the rail at 72% opacity, node cards at 90%, over a canvas that carries a faint glyph field, separated by hairlines; fields, menus, dialogs and toasts stay opaque, and floating layers detach from the window edge and cast a shadow |
| Motion | Immediate response to input, short state transitions; the glyph field's slow rewriting is the only idle motion |

Properties that make the product recognisable:

- The neutral scale is the terminal's greys and the saved accent touches only actions, selection and focus, so every palette keeps the same calm surfaces and accessible text.
- A field of protocol glyphs sits behind every view, interactive at sign-in and calm once signed in, where it mixes in the product's own values; both services read as one product.
- One radius parameter derives three corner steps, so a single setting reshapes every element in proportion.
- Status colours never take the accent, so a state reads the same under every palette and in both themes.
- The navigation rail floats inside the window with an even inset, so the shell reads as a layer above the workspace.
- Every view declares its scope: panel views name the fleet, node views keep the node's identity and state in their first row.

## 2. Colour

Light values sit in the root scope, dark values in the dark scope. Neutrals are fixed. The accent roles are recomputed from the saved accent A and the card surface S (light `#f8f8fa`, dark `#25262f`) before first paint; the table shows the result for the default A `#6D4ED1`.

| Role | Token | Light | Dark | Use |
| --- | --- | --- | --- | --- |
| Canvas | `--color-canvas` | `#eeeef2` | `#1c1d24` | Window background behind the rail and workspace; carries the glyph field |
| Surface | `--card`, `--color-surface` | `#f8f8fa` | `#25262f` | Sticky table headers, outline actions, the phone navigation sheet, the terminal window |
| Surface translucent, dense | `--color-surface-translucent`, `--color-surface-dense` | Surface at 72%, at 90% | Same | Cards, tables, forms, object rows, the rail; node cards dense, where live figures sit close together; Surface under more contrast |
| Field | `--color-field` | Surface | Surface with 30% of Border | Text fields and select triggers, opaque so no glyph sits behind typed text |
| Popover | `--popover` | `#fcfcfd` | `#2b2c36` | Menus, select lists, popovers, dialogs, sheets, toasts |
| Surface raised | `--color-surface-raised` | `#f0f0f4` | `#2d2e38` | Nested blocks, hovered rows, disabled fields |
| Muted fill | `--muted` | `#f0f0f4` | `#2b2c36` | Secondary actions, segmented tracks |
| Border | `--border`, `--input` | `#e2e2e9` | `#34353f` | Hairlines, dividers, input borders |
| Strong border | `--color-border-strong` | `#cfcfd9` | `#464753` | Hovered inputs |
| Text | `--foreground` | `oklch(24% 0.015 280)` | `#f8f8f2` | Primary text and icons, filled meter cells |
| Secondary text | `--color-text-secondary` | `oklch(40% 0.015 280)` | `oklch(84% 0.012 280)` | Field labels, secondary cells |
| Muted text | `--muted-foreground` | `oklch(50% 0.015 280)` | `oklch(72% 0.02 280)` | Meta, table headers, timestamps, terminal labels |
| Faint text | `--color-text-faint` | `oklch(54% 0.012 280)` | `oklch(62% 0.018 280)` | Placeholders, disabled text, the glyph field |
| Selection | `--color-selection` | `oklch(24% 0.015 280 / 14%)` | `oklch(97% 0.01 100 / 20%)` | Text selection, which keeps each text's colour; scrollbar thumbs |
| Accent | `--primary`, `--ring` | `#6d4ed1` | `#a08ce1` | Primary action, keyboard focus, links, prompt symbols and cursor |
| Accent hover | `--color-accent-hover` | `#6045b8` | `#ab9ae5` | Primary action on hover |
| On accent | `--primary-foreground` | `#ffffff` | `#17151e` | Text and icons on an accent fill |
| Accent soft | `--accent` | `#ebe9f6` | `#312c49` | Selected navigation, selected rows, pressed toggles |
| Success | `--color-success`, `-soft`, `-text`, `-border` | `oklch(59% 0.14 165)`, `oklch(95% 0.03 165)`, `oklch(42% 0.11 165)`, `oklch(82% 0.06 165)` | `oklch(72% 0.14 165)`, `oklch(27% 0.055 165)`, `oklch(83% 0.105 165)`, `oklch(46% 0.08 165)` | Healthy and completed states |
| Warning | `--color-warning`, `-soft`, `-text`, `-border` | `oklch(65% 0.16 62)`, `oklch(96% 0.035 72)`, `oklch(46% 0.12 62)`, `oklch(83% 0.07 62)` | `oklch(78% 0.14 72)`, `oklch(28% 0.055 72)`, `oklch(84% 0.11 72)`, `oklch(48% 0.08 72)` | Degraded or stale states, pending action, one-time secrets |
| Danger | `--color-danger`, `-soft`, `-text`, `-border` | `oklch(56% 0.18 20)`, `oklch(95% 0.035 20)`, `oklch(43% 0.14 20)`, `oklch(82% 0.07 20)` | `oklch(70% 0.17 20)`, `oklch(28% 0.06 20)`, `oklch(83% 0.105 20)`, `oklch(48% 0.08 20)` | Failed or unreachable states, errors, destructive actions |
| Info | `--color-info`, `-soft`, `-text`, `-border` | `oklch(58% 0.15 248)`, `oklch(95% 0.03 248)`, `oklch(44% 0.12 248)`, `oklch(82% 0.06 248)` | `oklch(72% 0.13 248)`, `oklch(28% 0.055 248)`, `oklch(82% 0.095 248)`, `oklch(47% 0.075 248)` | Neutral notices |
| Highlights | `--color-highlight-cyan`, `-magenta`, `-yellow` | `#0b7189`, `#b4237a`, `#8a6a00` | `#8be9fd`, `#f079bd`, `#f1fa8c` | Chart series 1 to 3, download and upload; the terminal prompt's path, user and symbol |
| Scrim | `--color-scrim` | `oklch(24% 0.015 280 / 34%)` | `oklch(7% 0.01 280 / 68%)` | Behind modal layers |
| Syntax | `--color-syntax-property`, `-string`, `-number`, `-keyword` | `#245fa1`, `#21774b`, `#9a541e`, `#8c4989` | `#88bdf3`, `#9ac99d`, `#e7b685`, `#d5a0d7` | JSON syntax in the configuration editor |
| Brand | `--color-brand-navy`, `--color-brand-orange` | `oklch(20.8% 0.042 265.755)`, `oklch(70.5% 0.18 47.6)` | `oklch(17% 0.03 265.755)`, `oklch(76% 0.16 55)` | Logo mark |
| Window marks | `--color-window-close`, `-minimize`, `-zoom` | `#ff5f57`, `#febc2e`, `#28c840` | Same | The terminal title bar's three marks |

Derivation, with mix(A, S, w) as A mixed into S at weight w:

| Token | Light | Dark |
| --- | --- | --- |
| `--primary` | A moved toward black in 1% steps until it reaches 4.6:1 against `--accent` and S | A moved toward white the same way |
| `--color-accent-hover` | `--primary` mixed 12% toward black | `--primary` mixed 12% toward white |
| `--primary-foreground` | `#ffffff` when it reaches 4.5:1 against `--primary`, otherwise `#17151e` | Same rule |
| `--accent` | A at 9% | A at 16% |

Accent policy: the accent fills the one primary action of an area, marks selection through its soft fill, rings keyboard focus, colours links and the prompt's symbol and cursor. It never tints a neutral, marks a state, a badge, the logo, a data series or a destructive action, and never fills a region. Data series take the highlights: cyan for download and the first series, magenta for upload and the second, yellow for the third.

State colours, shared by runtime and node connection states in every theme and palette:

| State | Colour |
| --- | --- |
| Running, online, healthy | Success |
| Degraded, stale, restart required | Warning |
| Failed, unreachable | Danger |
| Stopped, disabled | Muted text |
| Unknown, not yet reported | A hatch of raised surface and strong border |

## 3. Type

| Role | Token | Family | Use |
| --- | --- | --- | --- |
| Interface | `--font-body` | Geist, bundled variable font; system sans for scripts it lacks | All interface text |
| Display | `--font-display` | Same family | 20px and larger, the wordmark |
| Data | `--font-data` | Geist Mono, bundled; system monospace for scripts it lacks | Logs, configuration, versions, hashes, addresses, numeric readouts |

Weights: 400 for text, values, statuses, options and row actions; 500 for labels, buttons, tabs and table headers; 600 for titles and primary readouts; 700 only in the wordmark.

| Step | Token | Size | Line height | Tracking | Use |
| --- | --- | --- | --- | --- | --- |
| 2xs | `--text-2xs` | 11px | 16px | 0 | Chart axes, dense meta |
| xs | `--text-xs` | 12px | 16px | 0 | Secondary text, badges, timestamps |
| compact | `--text-compact` | 13px | 20px | 0 | Dense rows, compact actions |
| sm | `--text-sm` | 14px | 20px | 0 | Body, buttons, inputs, table cells |
| base | `--text-base` | 16px | 24px | 0 | Dialog titles; text inputs at 700px and below |
| xl | `--text-xl` | 20px | 28px | -0.01em | Card readouts, object names |
| 2xl | `--text-2xl` | 24px | 32px | -0.02em | Key figures |

Line heights sit on the 4px grid; running text outside a type utility inherits a 1.5 line height. Labels keep sentence case and normal tracking. Rates, sizes, counts, durations, ports and aligned numeric columns use tabular numerals.

## 4. Shape and density

| Step | Token | Value |
| --- | --- | --- |
| Spacing | `--space-2xs`, `--space-xs`, `--space-sm`, `--space-md`, `--space-lg`, `--space-6`, `--space-xl`, `--space-2xl` | 4, 8, 12, 16, 24, 32, 40, 64px |
| Form field gap | `--form-field-gap` | 12px |
| Form section gap | `--form-section-gap` | 20px |
| Toolbar to content | `--workspace-content-gap` | 16px |

| Control | Token | Height | Use |
| --- | --- | --- | --- |
| Standard | `--control-height` | 40px | Fields, selects, standard actions |
| Compact | `--control-height-sm` | 32px | Actions in toolbars, table rows and pagination |
| Large | `--control-height-lg` | 44px | Large actions |
| Embedded | `--control-height-xs` | 28px | Icon actions inside a field, a card or a floating tool cluster |

On a coarse pointer every control height resolves to 44px. Controls in one row share one height.

| Element | Token | Formula | Default |
| --- | --- | --- | --- |
| Controls, inputs, tooltips, menu items | `--radius-control` | R/2 | 6px |
| Cards, popovers, menus, toasts | `--radius-card` | R | 12px |
| Floating layers: rail, dialogs, alert dialogs, sheets | `--radius-window` | min(32, 7R/6) | 14px |
| Badges, status pills, avatars | `--radius-pill` | 999px | 999px |

R is the saved radius, an integer from 0 to 32, default 12.

| Shell parameter | Token | Value |
| --- | --- | --- |
| Window inset of floating layers | `--shell-edge` | 16px |
| Rail container width | `--shell-sidebar-width` | 280px, a 248px rail inside the inset |
| Workspace top row | `--shell-header-height` | 64px; 52px at 900px and below |
| Table header row | — | 44px, no divider |
| Table data row | `--workspace-row-height` | 64px plus a 1px divider; grows when content wraps |
| Page sizes | — | 5, 10, 50 |
| Breakpoints | — | 640, 768 (the rail becomes a sheet), 1024px; views also respond to their own width |

## 5. Surfaces and motion

| Level | Surface | Separation | Elements |
| --- | --- | --- | --- |
| Canvas | `--color-canvas` with the glyph field | — | Window background |
| Content | `--color-surface-translucent`; node cards `--color-surface-dense` | `--border` hairline; `--shadow-card` only where a hairline is not enough | Cards, tables, forms, object rows |
| Raised | `--color-surface-raised` | Tint | Nested blocks, hovered rows, code, configuration and log output |
| Popover | `--popover` | Hairline and the popover shadow | Menus, select lists, popovers, tooltips |
| Floating | `--popover`; the rail `--color-surface-translucent` | Hairline and `--shadow-panel` | Rail, dialogs, alert dialogs, sheets, toasts |

| Shadow | Light | Dark |
| --- | --- | --- |
| `--shadow-card` | `0 2px 5px -2px oklch(22% 0.018 274 / 6%), 0 10px 24px -18px oklch(22% 0.018 274 / 16%)` | `0 14px 34px -24px #000000b8` |
| Popover | `0 4px 6px -1px rgb(0 0 0 / 10%), 0 2px 4px -2px rgb(0 0 0 / 10%)` | Same |
| `--shadow-panel` | `inset 0 1px 0 oklch(100% 0 0 / 65%), 0 18px 48px -28px oklch(30% 0.05 286 / 30%)` | `inset 0 1px 0 oklch(100% 0 0 / 8%), 0 28px 72px -34px #000000d1` |

| Motion token | Value | Use |
| --- | --- | --- |
| `--ease-out` | `cubic-bezier(0.16, 1, 0.3, 1)` | All movement |
| `--ease-in` | `cubic-bezier(0.7, 0, 0.84, 0)` | Exits |
| `--ease-in-out` | `cubic-bezier(0.65, 0, 0.35, 1)` | Symmetric transitions |
| `--dur-micro` | 110ms | Press feedback |
| `--dur-short` | 220ms | State changes |
| `--duration-medium` | 280ms | Page entrance |
| Overlay entrance | 150ms, opacity and 0.98 scale | Dialogs, menus, popovers |
| Press | 0.975 scale | Buttons; none under reduced motion |

Layers: sticky content `--z-sticky` 200, modal `--z-modal` 400, popover above a modal `--z-popover` 450.

## 6. Components

| Pattern | Anatomy | Variants | States |
| --- | --- | --- | --- |
| Action | Label, optional 16px leading icon | Primary, outline, secondary, ghost, link, destructive (danger soft fill, 25% danger border); standard, compact (13px text), large, embedded; each with a square icon form | Hover, pressed, keyboard focus, pending keeps the label and width |
| State indicator | Dot or 20px pill with 12px medium text | Success, warning, danger, info, neutral | Static; a change of state replaces the colour without moving the row |
| Single choice | Trigger with value and chevron; list with a checkmark on the current value | Standard, compact | A mouse-opened list keeps options neutral; keyboard highlight follows focus |
| Field row | Label column, control filling the rest, optional help trigger beside the label | Single column; stacked below 520px | Invalid marks the control; messages go to the action bar or a toast |
| Data table | 44px header, 64px rows, row actions at the end | Paged or scrolled within its region | Loading, empty and error centred in the region; selection by soft fill |
| Pagination | Page-size choice, previous and next, page input, total | Footer below its list | Enter or blur commits a page, Escape cancels |
| Toolbar row | Tabs or filters at the start, actions at the end | List toolbar, object row | Stays in place while content scrolls |
| Object row | Back, name, state, context values, actions | Node, source, channel | Actions act on the named object only; below 1024px the context values move to a second line that scrolls sideways |
| Node card | One header line: a health dot for the node's worst condition, the name as the card's link, then its host, which gives way first; at the end, an online node's core state and version, or a state indicator only for a node away or not enrolled. Then two groups 24px apart: download, upload and connections, each a label over its icon and number, centred in its third of the row, icons in their series colours and rate numbers at 20px right-aligned in a three-character slot with units set back, 16px below 21rem inside; then CPU, memory, disk and what is left of the quota two by two, each a label and value over twenty cells spanning its column | Online; offline, with when it was last seen over its core, centred in the live row's place and height, above the same capacity grid with load unknown; not enrolled, with a dashed hairline, the added time in place of the host and one action showing its enrollment code | At least 22rem wide, on the dense surface. The dot follows the overview's attention rules and the card's load thresholds, its reasons on hover; details open on hover: core uptime and platform, cores and load, used and total memory and disk, the period's usage in each direction and the reset date; under touch the card stays one link. Hover strengthens the hairline; keyboard focus rings the card inside; load meters stay muted at rest, turn warning from 80% and danger from 95%; the remaining meter is success while more than 10% is left, warning from there and empty once reached, which says so in place of the amount; an unlimited quota keeps a full success meter; an offline node or a reached quota carries a danger hairline |
| Key figure | Label with an optional help trigger, value in the data face at 20px, then one meta line | — | Two a row below 48rem with the value at 16px, where rates and usage take a full row; four from 48rem; a value wraps rather than clips |
| Overview region | Card with a 44px header row: title, a status such as the update time, and a link to the region's full view; then its content | Table, list, counts | From 48rem the nodes table takes the height the lower row leaves, at least 240px, and the lower regions share one 288px row; below 48rem each takes its content's height up to 288px, activity 384px and the table 480px; all scroll inside, the table under its sticky header |
| Cell meter | Ten cells of the data face's grid, filled to the nearest cell in the text colour; none below half a percent | Success, warning, danger; a set number of equal cells spanning its container | Its number beside it carries the value; it leaves narrow containers first |
| Enrollment code | Command and code, each with a copy action, then a one-line warning note with the expiry | — | Shown once; closing discards it |
| Terminal window | Card on the surface and hairline, data face at 13px on 20px lines: a 40px title bar with three window marks, `console@host:~` centred in the body face and the link state's dot and hh:mm:ss duration at the end, then the buffer | — | 3:2, at most 64rem wide and no taller than the space below the first row; phones keep 32rem or less in height; the duration counts each second; stale stops it, marks the title bar in warning text and dims the newest status output to 60% |
| Status readout | A label column in muted text and value rows; parts of a value split by faint dots; shares as cell meters | Identifiers end in a 20px copy action whose target reaches 40px; meters leave phones | The newest output follows the node, earlier ones keep what they showed; a known zero reads 0 in the smallest unit, so rates read 0 B/s while no core runs, and "—" marks only what cannot be known; an address not yet detected is left out; a value wraps after a dot; meters turn warning from 80% and danger from 95% |
| Terminal buffer | Echoes after their prompt, then output in a tone: muted, plain, success, warning or danger; event lines lead with their time, details sit 2ch in; the input line follows the last line | A pending line ends in a spinner and the input line waits for it | Scrolls as a whole, following new output unless scrolled back; output types itself out within 360ms, echoes appear at once; selection is a soft band that keeps each text's colour |
| Input line | Prompt, then a transparent input with a caret in the terminal text and `help` as its placeholder; while open, the completion menu under it lists one command a line with its description, the chosen one in reverse | Shell `console@host in ~ λ` with user and host, path and symbol in their colours; code `code>` in the path colour; confirm `[y/N]` in danger | Up and Down recall; Tab completes a single match and otherwise opens the menu, which Tab, Shift+Tab and the arrows cycle while the line shows the choice; Enter or Space accepts it without running it, Escape restores what was typed, typing or leaving closes it; Tab never leaves the line, Shift+Tab does while no menu is open; Escape or Ctrl+C cancels a prompt, and an empty code line backs out |
| Dialog | Title, close, scrolling body, footer with actions at the end | Standard 30rem; wide with a section rail on the left | Cancel first, auxiliary next, primary last; Escape closes the innermost layer |
| Notification | Compact toast with a status colour | Success, warning, danger, info | Dismisses after 3 seconds, pausing on hover and focus; a repeated failure reuses one toast |
| Loading region | Accent spinner over a muted label | — | Short delayed fade-in inside the pending region only |
| Help | Information icon beside a label, tooltip; an action's own tooltip says what it does | — | Opens after 200ms on hover, at once on click or tap; outside the Tab order. Explanations live only here: empty states show a title and the next step, fields carry no helper text |
| Floating tool cluster | Embedded icon actions on a surface pinned to a corner | — | Hover tint; never covers the text it serves |

## 7. Layout

- The shell has two regions: a floating navigation rail and the workspace. There is no global header; each view's first row is its toolbar row or its object row.
- The rail lists panel-scope destinations only: overview, nodes, subscriptions, logs and settings. Theme, language and sign-out sit at its foot as equal embedded actions. Signing in opens the overview.
- The overview shows the fleet's key figures and links each region to the view that manages it: online nodes, running cores, live traffic with connections, and this period's usage summed but never a summed quota; then a nodes table with state, core, CPU, memory, traffic and period usage against each node's own quota; then needs attention (offline, quota reached, core down or unknown, quota, disk or memory at 90%, not enrolled), recent panel events and subscription counts. With no nodes it leads with adding one; it refreshes every 5 seconds while visible and marks a failed refresh as no longer updating.
- Nodes are first-class objects. The nodes view is a grid of node cards that scrolls in its own region under a fixed toolbar row and refreshes every 5 seconds while visible. The row filters by state, each filter with its count (all, online, offline, not enrolled), then carries the search and adding a node; phones put the search and adding beside the navigation and give the filter a full row. Nodes that need attention lead, offline, then over quota, then not enrolled, and the rest follow by name; live load never reorders the grid. Each card carries the node's health, identity and core, live rates and connections, host load and what is left of its own quota this period. Opening a node shows its object row, then tabs for overview, core versions, configuration, logs and settings.
- A node's settings tab edits its name, published host and traffic quota and period in one saved group, and keeps re-enrollment and removal in a separate destructive row below it; the group fills the height above that row.
- Runtime controls belong to the node they act on and sit in that node's object row.
- A node that is offline or not enrolled keeps what it last reported in its object row, marked stale with when it was last seen, and every tab that commands the node shows one region state in place of its view; its settings tab stays available, and a node not yet enrolled offers its enrollment code there.
- A panel-scope view never shows one node's runtime state as global state.
- The panel's glyph field fills the canvas behind the rail and workspace in calm mode at 30% of its sign-in opacity, mixing in the fleet's names, hosts, addresses, versions and rates; it shows faintly through cards and the rail.
- The node console has no rail. Its first row spans the window at the shell inset, with the wordmark at the start and theme, language and sign-out as equal embedded actions at the end. Below it one surface sits at the window's centre: signed in, a terminal window in 3:2 that follows the theme; the unavailable states use a card at 24rem, and sign-in is the showcase prompt in section 10.
- The terminal opens on the output of `status`: link, panel, node ID, trust, core, traffic rates with connections, totals, CPU, memory, disk, host and agent; the node's state decides which rows hold values, never which rows exist. Link changes, each new connection error with its next attempt, core changes and staleness print below it once each.
- Commands are typed; only copy actions are clickable. `help` lists the commands of the node's state and `status` prints an output that takes over following the node. Enrolling asks for the code at `code>`, after re-enrolling warns that it replaces the enrollment; unenrolling prints the panel it leaves in danger and asks `[y/N]`, where only y proceeds. Clicking the window's free space focuses the input unless text is selected.
- The sign-in field stays behind the terminal, calm: 60% of its sign-in opacity, still under the pointer, mixing the node's own values into its text, rippling once from the window on each status refresh, and moving or fading the glyphs the window covers.

## 8. Rules

Do and don't:

- Derive every accent-tinted value from the saved accent. A hard-coded tint breaks as soon as another colour is saved.
- Keep blur out, and translucency to cards and the rail at 72%, node cards at 90%. Blur repaints under every live row, and glyphs behind typed text, menus, scrolled rows or dense live figures read as content or flicker.
- Keep gradients out of working views. The hatched unknown state is a data encoding, not decoration.
- Colour a state only with a state colour. A state in the accent disappears under a palette of the same hue.
- Use compact actions only in toolbars, table rows and pagination, and embedded actions only inside a field, card or tool cluster. Mixed heights break row alignment.
- Style new work with component variants and utilities on the instance, and read values only from tokens. Page-level styles and literal values are how the views drifted apart.
- Use only the type steps and weights above. Off-scale text breaks the hierarchy between data and labels.
- Keep the node's name and state visible in every node-scope view. An action on the wrong node is the most expensive mistake in a fleet.

Overrides of the global guide:

| Global rule | Project value | Reason |
| --- | --- | --- |
| Control heights from the set 36, 40, 44 | 40, 32, 44, and 28 for embedded actions | Compact actions fit table rows and toolbars without enlarging them; touch keeps 44 |
| Adjust instances with utilities, not page-level CSS | The schema form theme and the colour picker's own parts keep one stylesheet beside their component; the node console keeps one for its sign-in morph and one for its terminal's keyframes | Their rules follow markup that rjsf nests recursively or that a library renders, or define keyframes and view transitions, which utilities on one instance cannot address |
| Working screens have no gradients, glows or textures | Every signed-in view keeps the sign-in glyph field behind it in calm mode | Sign-in and the working views read as one product; the field is faint, carries the product's own values, stays still under the pointer and shows only faintly through translucent cards |

## 9. Tokens in code

- `web/packages/shared/src/styles/tokens.css` holds every token and nothing else: the root scope, the dark scope `:root.dark, :root[data-resolved-theme='dark']`, the more-contrast and coarse-pointer overrides, and the `@theme inline` block that exposes tokens to Tailwind. `base.css` holds element defaults and `components.css` shared component classes; `global.css` imports Tailwind and the three files in that order.
- Component roles use the shadcn names (`--background`, `--foreground`, `--card`, `--popover`, `--primary`, `--muted`, `--accent`, `--border`, `--ring`, `--chart-*`, `--sidebar-*`), read through utilities such as `bg-card`, `text-muted-foreground` and `border-border`. Product tokens use the `--color-*`, `--space-*`, `--radius-*`, `--control-height*`, `--shadow-*`, `--dur-*`, `--ease-*` and `--z-*` prefixes and are read with `var()`. A product token never reuses a component role name with another meaning, and no token is an alias of another: code reads the names in sections 2 to 5.
- Utilities follow the scales: `rounded-md` is R/2, `rounded-lg` and `rounded-xl` are R, `rounded-[var(--radius-window)]` is the floating step; sizes are `text-2xs`, `text-xs`, `text-compact`, `text-sm`, `text-base`, `text-xl` and `text-2xl`; weights are `font-normal`, `font-medium` and `font-semibold`.
- `web/apps/panel/src/theme/appearance.ts` derives the tokens in section 2's derivation table and the radius tokens from the saved `{ theme, color, radius }`; the defaults are `system`, `#6D4ED1` and 12, and the presets `#6D4ED1`, `#2563EB`, `#0891B2`, `#15803D`, `#C65B13` and `#BE185D`. `ThemeProvider` writes them on the document root, from the appearance meta the server fills; before scripts load, the server paints the saved theme's `--color-canvas`.
- Shared patterns live in `web/packages/shared/src/components`: the glyph field, the prompt input and the cell meter; their classes sit in `components.css`.
- The component library is shadcn `base-nova` on Base UI with lucide icons (`web/packages/shared/components.json`); patterns map to `components/ui` primitives and the shared components beside them. Tokens the shadcn CLI adds go into `tokens.css`.
- Code that paints, such as charts, reads tokens with `getComputedStyle(document.documentElement)` and redraws when the theme or appearance changes.
- To add a token: give it light and dark values in `tokens.css`, derive it in `appearance.ts` when it depends on the accent, register it in `@theme inline` when components need a utility, and add its row here, in one change.

## 10. Showcase

The two sign-ins are the only showcase surfaces and share one terminal prompt. They have no card or visible heading: below a first row with the wordmark, theme and language, the prompt sits at the window's centre in the data face. The node console asks for its password after a `$` in the accent; the panel asks `login:` for the email and `password:` below it, its labels in the accent, Enter on the email moving to the password. Each line draws its characters, or for a password a dot in a wider cell popping in, with a blinking block cursor at the end, a bar between characters and the selection as an accent band over whole cells; the password line ends in the visibility toggle and an embedded submit. A placeholder, in weight 300 at reduced contrast, shows only while its line is empty and unfocused. A refused attempt shakes the prompt once with a danger border and prints its message as the prompt's output row: attached below on the same surface and border, a danger cross and danger text, sliding down from the prompt outside the flow and closing when typing resumes. A sign-in turns the prompt success-green and holds 520ms; the node's prompt then morphs into its terminal window, the panel's cross-fades into the shell. While checking the session each shows a spinner, and an unreachable service shows a card at 24rem with a retry.

Behind both a canvas of protocol names, settings, documentation addresses and log fragments in the data face fills the window below the first row in three layers at 10, 12 and 14px and rising opacity, longer fragments further back; each layer is scattered by best-candidate sampling, tilted mostly within 12°, whole at the window's edges and kept a narrow margin from the prompt, so layers overlap without leaving holes. The pointer pushes and leans glyphs aside, nearer layers further, a keystroke streams a few toward the prompt, a refusal sends a danger shock outward and a sign-in a success wave across; while idle one glyph at a time types a new text with a cursor at its end. The field pauses while the page is hidden and is a still field that only changes colour under reduced motion.
