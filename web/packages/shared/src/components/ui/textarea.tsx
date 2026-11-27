import * as React from 'react';
import { cn } from '@shared/lib/utils';

function Textarea({ className, ...props }: React.ComponentProps<'textarea'>) {
  return (
    <textarea
      data-slot='textarea'
      className={cn(
        'flex field-sizing-content min-h-16 w-full rounded-lg border border-input bg-[var(--color-field)] px-2.5 py-2 text-base transition-colors outline-hidden placeholder:text-muted-foreground focus-visible:border-[var(--input-focus)] aria-invalid:focus-visible:border-destructive disabled:cursor-default disabled:bg-[var(--color-surface-raised)] disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40',
        className,
      )}
      {...props}
    />
  );
}

export { Textarea };
