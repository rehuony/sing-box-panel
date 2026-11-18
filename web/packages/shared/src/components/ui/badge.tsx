import type { VariantProps } from 'class-variance-authority';

import { cn } from '@shared/lib/utils';
import { cva } from 'class-variance-authority';
import { useRender } from '@base-ui/react/use-render';
import { mergeProps } from '@base-ui/react/merge-props';

const badgeVariants = cva(
  'group/badge inline-flex h-5 w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-4xl border border-transparent px-2 py-1 text-xs font-medium whitespace-nowrap transition-[background-color,border-color,color] outline-none has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&>svg]:pointer-events-none [&>svg]:size-3!',
  {
    variants: {
      variant: {
        default: 'bg-secondary text-secondary-foreground [a]:hover:bg-[var(--color-hover)]',
        secondary:
          'bg-secondary text-secondary-foreground [a]:hover:bg-secondary/80',
        success:
          'border-[var(--color-success-border)] bg-[var(--color-success-soft)] text-[var(--color-success-text)]',
        warning:
          'border-[var(--color-warning-border)] bg-[var(--color-warning-soft)] text-[var(--color-warning-text)]',
        info:
          'border-[var(--color-info-border)] bg-[var(--color-info-soft)] text-[var(--color-info-text)]',
        destructive:
          'border-[var(--color-danger-border)] bg-[var(--color-danger-soft)] text-[var(--color-danger-text)]',
        outline:
          'border-border text-foreground [a]:hover:bg-[var(--color-hover)]',
        ghost:
          'hover:bg-[var(--color-hover)]',
        link: 'text-primary underline-offset-4 hover:underline',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

function Badge({
  className,
  variant = 'default',
  render,
  ...props
}: useRender.ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
  return useRender({
    defaultTagName: 'span',
    props: mergeProps<'span'>(
      {
        className: cn(badgeVariants({ variant }), className),
      },
      props,
    ),
    render,
    state: {
      slot: 'badge',
      variant,
    },
  });
}

export { Badge };
