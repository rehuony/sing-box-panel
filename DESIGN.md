# sing-box-panel design system

The design system of the sing-box-panel web interface: roles, scales, formulas, patterns and rules. It extends the global design guide and repeats nothing from it. Every value is the one the tokens in code use; a change to one changes the other in the same work.

The panel interface serves two scopes. The panel scope covers the fleet: the nodes home, subscriptions, the panel log and panel settings. The node scope covers one node: its overview, core versions, configuration, logs and settings. The node console is a separate interface that one node serves on its own host for its enrollment with a panel and a read-only view of its state; it uses the same tokens, type and components, and the panel's command line shares its terminal parts.

## 1. Character

| Property | Declaration |
| --- | --- |
| Mood | Calm and technical; the interface reports state and stays out of the way |
| Colour temperature | The terminal's cool neutral greys, the same under every accent |
| Density | Dense working screens; data at 12–14px, one title strip per view |
| Type | One bundled variable sans at three weights; one bundled monospace for machine text |
| Surface material | Content surfaces (cards, tables, forms, panes, title strips, key figures) at 90% opacity over a canvas that carries a faint glyph field, separated by hairlines; the status line, fields, menus, dialogs and toasts stay opaque, and floating layers detach from the window edge and cast a shadow |
| Motion | Immediate response to input, short state transitions; the glyph field's slow rewriting is the only idle motion |

Properties that make the product recognisable:

- The neutral scale is the terminal's greys and the saved accent touches only actions, selection and focus, so every palette keeps the same calm surfaces and accessible text; status colours never take the accent, so a state reads the same under every palette and in both themes.
- A field of protocol glyphs sits behind every view, interactive at sign-in and calm once signed in, where it mixes in the product's own values; both services read as one product.
- One radius parameter derives three corner steps, so a single setting reshapes every element in proportion.
- One status line below the workspace lists the scope's numbered windows, leads back to the fleet and reads typed commands, so the shell works like a terminal multiplexer: the panel's command line and the node console share one prompt, buffer and completion grammar, and nodes appear as terminal panes whose labelled readings follow the console's status output.
- Every view declares its scope: the status line lists the scope's windows and leads back to the fleet, in a node's windows also to the neighbouring nodes, and a node window's title strip carries the node's identity and state.

## 2. Colour

Light values sit in the root scope, dark values in the dark scope. Neutrals are fixed. The accent roles are recomputed from the saved accent A and the card surface S (light `#f8f8fa`, dark `#25262f`) before first paint; the table shows the result for the default A `#6D4ED1`.

| Role | Token | Light | Dark | Use |
| --- | --- | --- | --- | --- |
| Canvas | `--color-canvas` | `#eeeef2` | `#1c1d24` | Window background behind the workspace and status line; carries the glyph field |
| Surface | `--card`, `--color-surface` | `#f8f8fa` | `#25262f` | Sticky table headers, outline actions, the status line, the terminal window |
| Surface translucent | `--color-surface-translucent` | Surface at 90% | Same | Every content surface: cards, tables, forms, panes, title strips, key figures; Surface under more contrast |
| Field | `--color-field` | Surface | Surface with 30% of Border | Text fields and select triggers, opaque so no glyph sits behind typed text |
| Popover | `--popover` | `#fcfcfd` | `#2b2c36` | Menus, select lists, popovers, dialogs, toasts, the command output |
| Surface raised | `--color-surface-raised` | `#f0f0f4` | `#2d2e38` | Nested blocks, table footers, disabled fields |
| Muted fill | `--muted` | `#f0f0f4` | `#2b2c36` | Secondary actions, segmented tracks |
| Border | `--border`, `--input` | `#e2e2e9` | `#34353f` | Hairlines, dividers, input borders |
| Strong border | `--color-border-strong` | `#cfcfd9` | `#464753` | Hovered inputs |
| Text | `--foreground` | `oklch(24% 0.015 280)` | `#f8f8f2` | Primary text and icons, filled meter cells |
| Secondary text | `--color-text-secondary` | `oklch(40% 0.015 280)` | `oklch(84% 0.012 280)` | Field labels, secondary cells |
| Muted text | `--muted-foreground` | `oklch(50% 0.015 280)` | `oklch(72% 0.02 280)` | Meta, table headers, timestamps, terminal labels |
| Faint text | `--color-text-faint` | `oklch(54% 0.012 280)` | `oklch(62% 0.018 280)` | Placeholders, disabled text, the glyph field |
| Selection | `--color-selection` | `oklch(24% 0.015 280 / 14%)` | `oklch(97% 0.01 100 / 20%)` | Text selection, which keeps each text's colour; scrollbar thumbs |
| Meter track | `--color-meter-track` | Muted text at 30% over Surface, opaque | Same | A cell meter's empty cells, which cover its scale |
| Hover | `--color-hover` | Text at 6% over what lies below | Text at 8% | Every hovered or highlighted control, menu item, list option and row, and an open trigger, on any surface |
| Accent | `--primary`, `--ring` | `#6d4ed1` | `#a08ce1` | The primary action's text and hairline, keyboard focus, links, prompt symbols and cursor, a switch that is on, a slider's range |
| On accent | `--primary-foreground` | `#ffffff` | `#17151e` | Text and icons on an accent fill |
| Accent soft | `--accent` | `#ebe9f6` | `#312c49` | The primary action's fill, the current window and tab, selected panes, rows and choice cards, pressed toggles, a filter in effect |
| Success | `--color-success`, `-soft`, `-text`, `-border` | `oklch(59% 0.14 165)`, `oklch(95% 0.03 165)`, `oklch(42% 0.11 165)`, `oklch(82% 0.06 165)` | `oklch(72% 0.14 165)`, `oklch(27% 0.055 165)`, `oklch(83% 0.105 165)`, `oklch(46% 0.08 165)` | Healthy and completed states |
| Warning | `--color-warning`, `-soft`, `-text`, `-border` | `oklch(65% 0.16 62)`, `oklch(96% 0.035 72)`, `oklch(46% 0.12 62)`, `oklch(83% 0.07 62)` | `oklch(78% 0.14 72)`, `oklch(28% 0.055 72)`, `oklch(84% 0.11 72)`, `oklch(48% 0.08 72)` | Degraded or stale states, pending action, one-time secrets |
| Danger | `--color-danger`, `-soft`, `-text`, `-border` | `oklch(56% 0.18 20)`, `oklch(95% 0.035 20)`, `oklch(43% 0.14 20)`, `oklch(82% 0.07 20)` | `oklch(70% 0.17 20)`, `oklch(28% 0.06 20)`, `oklch(83% 0.105 20)`, `oklch(48% 0.08 20)` | Failed or unreachable states, errors, destructive actions |
| Info | `--color-info`, `-soft`, `-text`, `-border` | `oklch(58% 0.15 248)`, `oklch(95% 0.03 248)`, `oklch(44% 0.12 248)`, `oklch(82% 0.06 248)` | `oklch(72% 0.13 248)`, `oklch(28% 0.055 248)`, `oklch(82% 0.095 248)`, `oklch(47% 0.075 248)` | Neutral notices |
| Highlights | `--color-highlight-cyan`, `-magenta`, `-yellow` | `#0b7189`, `#b4237a`, `#8a6a00` | `#8be9fd`, `#f079bd`, `#f1fa8c` | Only chart series 1 to 3 (download, upload, a third) and the command line's prompt (path, user, symbol); readings' arrows stay in secondary text |
| Scrim | `--color-scrim` | `oklch(24% 0.015 280 / 34%)` | `oklch(7% 0.01 280 / 68%)` | Behind modal layers |
| Syntax | `--color-syntax-property`, `-string`, `-number`, `-keyword` | `#245fa1`, `#21774b`, `#9a541e`, `#8c4989` | `#88bdf3`, `#9ac99d`, `#e7b685`, `#d5a0d7` | JSON syntax in the configuration editor |
| Brand | `--color-brand-navy`, `--color-brand-orange` | `oklch(20.8% 0.042 265.755)`, `oklch(70.5% 0.18 47.6)` | `oklch(17% 0.03 265.755)`, `oklch(76% 0.16 55)` | Logo mark |
| Window marks | `--color-window-close`, `-minimize`, `-zoom` | `#ff5f57`, `#febc2e`, `#28c840` | Same | Only the three marks of the node console's terminal window |

Derivation, with mix(A, S, w) as A mixed into S at weight w:

| Token | Light | Dark |
| --- | --- | --- |
| `--primary` | A moved toward black in 1% steps until it reaches 4.6:1 against `--accent` and S | A moved toward white the same way |
| `--primary-foreground` | `#ffffff` when it reaches 4.5:1 against `--primary`, otherwise `#17151e` | Same rule |
| `--accent` | A at 9% | A at 16% |

Accent policy: at rest a view shows only neutrals and the one accent, besides the cell meter's scale. The one primary action of an area takes the accent's tint: its soft fill, a 50% accent hairline and accent text, the tint of a selected pane, as a destructive action takes the danger tint; on hover the accent at 15% deepens it. The accent marks selection, the current window, a current tab, a pressed toggle and a filter in effect through its soft fill, rings keyboard focus, colours links and the prompt's cursor. It never tints a neutral, marks a state, a badge, the logo, a data series or a destructive action. Interaction states read the same in every component: hover and a highlighted option take the hover wash; selected, current, pressed and on take the accent's soft fill; a checked option shows its checkmark; text keeps the text colour in all of them; only the primary action, links and the prompt write in the accent. State colours mark only the exception (a dot, a word), never a border or a filled region; the cell meter's scale is the one place they blend. Data series take the highlights: cyan for download and the first series, magenta for upload and the second, yellow for the third.

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
| Form gaps | `--form-field-gap`, `--form-section-gap` | 12px between fields, 20px between sections |
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
| Floating layers: dialogs, alert dialogs | `--radius-window` | min(32, 7R/6) | 14px |
| Badges, status pills, avatars | `--radius-pill` | 999px | 999px |

R is the saved radius, an integer from 0 to 32, default 12.

| Shell parameter | Token | Value |
| --- | --- | --- |
| Window inset of the workspace and status line | `--shell-edge` | 16px; the workspace's own inset on phones |
| Status line | `--shell-bar-height` | 40px, 12px below the workspace; 56px as a tab bar along the foot below 640px and on coarse pointers |
| Title strip | `--shell-strip-height` | 48px at least, with a 16px side inset, the cards' own, so its first and last items line up with the content below |
| Pane | `--pane-min-width`, `--pane-height` | 20rem at least, 184px tall: a 32px title bar, a 28px headline, a 20px core line and four 20px meter rows |
| Table rows | `--workspace-row-height` | Header 44px without a divider; data rows 64px plus a 1px divider, growing when content wraps |
| Page sizes | — | 5, 10, 50 |
| Breakpoints | — | 640 (the status line becomes a tab bar), 768, 1024px (context values take their own line); views also respond to their own width |

## 5. Surfaces and motion

| Level | Surface | Separation | Elements |
| --- | --- | --- | --- |
| Canvas | `--color-canvas` with the glyph field | — | Window background |
| Content | `--color-surface-translucent` | `--border` hairline; `--shadow-card` only where a hairline is not enough | Cards, tables, forms, panes, title strips, key figures |
| Raised | `--color-surface-raised` | Tint | Nested blocks, code, configuration and log output |
| Popover | `--popover` | Hairline and the popover shadow | Menus, select lists, popovers; tooltips invert to the text colour with surface text |
| Floating | `--popover`; the status line `--card` | Hairline and `--shadow-panel` | Status line, command output, dialogs, alert dialogs, toasts |

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
| Action | Label, optional 16px leading icon | Primary (accent soft fill, 50% accent hairline, accent text), outline, secondary, ghost, link, destructive (danger soft fill, 25% danger border), flat (text colour, no fill or border at rest, the hover wash, the accent's soft fill while pressed); standard, compact (13px text), large, embedded; each with a square icon form | Hover, pressed, keyboard focus, pending keeps the label and width |
| State indicator | Dot or 20px pill with 12px medium text | Success, warning, danger, info, neutral | Static; a change of state replaces the colour without moving the row |
| Single choice | Trigger with value and chevron; list with a checkmark on the current value | Standard, compact; flat in a bar (compact, no border or fill, secondary text, the hover wash) | A mouse-opened list keeps options neutral; keyboard highlight follows focus; a flat filter set to anything but its default takes the accent's soft fill and the text colour |
| Field row | Label column, control filling the rest, optional help trigger beside the label | Single column; stacked below 520px; a tag input holds tags as data-face chips with a remove action before its input | Invalid marks the control; messages go to the action bar or a toast; a tag input adds what was typed in lower case on Enter, a comma or a space, removes the last on Backspace in an empty input, keeps at most 16 sorted and distinct, and says a refused tag in the form's message slot |
| Data table | 44px header kept in view, 64px rows, row actions at the end; cells inset 16px | Paged or scrolled within its region, edge to edge on its card so the header's fill and the row dividers reach its edges; on a shared card a grid of cards sits 16px in, a form or editor 16px and 24px from 640px | Loading, empty and error centred in the region; selection by soft fill |
| Pagination | Page-size choice, previous and next, page input, total | Footer below its list over a hairline across the list's width, the page-size text aligned with the first column's | Looks and works the same while its list loads or is empty; previous and next rest only at the first and last page; Enter or blur commits a page, Escape cancels |
| Status line | Command action (the prompt symbol); the scope segment: `fleet`, the link to the nodes home, in panel windows, and in a node's windows the previous node, the nodes home and the next node as embedded icon actions (chevron, house, chevron), the neighbours in the fleet's name order, wrapping around, keeping the window and absent with one node; the scope's windows as numbered tabs; then an update hint, the fleet readout (`● online/total online`) and theme, language and sign-out as embedded actions | Desktop line at the shell inset; phone tab bar along the foot: the nodes home as a house in a node, windows as icons with the current one named, the command action and a more menu with theme, language and sign-out | The current window takes the accent's soft fill and its number stays faint; the readout dot is success with every node online, warning with some away and muted with none, and becomes a warning ring while the fleet stream is down, its reason in the tooltip and the accessible name; with panes selected the selection (count, select all, clear, batch actions) takes the scope's and windows' place; command mode puts the input line in place of everything after the command action |
| Command output | One floating panel above the status line on the popover surface: an output header with a close action, the terminal buffer, and the completion menu below it | Desktop panel at most 40% of the window's height; phone sheet at most half of it, attached to the line | Shows while there is output or a menu; scrolls inside; output stays until closed or cleared, across opening and closing the line, and ends with the signed-in session; it keeps room for every node's line of a batch; one batch runs at a time, keeps running when the line closes and then reports its summary in a toast, or a danger toast when it stopped before one |
| Title strip | The view's toolbar in the status line's grammar: flat compact controls grouped by its dividers (1px by 20px), tabs and filters at the start, a flat search filling the room between dividers, actions at the end; tabs have no track, the current one on the accent's soft fill, counts in faint data-face numerals | Panel views: the toolbar; node windows: the node's name at the xl step in 600, its state indicator, then labelled readings (online: `core` state · version · uptime, `net` ↓ · ↑ · connections; offline: when it was last seen in warning, then `core` and the version it last reported) and flat runtime actions with icon and label | On the content surface with a hairline, 48px; a search is raised while focused and ringed when the keyboard brought focus, with a clear action while it holds text; actions keep only their icon on a phone; wraps when narrow; below 1024px a node's readings take a second line that scrolls sideways. A node's readings open with what the fleet last heard until the node's stream answers; a part the node has not given is left out and a reading with none left goes with its divider, while a known zero reads 0, so a stopped core reads 0 B/s and 0 connections; traffic keeps its newest sample until the next replaces it and turns muted while the stream reconnects; each rate sits in a 9ch slot aligned to its end |
| Pane | A 32px title bar over a hairline: health dot, name as the pane's link in 14px medium, host in muted 12px that gives way first, and at the end what needs saying; then in the data face the headline, which says what the node is doing now, the core's line, and `cpu`, `mem`, `disk` and `quota` as labelled meters | Online: the headline's ↓ and ↑ rates at 20px semibold with their units in muted 12px and connections at the end, the line `state · version · uptime`. Offline: the same skeleton, the headline `offline` with how long in danger and when it was last seen at the end in muted text (month and day with the time, the year only when it differs), the line the version it last ran and the period's ↓ · ↑ traffic in secondary text, the host meters hatched | 184px tall at least 20rem wide on the content surface; one line a row, uptime in its largest unit; core state running in the text colour, stopped in secondary text, unknown in warning; arrows, a known zero rate and a meter's value at rest in secondary text. The title's end says, worst first, `quota reached` in danger or `↑` and the panel's release in info text when an update is available. The dot follows the attention rules and the meters' thresholds, with its reasons on hover. The hairline stays neutral and strengthens on hover; keyboard focus rings the pane inside; a selected pane takes the accent's soft fill with a 50% accent hairline and a 25% accent divider, its texts unchanged, and its link says it is selected |
| Pool node | A pane's title bar: the display name in 14px medium and its origin in muted 12px, `node · inbound` or the source's name, giving way first; then in the data face the protocol in secondary text with `server:port`, the security line (`tls`, `reality`, the SNI) and the transport | — | 116px tall in the panes' grid, in the pool's saved order and unpaged; the subscriptions window opens on the pool, before sources, channels and keys. The whole card drags with the mouse, touch and keyboard sensors, follows the pointer 1:1 and lifts with the panel shadow while its slot stays; a click opens its detail, where only the display name edits; hover strengthens the hairline and keyboard focus rings it inside |
| Key figure | Label with an optional help trigger, value in the data face at 20px with an optional cell meter beside it, then one meta line | Neutral; warning or danger, colouring the value and its line | Four in one card on the content surface divided by hairlines, two a row below 48rem of its own width with the value at 16px; a value wraps rather than clips; the nodes home and a node's overview share it |
| Cell meter | Ten cells of the data face's grid, filled to the nearest cell; none below half a percent; filled cells reveal one scale laid along the whole meter, success at empty blending in OKLCH to warning at its warning limit and danger at its danger limit, and empty cells in the meter track cover the rest | Scale, unknown; a set number of equal cells stretching across the room its row leaves, which a pane sets as 20 cells between faint brackets with the value in a 4ch column before the closing one | Its number beside it carries the value and takes warning or danger text past the limits; load's limits are 80% and 95%, a quota's 90% used and reached, when it shows full; an unlimited quota never fills and says so in secondary text; an unknown share hatches every cell beside "—" |
| Add-node dialog | Standard dialog in three states. Naming: one field row, `Name` and its input. Waiting: a raised block with a hairline, headed `Run on the node host` with flat copy actions for the command and the code alone, then the command in the data face after an accent `$`, wrapping and selectable; the footer's slot holds a spinner, `Waiting for <name>` and the code's countdown in secondary tabular numerals. Enrolled: the same block with the node's dot and name, `Host` and `System` readings where known | Adding (from naming); enrolling an enrolled node again (from waiting, titled for it) | Waiting and enrolled keep one height. Close, Escape and Cancel discard the unused code while waiting; the scrim does not close it; leaving the page discards it with a keepalive request; an expired code says so in warning and offers Regenerate. The node counts as enrolled once the fleet status lists it, read from the stream or polled while the stream is down; then Done and Open node close without discarding |
| Difference view | A read-only editor: the next text with removed lines shown where they were | — | Removed lines on danger soft, added ones on success soft; fills the dialog's body and scrolls inside |
| Terminal window | Card on the surface and hairline, data face at 13px on 20px lines: a 40px title bar with three window marks, `console@host:~` centred in the body face and the link state's dot and hh:mm:ss duration at the end, then the buffer | — | 3:2, at most 64rem wide and no taller than the space below the first row; phones keep 32rem or less in height; the duration counts each second; stale stops it, marks the title bar in warning text and dims the newest status output to 60% |
| Status readout | A label column in muted text and value rows; parts of a value split by faint dots; shares as cell meters | Identifiers end in a 20px copy action whose target reaches 40px; meters leave phones | The newest output follows the node, earlier ones keep what they showed; a known zero reads 0 in the smallest unit, so rates read 0 B/s while no core runs, and "—" marks only what cannot be known; an address not yet detected is left out; a value wraps after a dot |
| Terminal buffer | Echoes after the prompt they were typed at, then output in a tone: muted, plain, success, warning or danger; event lines lead with their time, details sit 2ch in, and a line led by a term keeps its text in a column beside it | A pending line ends in a spinner | Scrolls as a whole, following new output unless scrolled back; output types itself out within 360ms, echoes appear at once; selection is a soft band that keeps each text's colour |
| Input line | Prompt, then a transparent input with a caret in the terminal text and `help` as its placeholder; while open, the completion menu lists one value a line with its description, the chosen one in reverse | Shell `user@host in path λ` with user and host, path and symbol in their colours (`console@host in ~`, `panel@fleet` or `panel@<node>` in the current window); code `code>` in the path colour; confirm `[y/N]` in danger; the menu under the line, or in a panel above it | Up and Down recall; Tab completes the word under the caret, a single match at once and otherwise through the menu, which Tab, Shift+Tab and the arrows cycle while the line shows the choice; Enter or Space accepts it without running it, Escape restores what was typed, typing or leaving closes it; Tab never leaves the line, Shift+Tab does while no menu is open; Escape or Ctrl+C cancels a prompt, and an empty code line backs out; where the line cannot wrap the prompt gives way first |
| Dialog | Title, close, scrolling body, footer with actions at the end | Standard 30rem; wide with a section rail on the left | Cancel first, auxiliary next, primary last; Escape closes the innermost layer |
| Notification | Compact toast with a status colour | Success, warning, danger, info | Dismisses after 3 seconds, pausing on hover and focus; a repeated failure reuses one toast |
| Loading region | Accent spinner over a muted label | — | Short delayed fade-in inside the pending region only |
| Help | Information icon beside a label, tooltip; an action's own tooltip says what it does | — | Opens after 200ms on hover, at once on click or tap; outside the Tab order. Explanations live only here: empty states show a title and the next step, fields carry no helper text |
| Floating tool cluster | Embedded icon actions on a surface pinned to a corner | — | Hover tint; never covers the text it serves |

## 7. Layout

- The shell has two regions: the workspace, filling the window at the shell inset, and the status line below it. There is no global header and no navigation rail; each view's first row is its title strip, and the status line leads back to the fleet and lists the scope's windows.
- Panel-scope windows are `1 nodes`, `2 subscriptions`, `3 logs` and `4 settings`; node windows are `1 overview`, `2 cores`, `3 configuration`, `4 logs` and `5 settings`. Their names are the command line's words and read the same in every language; their tooltips and accessible names are translated. Switching to another node keeps the window. `/` and `/overview` open the nodes home, and so does signing in.
- The command line opens with Ctrl or Cmd+K anywhere, `:` outside fields, editors, menus and dialogs, or the command action, but not under a modal layer. It takes `help`, `clear`, `logout` and `exit`, a window's number, `open <window>`, `goto <node> [window]`, `filter [text | state:<state> | tag:<tag>]` and `start`, `stop`, `restart` and `update` with a target: a node's name, a glob such as `hk-*`, `tag:<tag>` or `all`, resolved in the browser. Commands that only open a window, a node or the filtered home close the line and print nothing; anything else unknown prints `command not found` in danger. Tab completes verbs, windows, node names, tags and targets.
- A batch that interrupts, and any batch on more than one node, first prints its count, up to five names and what it interrupts, and asks `[y/N]`; only y proceeds. Each node's result prints as it finishes, one line a node, then the summary; nodes that cannot answer fail on their own line, and `update` runs one node at a time and stops at the first failure. Leaving the line never cancels a batch.
- The nodes home has the title strip (state tabs all, online and offline with counts, a tag filter when any node has tags, search, a selection toggle, adding a node; below 48rem the tabs keep the first line), the key figures (running cores, live traffic with both directions summed, active connections with the online nodes, this period's usage summed but never a summed quota; on a phone they scroll with the panes) and the panes in a grid that scrolls in its own region. Offline nodes lead, then nodes over quota, and the rest follow by name; live load never reorders them. With no nodes the screen leads with the primary action that adds one. A click opens a node; selection mode, entered from the strip's toggle, Ctrl or Cmd click, Shift click, a touch held for 500ms or Space on a focused pane, makes a click toggle the pane and Shift extend from the last one toggled, while Enter, a middle click and the context menu still open the node; Escape, clearing the selection or the toggle leaves it and clears it. The selection's batch actions (start, stop, restart, install a core at an exact catalog version, update) each confirm in a dialog that names the nodes and what the action interrupts, then report in the command output.
- A node window's title strip keeps the node's identity, state, readings and runtime controls; the status line leads to the fleet and the neighbouring nodes; an offline node keeps what it last reported there, led by when it was last seen, and every window that commands the node shows one region state in place of its view, while its settings stay available. A panel-scope view never shows one node's runtime state as global state. A node's settings edit its name, published host, tags and traffic quota and period in one saved group; below it a version row shows the node's version against the panel's release with its update action, and a separate destructive row keeps re-enrollment and removal.
- The configuration window can copy another node's saved configuration into its draft after showing the difference. It also lists the node's applied configurations, newest first, twenty at a time, with the running one marked; Roll back runs one again after a danger confirmation that names it and says the core restarts and the saved text stays, so the next start loads the saved text again.
- The panel's glyph field fills the canvas behind the workspace in calm mode at 30% of its sign-in opacity, mixing in the fleet's names, hosts, addresses, versions and rates; it shows faintly through cards.
- The node console has no status line. Its first row spans the window at the shell inset, with the wordmark at the start and theme, language and sign-out as equal embedded actions at the end. Below it one surface sits at the window's centre: signed in, a terminal window in 3:2 that follows the theme; the unavailable states use a card at 24rem, and sign-in is the showcase prompt in section 10. The terminal opens on the output of `status`: link, panel, node ID, trust, core, traffic rates with connections, totals, CPU, memory, disk, host and agent; the node's state decides which rows hold values, never which rows exist. Link changes, each new connection error with its next attempt, core changes and staleness print below it once each.
- Console commands are typed; only copy actions are clickable. `help` lists the commands of the node's state and `status` prints an output that takes over following the node. Enrolling asks for the code at `code>`, after re-enrolling warns that it replaces the enrollment; unenrolling prints the panel it leaves in danger and asks `[y/N]`. Clicking the window's free space focuses the input unless text is selected. The sign-in field stays behind the terminal, calm: 60% of its sign-in opacity, still under the pointer, mixing the node's own values into its text, rippling once from the window on each status refresh, and moving or fading the glyphs the window covers.

## 8. Rules

Do and don't:

- Derive every accent-tinted value from the saved accent. A hard-coded tint breaks as soon as another colour is saved.
- Keep blur out, and translucency to content surfaces at 90%. Blur repaints under every live row, and glyphs behind typed text, menus, scrolled rows or dense live figures read as content or flicker.
- Keep gradients out of working views and colour a state only with a state colour. The cell meter's scale and the hatched unknown state are data encodings, not decoration, and a state in the accent disappears under a palette of the same hue.
- Use compact actions only in toolbars, table rows and pagination, and embedded actions only inside a field, card or tool cluster; keep a bar's controls flat and a form's fields bordered, with no filled primary action in a title strip. Mixed heights break row alignment, a bordered or filled control in a bar competes with the content below it, and a flat field in a form loses its edge.
- Style new work with component variants and utilities on the instance, and read values only from tokens. Page-level styles and literal values are how the views drifted apart.
- Use only the type steps and weights above. Off-scale text breaks the hierarchy between data and labels.
- Keep the node's name and state visible in every node-scope view. An action on the wrong node is the most expensive mistake in a fleet.
- Keep window names as the command words in every language, translating only their tooltips and accessible names. A translated tab that reads differently from what `open` and `goto` take breaks the line between clicking and typing.
- Give every message and result a place that is always there, such as the form's message slot, the command output or a toast, and keep a pane's rows on one line. A line that appears in the flow or wraps moves what the user was about to touch.
- Confirm every batch and destructive action with the nodes it acts on and what it interrupts. A fleet-wide action sent by mistake interrupts every proxy at once.

Overrides of the global guide:

| Global rule | Project value | Reason |
| --- | --- | --- |
| Control heights from the set 36, 40, 44 | 40, 32, 44, and 28 for embedded actions | Compact actions fit table rows and toolbars without enlarging them; touch keeps 44 |
| Adjust instances with utilities, not page-level CSS | The schema form theme and the colour picker's own parts keep one stylesheet beside their component; the node console keeps one for its sign-in morph | Their rules follow markup that rjsf nests recursively or that a library renders, or define view transitions, which utilities on one instance cannot address |
| Working screens have no gradients, glows or textures | Every signed-in view keeps the sign-in glyph field behind it in calm mode; cell meters fill along a success-to-danger scale | Sign-in and the working views read as one product; the field is faint, carries the product's own values, stays still under the pointer and shows only faintly through translucent cards. A meter's colour says where its share sits against its limits, so load and quota read at a glance across a fleet; it moves only with its data |
| Problems the user must fix stay inline where they occur | A refused sign-in marks its prompt and the line at fault inline and reports why in the shared toast | The sign-in prompt keeps one shape at the window's centre, and its messages appear where every other message of the product does |
| Unavailable values show as "—", not 0 or blank | A title strip leaves out a reading it lacks instead of marking it; a known zero still reads 0 | The strip is the view's one line of status: a dash there reads as noise, and a dash that turns into a value changes the line's width, so everything after it moves |

## 9. Tokens in code

- `web/packages/shared/src/styles/tokens.css` holds every token and nothing else: the root scope, the dark scope `:root.dark, :root[data-resolved-theme='dark']`, the more-contrast and coarse-pointer overrides, and the `@theme inline` block that exposes tokens to Tailwind. `base.css` holds element defaults and `components.css` shared component classes; `global.css` imports Tailwind and the three files in that order.
- Component roles use the shadcn names (`--background`, `--foreground`, `--card`, `--popover`, `--primary`, `--muted`, `--accent`, `--border`, `--ring`, `--chart-*`), read through utilities such as `bg-card`, `text-muted-foreground` and `border-border`. Product tokens use the `--color-*`, `--space-*`, `--radius-*`, `--control-height*`, `--shell-*`, `--pane-*`, `--shadow-*`, `--dur-*`, `--ease-*` and `--z-*` prefixes and are read with `var()`. A product token never reuses a component role name with another meaning, and no token is an alias of another: code reads the names in sections 2 to 5.
- Utilities follow the scales: `rounded-md` is R/2, `rounded-lg` and `rounded-xl` are R, `rounded-[var(--radius-window)]` is the floating step; sizes are `text-2xs`, `text-xs`, `text-compact`, `text-sm`, `text-base`, `text-xl` and `text-2xl`; weights are `font-normal`, `font-medium` and `font-semibold`.
- `web/apps/panel/src/theme/appearance.ts` derives the tokens in section 2's derivation table and the radius tokens from the saved `{ theme, color, radius }`; the defaults are `system`, `#6D4ED1` and 12, and the presets `#6D4ED1`, `#2563EB`, `#0891B2`, `#15803D`, `#C65B13` and `#BE185D`. `ThemeProvider` writes them on the document root, from the appearance meta the server fills; before scripts load, the server paints the saved theme's `--color-canvas`.
- Shared patterns live in `web/packages/shared/src/components`: the glyph field, the prompt input, the cell meter and the terminal parts both apps use (input line, prompt label, buffer and its entries, readout rows and window marks); their classes, such as a terminal line's typing keyframes, sit in `components.css`.
- The component library is shadcn `base-nova` on Base UI with lucide icons (`web/packages/shared/components.json`); patterns map to `components/ui` primitives and the shared components beside them. Tokens the shadcn CLI adds go into `tokens.css`.
- Code that paints, such as charts, reads tokens with `getComputedStyle(document.documentElement)` and redraws when the theme or appearance changes.
- To add a token: give it light and dark values in `tokens.css`, derive it in `appearance.ts` when it depends on the accent, register it in `@theme inline` when components need a utility, and add its row here, in one change.

## 10. Showcase

The two sign-ins are the only showcase surfaces and share one terminal prompt. They have no card or visible heading: below a first row with the wordmark, theme and language, the prompt sits at the window's centre in the data face. The node console asks for its password after a `$` in the accent; the panel asks `login:` for the email and `password:` below it, its labels in the accent, Enter on the email moving to the password. Each line draws its characters, or for a password a dot in a wider cell popping in, with a blinking block cursor at the end, a bar between characters and the selection as an accent band over whole cells; the password line ends in the visibility toggle and an embedded submit. A placeholder, in weight 300 at reduced contrast, shows only while its line is empty and unfocused. A refused attempt shakes the prompt once and keeps a danger border on it, and the line the refusal is about, until typing resumes; its message goes to a danger toast, which a further refusal replaces and typing again closes. A sign-in turns the prompt success-green and holds 520ms; the node's prompt then morphs into its terminal window, the panel's cross-fades into the shell. While checking the session each shows a spinner, and an unreachable service shows a card at 24rem with a retry.

Behind both a canvas of protocol names, settings, documentation addresses and log fragments in the data face fills the window below the first row in three layers at 10, 12 and 14px and rising opacity, longer fragments further back; each layer is scattered by best-candidate sampling, tilted mostly within 12°, whole at the window's edges and kept a narrow margin from the prompt, so layers overlap without leaving holes. The pointer pushes and leans glyphs aside, nearer layers further, a keystroke streams a few toward the prompt, a refusal sends a danger shock outward and a sign-in a success wave across; while idle one glyph at a time types a new text with a cursor at its end. The field pauses while the page is hidden and is a still field that only changes colour under reduced motion.
