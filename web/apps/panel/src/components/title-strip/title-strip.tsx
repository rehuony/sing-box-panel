import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';

interface TitleStripProps {
  /** The strip's name for assistive technology, where its content does not name it. */
  label?: string;
  className?: string;
  children?: ReactNode;
}

/**
 * A view's first row: one strip on the content surface with a hairline that
 * holds the view's toolbar or object row in flat compact controls, grouped
 * by the status line's dividers. Its content wraps below when narrow.
 */
export function TitleStrip({ children, className, label }: TitleStripProps) {
  return (
    <div
      aria-label={label}
      className={cn(
        'flex min-h-[var(--shell-strip-height)] min-w-0 flex-none flex-wrap items-center gap-x-2 gap-y-1 rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)] px-4 py-1',
        className,
      )}
      data-slot='title-strip'
      role={label ? 'group' : undefined}
    >
      {children}
    </div>
  );
}

/** The status line's divider between groups of a strip's controls. */
export function StripDivider({ className }: { className?: string }) {
  return <span aria-hidden='true' className={cn('h-5 w-px flex-none bg-border', className)} />;
}
