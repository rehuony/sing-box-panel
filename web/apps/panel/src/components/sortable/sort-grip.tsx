import type { DraggableAttributes, DraggableSyntheticListeners } from '@dnd-kit/core';

import { cn } from '@shared/lib/utils';
import { GripVertical } from 'lucide-react';

/**
 * The handle a sortable row drags by: a faint grip that a press or a touch
 * picks up and that takes the keyboard, F2 starting and ending the move.
 */
export function SortGrip({ activatorRef, attributes, className, disabled, label, listeners }: {
  /** The handle's name, such as which entry it moves. */
  label: string;
  disabled?: boolean;
  className?: string;
  attributes: DraggableAttributes;
  listeners: DraggableSyntheticListeners;
  activatorRef: (element: HTMLElement | null) => void;
}) {
  return (
    <button
      {...attributes}
      {...listeners}
      aria-label={label}
      className={cn(
        'flex size-7 flex-none cursor-grab touch-none items-center justify-center rounded-[var(--radius-control)] text-[var(--color-text-faint)] outline-none hover:text-foreground focus-visible:shadow-[inset_0_0_0_1px_var(--ring)] disabled:cursor-default disabled:hover:text-[var(--color-text-faint)]',
        className,
      )}
      disabled={disabled}
      ref={activatorRef}
      type='button'
    >
      <GripVertical aria-hidden='true' className='size-4' />
    </button>
  );
}

/** The copy of a dragged row that follows the pointer: its name and a detail on the popover surface. */
export function SortOverlay({ detail, name }: { name: string; detail?: string }) {
  return (
    <div
      aria-hidden='true'
      className='flex h-11 max-w-80 min-w-56 cursor-grabbing items-center gap-2 rounded-[var(--radius-control)] border bg-popover ps-1.5 pe-3 text-sm shadow-[var(--shadow-panel)]'
    >
      <GripVertical className='size-4 flex-none text-[var(--color-text-secondary)]' />
      <span className='min-w-0 flex-1 truncate'>{name}</span>
      {detail ? <span className='flex-none text-xs text-[var(--color-text-secondary)]'>{detail}</span> : null}
    </div>
  );
}
