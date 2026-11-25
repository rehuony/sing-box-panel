import { cva } from 'class-variance-authority';

export const buttonVariants = cva(
  'group/button inline-flex shrink-0 items-center justify-center rounded-lg border border-transparent bg-clip-padding text-sm font-medium whitespace-nowrap transition-[background-color,border-color,color,transform] duration-150 ease-out active:scale-[0.975] motion-reduce:active:scale-100 outline-none select-none focus-visible:border-primary disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4',
  {
    variants: {
      variant: {
        // The primary action takes the accent's tint, as a selected pane does; a destructive one takes the danger tint.
        default: 'border-primary/50 bg-accent text-primary hover:bg-primary/15',
        outline:
          'border-border bg-card text-foreground hover:bg-[var(--color-hover)] aria-expanded:bg-[var(--color-hover)] aria-pressed:border-primary/50 aria-pressed:bg-accent',
        secondary:
          'border-border bg-muted text-foreground hover:bg-[var(--color-hover)] aria-expanded:bg-[var(--color-hover)]',
        ghost:
          'text-muted-foreground hover:bg-[var(--color-hover)] hover:text-foreground aria-expanded:bg-[var(--color-hover)] aria-expanded:text-foreground aria-pressed:bg-accent aria-pressed:text-foreground',
        destructive:
          'border-[var(--color-danger-border)] bg-[var(--color-danger-soft)] text-[var(--color-danger-text)] hover:bg-destructive/20',
        link: 'text-primary underline-offset-4 hover:underline',
        // A bar's actions: no fill or border at rest, the text colour, and the accent's soft fill while pressed.
        flat: 'text-foreground hover:bg-[var(--color-hover)] aria-expanded:bg-[var(--color-hover)] aria-pressed:bg-accent',
      },
      size: {
        'default': 'h-(--control-height) min-h-(--control-height) gap-2 px-4',
        'xs': 'h-(--control-height-xs) min-h-(--control-height-xs) gap-1 px-2 text-xs',
        'sm': 'h-(--control-height-sm) min-h-(--control-height-sm) gap-1.5 px-3 text-[0.8125rem]',
        'lg': 'h-(--control-height-lg) min-h-(--control-height-lg) gap-2 px-5',
        'icon': 'size-(--control-height) min-h-(--control-height) p-0',
        'icon-xs': 'size-(--control-height-xs) min-h-(--control-height-xs) p-0',
        'icon-sm': 'size-(--control-height-sm) min-h-(--control-height-sm) p-0',
        'icon-lg': 'size-(--control-height-lg) min-h-(--control-height-lg) p-0',
        'content': 'h-auto min-h-(--control-height) gap-2 p-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);
