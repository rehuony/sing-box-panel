import * as React from 'react';
import { cn } from '@shared/lib/utils';
import { Input as InputPrimitive } from '@base-ui/react/input';

function Input({ className, type, ...props }: React.ComponentProps<'input'>) {
  return (
    <InputPrimitive
      type={type}
      data-slot='input'
      className={cn(
        'h-(--control-height) w-full min-w-0 rounded-lg border border-input bg-[var(--color-field)] px-3 py-2 text-base transition-colors outline-hidden file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:border-[var(--input-focus)] aria-invalid:focus-visible:border-destructive disabled:pointer-events-none disabled:cursor-default disabled:bg-[var(--color-surface-raised)] disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40',
        // A number takes typing alone, without the browser's spinner, in tabular numerals.
        '[&[type=number]]:[appearance:textfield] [&[type=number]]:tabular-nums [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none',
        className,
      )}
      {...props}
    />
  );
}

export { Input };
