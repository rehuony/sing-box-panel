/**
 * Where a dragged row or board cell will land while a copy of it follows the
 * pointer: its own content at 40% in a dashed slot washed with hover, inset
 * from the hairlines around it.
 */
export const dropSlot = 'relative isolate before:absolute before:inset-1 before:-z-1 before:rounded-[var(--radius-control)] before:border before:border-dashed before:border-[var(--color-border-strong)] before:bg-[var(--color-hover)] *:opacity-40';

/** The same slot for a card that draws its own border: the border turns dashed. */
export const dropSlotCard = 'border-dashed border-[var(--color-border-strong)] bg-[var(--color-hover)] *:opacity-40';
