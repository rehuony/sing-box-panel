import { cva } from 'class-variance-authority';

export const buttonVariants = cva(
  'group/button inline-flex shrink-0 items-center justify-center rounded-lg border border-transparent bg-clip-padding text-sm font-medium whitespace-nowrap transition-[background-color,border-color,color,transform] duration-150 ease-out active:scale-[0.975] motion-reduce:active:scale-100 outline-none select-none disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4',
  {
    variants: {
      variant: {
        default: 'border-primary bg-primary text-primary-foreground hover:bg-primary/90',
        outline:
          'border-border bg-card text-foreground hover:bg-muted aria-expanded:bg-muted aria-pressed:border-primary/50 aria-pressed:bg-accent aria-pressed:text-primary',
        secondary:
          'border-border bg-muted text-foreground hover:bg-accent aria-expanded:bg-accent',
        ghost:
          'text-muted-foreground hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground',
        destructive:
          'border-destructive/25 bg-destructive/10 text-destructive hover:bg-destructive/20',
        link: 'text-primary underline-offset-4 hover:underline',
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
