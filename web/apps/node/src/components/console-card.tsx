import type { ComponentProps } from 'react';

import { cn } from '@shared/lib/utils';

/** The console's one card, centered in the window. */
export function ConsoleCard({ className, ...props }: ComponentProps<'section'>) {
  return (
    <section
      className={cn('w-full max-w-120 rounded-[var(--radius-card)] border bg-card shadow-[var(--shadow-card)] [view-transition-name:console-card]', className)}
      {...props}
    />
  );
}
