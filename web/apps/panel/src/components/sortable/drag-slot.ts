/**
 * Where a dragged row will land while a copy of it follows the pointer: its
 * own content at 40% in a dashed slot washed with hover, inset from the
 * hairlines around it.
 */
export const dropSlot = 'relative isolate before:absolute before:inset-1 before:-z-1 before:rounded-[var(--radius-control)] before:border before:border-dashed before:border-[var(--color-border-strong)] before:bg-[var(--color-hover)] *:opacity-40';

/**
 * The same slot as an element of its own, for a table row: a row's own
 * pseudo-element would become a cell and move the columns. The row is its
 * positioned, isolated container and fades its other cells.
 */
export const dropSlotRow = {
  row: 'relative isolate [&>td:not(:first-child)]:opacity-40',
  slot: 'pointer-events-none absolute inset-1 -z-1 rounded-[var(--radius-control)] border border-dashed border-[var(--color-border-strong)] bg-[var(--color-hover)]',
};

/** The same slot for a card that draws its own border: the border turns dashed. */
export const dropSlotCard = 'border-dashed border-[var(--color-border-strong)] bg-[var(--color-hover)] *:opacity-40';

/** The page-wide grabbing hand, over whatever the pointer passes while something drags. */
const grabbing = ['cursor-grabbing!', '[&_*]:cursor-grabbing!'];

/** Shows the grabbing hand across the page from a drag's start until it ends or is cancelled. */
export function showGrabbing(on: boolean) {
  for (const name of grabbing) document.documentElement.classList.toggle(name, on);
}
