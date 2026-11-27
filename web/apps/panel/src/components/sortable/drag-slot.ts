/**
 * Where a dragged row will land while a copy of it follows the pointer: its
 * own content at 40% in a dashed slot washed with hover, inset from the
 * hairlines around it.
 */
export const dropSlot = 'relative isolate before:absolute before:inset-1 before:-z-1 before:rounded-[var(--radius-control)] before:border before:border-dashed before:border-[var(--color-border-strong)] before:bg-[var(--color-hover)] *:opacity-40';

/** The same slot for a card that draws its own border: the border turns dashed. */
export const dropSlotCard = 'border-dashed border-[var(--color-border-strong)] bg-[var(--color-hover)] *:opacity-40';

/** The page-wide grabbing hand, over whatever the pointer passes while something drags. */
const grabbing = ['cursor-grabbing!', '[&_*]:cursor-grabbing!'];

/** Shows the grabbing hand across the page from a drag's start until it ends or is cancelled. */
export function showGrabbing(on: boolean) {
  for (const name of grabbing) document.documentElement.classList.toggle(name, on);
}
