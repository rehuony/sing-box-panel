# sing-box-panel design system

The design system of the sing-box-panel web interface: roles, scales, formulas, patterns and rules. It extends the global design guide and repeats nothing from it. Every value is the one the tokens in code use; a change to one changes the other in the same work.

The panel interface serves two scopes. The panel scope covers the fleet: the node overview, subscriptions, the panel log and panel settings. The node scope covers one node: its overview, core versions, configuration, logs and settings. The node console is a separate interface that one node serves on its own host for its enrollment with a panel; it uses the same tokens, type and components.

## 1. Character

| Property | Declaration |
| --- | --- |
| Mood | Calm and technical; the interface reports state and stays out of the way |
| Colour temperature | Cool neutrals that carry a faint cast of the saved accent |
| Density | Dense working screens; data at 12–14px, one toolbar row per view |
| Type | One bundled variable sans at three weights; one bundled monospace for machine text |
| Surface material | Opaque surfaces separated by hairlines; floating layers detach from the window edge and cast a shadow |
| Motion | Immediate response to input, short state transitions, no idle motion |

Properties that make the product recognisable:

- The saved accent derives the whole neutral scale, so every palette keeps one coherent tint and accessible text.
- One radius parameter derives three corner steps, so a single setting reshapes every element in proportion.
- Status colours never take the accent, so a state reads the same under every palette and in both themes.
- The navigation rail floats inside the window with an even inset, so the shell reads as a layer above the workspace.
- Every view declares its scope: panel views name the fleet, node views keep the node's identity and state in their first row.

## 2. Colour

Light values sit in the root scope, dark values in the dark scope. A derived token is recomputed from the saved accent A and the base surface S (light `#ffffff`, dark `#18171e`) before first paint; the table shows the result for the default A `#6D4ED1`.

| Role | Token | Light | Dark | Use |
| --- | --- | --- | --- | --- |
| Canvas | `--color-canvas` | `#f9f8fd` | `#1a1822` | Window background behind the rail and workspace |
| Surface | `--card`, `--popover`, `--color-surface` | `#fefeff` | `#18171f` | Cards, tables, forms, dialogs, menus, the rail |
| Surface raised | `--color-surface-raised` | `#f9f8fd` | `oklch(24% 0.02 274)` | Nested blocks, hovered rows, disabled fields |
| Muted fill | `--muted` | `#f9f8fd` | `#1b1925` | Secondary actions, segmented tracks |
| Border | `--border`, `--input` | `#e8e3f8` | `#332957` | Hairlines, dividers, input borders |
| Strong border | `--color-border-strong` | `#d9d1f3` | `#3c2e69` | Hovered inputs, scrollbar thumbs |
| Text | `--foreground` | `oklch(22% 0.018 274)` | `oklch(94% 0.01 286)` | Primary text and icons |
| Secondary text | `--color-text-secondary` | `oklch(43% 0.02 274)` | `oklch(76% 0.016 286)` | Field labels, secondary cells |
| Muted text | `--muted-foreground` | `oklch(48% 0.018 274)` | `oklch(70% 0.016 286)` | Meta, table headers, timestamps |
| Faint text | `--color-text-faint` | `oklch(51% 0.016 280)` | `oklch(60% 0.018 286)` | Placeholders, disabled text |
| Accent | `--primary`, `--ring` | `#6d4ed1` | `#947edd` | Primary action, keyboard focus, links, first chart series |
| Accent hover | `--color-accent-hover` | `#6045b8` | `#a18de1` | Primary action on hover |
| On accent | `--primary-foreground` | `#ffffff` | `#17151e` | Text and icons on an accent fill |
| Accent soft | `--accent` | `#f2effb` | `#26203b` | Selected navigation, selected rows, pressed toggles |
| Success | `--color-success`, `-soft`, `-text`, `-border` | `oklch(59% 0.14 165)`, `oklch(95% 0.03 165)`, `oklch(42% 0.11 165)`, `oklch(82% 0.06 165)` | `oklch(72% 0.14 165)`, `oklch(27% 0.055 165)`, `oklch(83% 0.105 165)`, `oklch(46% 0.08 165)` | Healthy and completed states |
| Warning | `--color-warning`, `-soft`, `-text`, `-border` | `oklch(65% 0.16 62)`, `oklch(96% 0.035 72)`, `oklch(46% 0.12 62)`, `oklch(83% 0.07 62)` | `oklch(78% 0.14 72)`, `oklch(28% 0.055 72)`, `oklch(84% 0.11 72)`, `oklch(48% 0.08 72)` | Degraded or stale states, pending action, one-time secrets |
| Danger | `--color-danger`, `-soft`, `-text`, `-border` | `oklch(56% 0.18 20)`, `oklch(95% 0.035 20)`, `oklch(43% 0.14 20)`, `oklch(82% 0.07 20)` | `oklch(70% 0.17 20)`, `oklch(28% 0.06 20)`, `oklch(83% 0.105 20)`, `oklch(48% 0.08 20)` | Failed or unreachable states, errors, destructive actions |
| Info | `--color-info`, `-soft`, `-text`, `-border` | `oklch(58% 0.15 248)`, `oklch(95% 0.03 248)`, `oklch(44% 0.12 248)`, `oklch(82% 0.06 248)` | `oklch(72% 0.13 248)`, `oklch(28% 0.055 248)`, `oklch(82% 0.095 248)`, `oklch(47% 0.075 248)` | Neutral notices, third chart series |
| Scrim | `--color-scrim` | `oklch(22% 0.018 274 / 34%)` | `oklch(7% 0.01 274 / 68%)` | Behind modal layers |
| Syntax | `--color-syntax-property`, `-string`, `-number`, `-keyword` | `#245fa1`, `#21774b`, `#9a541e`, `#8c4989` | `#88bdf3`, `#9ac99d`, `#e7b685`, `#d5a0d7` | JSON syntax in the configuration editor |
| Brand | `--color-brand-navy`, `--color-brand-orange` | `oklch(20.8% 0.042 265.755)`, `oklch(70.5% 0.18 47.6)` | `oklch(17% 0.03 265.755)`, `oklch(76% 0.16 55)` | Logo mark, second chart series |

Derivation, with mix(A, S, w) as A mixed into S at weight w:

| Token | Light | Dark |
| --- | --- | --- |
| `--primary` | A moved toward black in 1% steps until it reaches 4.6:1 against `--accent` and S | A moved toward white the same way |
| `--color-accent-hover` | `--primary` mixed 12% toward black | `--primary` mixed 12% toward white |
| `--primary-foreground` | `#ffffff` when it reaches 4.5:1 against `--primary`, otherwise `#17151e` | Same rule |
| `--accent` | A at 9% | A at 16% |
| `--color-canvas` | A at 4% | A at 2% |
| `--card`, `--popover` | A at 0.5% | A at 0.5% |
| `--muted`, `--color-surface-raised` | A at 4% | `--muted` A at 4%; raised fixed |
| `--border`, `--input` | A at 16% | A at 32% |
| `--color-border-strong` | A at 26% | A at 42% |

Accent policy: the accent fills the one primary action of an area, marks selection through its soft fill, rings keyboard focus, colours links and the first chart series, and tints neutrals only through derivation. It never marks a state, a badge, the logo or a destructive action, and never fills a region.

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
| 3xl | `--text-3xl` | 28px | 36px | -0.02em | Sign-in heading |

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
| Canvas | `--color-canvas`, flat | — | Window background |
| Content | `--color-surface` | `--border` hairline; `--shadow-card` only where a hairline is not enough | Cards, tables, forms, object rows |
| Raised | `--color-surface-raised` | Tint | Nested blocks, hovered rows, code, configuration and log output |
| Popover | `--popover` | Hairline and the popover shadow | Menus, select lists, popovers, tooltips |
| Floating | `--color-surface` | Hairline and `--shadow-panel` | Rail, dialogs, alert dialogs, sheets, toasts |

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

| Layer | Token | Value |
| --- | --- | --- |
| Sticky content | `--z-sticky` | 200 |
| Modal | `--z-modal` | 400 |
| Popover above modal | `--z-popover` | 450 |

## 6. Components

| Pattern | Anatomy | Variants | States |
| --- | --- | --- | --- |
| Action | Label, optional 16px leading icon | Primary, outline, secondary, ghost, link, destructive (danger soft fill, 25% danger border); standard, compact (13px text), large, embedded; each with a square icon form | Hover, pressed, keyboard focus, pending keeps the label and width |
| State indicator | Dot or 20px pill with 12px medium text | Success, warning, danger, info, neutral | Static; a change of state replaces the colour without moving the row |
| Single choice | Trigger with value and chevron; list with a checkmark on the current value | Standard, compact | A mouse-opened list keeps options neutral; keyboard highlight follows focus |
| Field row | Label column, control filling the rest, optional help trigger beside the label | Single column; stacked below 520px | Invalid marks the control; messages go to the action bar or a toast |
| Data table | 44px header, 64px rows, row actions at the end | Paged or scrolled within its region | Loading, empty and error centred in the region; selection by soft fill |
| Pagination | Page-size choice, previous and next, page input, total | — | Enter or blur commits a page, Escape cancels |
| Toolbar row | Tabs or filters at the start, actions at the end | List toolbar, object row | Stays in place while content scrolls |
| Object row | Back, name, state, context values, actions | Node, source, channel | Actions act on the named object only; below 1024px the context values move to a second line that scrolls sideways |
| Node card | Name as the card's link, host, state indicator, core version, status, period usage meter | — | Hover strengthens the hairline; keyboard focus rings the card; the meter turns warning from 90% and danger once the quota is reached |
| Enrollment code | Command and code, each with a copy action, then a one-line warning note with the expiry | — | Shown once; closing discards it |
| Device tile | 48px tile on the raised surface with a 20px icon and a 14px presence dot at its lower right, ringed in the card colour | One per connection state | Only the dot's colour changes |
| Grouped rows | Group label in xs medium muted text, then rows on the raised surface split by hairlines; label at the start, value at the end | Identifiers end in an embedded copy action | A long value truncates and shows in full on hover; copy confirms with a check for 1.5s |
| Dialog | Title, close, scrolling body, footer with actions at the end | Standard 30rem; wide with a section rail on the left | Cancel first, auxiliary next, primary last; Escape closes the innermost layer |
| Notification | Compact toast with a status colour | Success, warning, danger, info | Dismisses after 3 seconds, pausing on hover and focus; a repeated failure reuses one toast |
| Loading region | Accent spinner over a muted label | — | Short delayed fade-in inside the pending region only |
| Help | Information icon beside a label, tooltip | — | Opens after 200ms on hover, at once on click or tap; outside the Tab order |
| Floating tool cluster | Embedded icon actions on a surface pinned to a corner | — | Hover tint; never covers the text it serves |

## 7. Layout

- The shell has two regions: a floating navigation rail and the workspace. There is no global header; each view's first row is its toolbar row or its object row.
- The rail lists panel-scope destinations only: nodes, subscriptions, logs and settings. Theme, language and sign-out sit at its foot as equal embedded actions.
- Nodes are first-class objects. The node overview is a searchable, paged grid of node cards; its toolbar row carries the fleet's online count and summed period usage, never a summed quota, and each card carries the node's name and host, state, core version, and its own period usage against its own quota. Opening a node shows its object row, then tabs for overview, core versions, configuration, logs and settings.
- A node's settings tab edits its name, published host and traffic quota and period in one saved group, and keeps re-enrollment and removal in a separate destructive section.
- Runtime controls belong to the node they act on and sit in that node's object row.
- A node that is offline or not enrolled keeps what it last reported in its object row, marked stale with when it was last seen, and every tab that commands the node shows one region state in place of its view; its settings tab stays available, and a node not yet enrolled offers its enrollment code there.
- A panel-scope view never shows one node's runtime state as global state.
- Views have no title band: the rail and the object row name the place.
- The node console has no rail. Its first row spans the window at the shell inset, with the wordmark at the start and theme, language and sign-out as equal embedded actions at the end. Below it one card, at most 30rem wide, sits at the window's centre; the unavailable states use the same card at 24rem, and sign-in is the showcase prompt in section 10.
- Signed in, the card leads with the node as a device tile, its hostname, and its connection state with how long it has held. An unenrolled node continues with the enrollment code form, then its own details; an enrolled node continues with grouped rows for its panel and for itself. Re-enrollment and unenrollment sit in the card's more-actions menu; unenrollment is the destructive item and confirms with the panel it leaves.
- While the node is not connected, a warning block above the groups shows the last error and the next attempt, and stays until the node connects, so the centred card does not change height on each retry.
- The sign-in field stays behind the signed-in card, calm: 60% of its sign-in opacity, still under the pointer, mixing the node's own values into its text, rippling once from the card on each status refresh, and moving or fading the glyphs the card covers.

## 8. Rules

Do and don't:

- Derive every accent-tinted value from the saved accent. A hard-coded tint breaks as soon as another colour is saved.
- Keep translucency and blur out of the interface. Text over a blurred, moving background loses contrast, and many live rows repaint the blur constantly.
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
| Adjust instances with utilities, not page-level CSS | The schema form theme and the colour picker's own parts keep one stylesheet beside their component; each sign-in keeps one for its scene and keyframes | Their rules follow markup that rjsf nests recursively or that a library renders, or define keyframes, which utilities on one instance cannot address |
| Working screens have no gradients, glows or textures | The node console's signed-in view keeps the sign-in field behind its card in calm mode | Sign-in and the console read as one surface; the field is faint, carries the node's own values and stays still under the pointer |

## 9. Tokens in code

- `web/packages/shared/src/styles/tokens.css` holds every token and nothing else: the root scope, the dark scope `:root.dark, :root[data-resolved-theme='dark']`, the more-contrast and coarse-pointer overrides, and the `@theme inline` block that exposes tokens to Tailwind. `base.css` holds element defaults and `components.css` shared component classes; `global.css` imports Tailwind and the three files in that order.
- Component roles use the shadcn names (`--background`, `--foreground`, `--card`, `--popover`, `--primary`, `--muted`, `--accent`, `--border`, `--ring`, `--chart-*`, `--sidebar-*`), read through utilities such as `bg-card`, `text-muted-foreground` and `border-border`. Product tokens use the `--color-*`, `--space-*`, `--radius-*`, `--control-height*`, `--shadow-*`, `--dur-*`, `--ease-*` and `--z-*` prefixes and are read with `var()`. A product token never reuses a component role name with another meaning, and no token is an alias of another: code reads the names in sections 2 to 5.
- Utilities follow the scales: `rounded-md` is R/2, `rounded-lg` and `rounded-xl` are R, `rounded-[var(--radius-window)]` is the floating step; sizes are `text-2xs`, `text-xs`, `text-compact`, `text-sm`, `text-base`, `text-xl`, `text-2xl` and `text-3xl`; weights are `font-normal`, `font-medium` and `font-semibold`.
- `web/apps/panel/src/theme/appearance.ts` derives the tokens in section 2's derivation table and the radius tokens from the saved `{ theme, color, radius }`; the defaults are `system`, `#6D4ED1` and 12, and the presets `#6D4ED1`, `#2563EB`, `#0891B2`, `#15803D`, `#C65B13` and `#BE185D`. `ThemeProvider` writes them on the document root, from the appearance meta the server fills; before scripts load, the server paints only `--color-canvas` with the same blend.
- The component library is shadcn `base-nova` on Base UI with lucide icons (`web/packages/shared/components.json`); patterns map to `components/ui` primitives and the shared components beside them. Tokens the shadcn CLI adds go into `tokens.css`.
- Code that paints, such as charts, reads tokens with `getComputedStyle(document.documentElement)` and redraws when the theme or appearance changes.
- To add a token: give it light and dark values in `tokens.css`, derive it in `appearance.ts` when it depends on the accent, register it in `@theme inline` when components need a utility, and add its row here, in one change.

## 10. Showcase

The two sign-in pages are the only showcase surfaces. On the panel, one responsive frame holds session checking, sign-in and service unavailability on the same canvas and surface tokens as the working views. Its scene is a group of decorative geometric characters: their pupils follow a fine pointer, they turn toward form focus, and they close their eyes while the password field or its visibility toggle has focus or the password is shown. The scene receives presentation state only, rests when nothing happens, switches poses without animation under reduced motion and never intercepts the form.

The node console's sign-in has no card or visible heading: its password is a terminal prompt at the window's centre, a `$` in the accent, each character popping in as a dot in a cell as wide as the input's own character, with a blinking block cursor at the end, a bar between characters and the selection as an accent band over whole cells, then the visibility toggle and an embedded submit. Its placeholder, in weight 300 at reduced contrast, shows only while the prompt is empty and unfocused. A refused password shakes the prompt once with a danger border and prints its message as the prompt's output row: attached below on the same surface and border, a danger cross and danger text in the data face, sliding down from the prompt outside the flow and closing when typing resumes; a sign-in turns it success-green, holds 520ms while the status loads, and morphs the prompt into the console card. Behind it a canvas of protocol names, settings, documentation addresses and log fragments in the data face fills the window below the first row in three layers at 10, 12 and 14px and rising opacity, longer fragments further back; each layer is scattered by best-candidate sampling, tilted mostly within 12°, whole at the window's edges and kept a narrow margin from the prompt, so layers overlap without leaving holes. The pointer pushes and leans glyphs aside, nearer layers further, a keystroke streams a few toward the prompt, a refusal sends a danger shock outward and a sign-in a success wave across; while idle one glyph at a time types a new text with a cursor at its end. The field pauses while the page is hidden and is a still field that only changes colour under reduced motion.
