/**
 * A subscription node card's face, one size wherever a node shows: the pool's
 * grid, a group's members and nodes, and the picker. A 32px title bar over
 * three lines in the data face.
 */
export const nodeCardFace = 'relative flex h-[var(--node-card-height)] min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border font-[family-name:var(--font-data)] text-compact';

/** Node cards as cards of their own in one grid with the panes' gaps. */
export const nodeCardGrid = 'grid grid-cols-[repeat(auto-fill,minmax(min(100%,var(--node-card-min-width)),1fr))] content-start gap-3';
