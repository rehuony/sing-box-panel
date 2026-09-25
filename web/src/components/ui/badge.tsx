import type { VariantProps } from 'class-variance-authority';

import { cva } from 'class-variance-authority';
import { useRender } from '@base-ui/react/use-render';
import { mergeProps } from '@base-ui/react/merge-props';

import { cn } from '@/lib/utils';

const badgeVariants = cva(
  'group/badge inline-flex h-5 w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-4xl border border-transparent px-2 py-1 text-xs font-medium whitespace-nowrap transition-[background-color,border-color,color] outline-none has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&>svg]:pointer-events-none [&>svg]:size-3!',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground [a]:hover:bg-primary/80',
        secondary:
          'bg-secondary text-secondary-foreground [a]:hover:bg-secondary/80',
        success:
          'border-status-success-border bg-status-success-soft text-status-success-foreground',
        warning:
          'border-status-warning-border bg-status-warning-soft text-status-warning-foreground',
        info:
          'border-status-info-border bg-status-info-soft text-status-info-foreground',
        destructive:
          'bg-destructive/10 text-destructive dark:bg-destructive/20 [a]:hover:bg-destructive/20',
        outline:
          'border-border text-foreground [a]:hover:bg-muted',
        ghost:
          'hover:bg-muted dark:hover:bg-muted/50',
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
