import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { WindowMarks } from '@shared/components/terminal';

interface TitleStripProps {
  /** The strip's name for assistive technology, where its content does not name it. */
  label?: string;
  className?: string;
  children?: ReactNode;
}

/**
 * A view's first row: one strip on the dense surface with a hairline, the
 * window's three marks at its start on a desktop, then the view's toolbar or
 * object row. Its content wraps below when narrow.
 */
export function TitleStrip({ children, className, label }: TitleStripProps) {
  return (
    <div
      aria-label={label}
      className={cn(
        'flex min-h-[var(--shell-strip-height)] min-w-0 flex-none flex-wrap items-center gap-x-3 gap-y-2 rounded-[var(--radius-card)] border bg-[var(--color-surface-dense)] px-3 py-1.5',
        className,
      )}
      data-slot='title-strip'
      role={label ? 'group' : undefined}
    >
      <WindowMarks className='me-1 max-sm:hidden' />
      {children}
    </div>
  );
}
