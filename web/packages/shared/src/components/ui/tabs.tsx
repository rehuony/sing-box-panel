'use client';

import type { VariantProps } from 'class-variance-authority';

import { cn } from '@shared/lib/utils';
import { cva } from 'class-variance-authority';
import { Tabs as TabsPrimitive } from '@base-ui/react/tabs';

function Tabs({
  className,
  orientation = 'horizontal',
  ...props
}: TabsPrimitive.Root.Props) {
  return (
    <TabsPrimitive.Root
      data-slot='tabs'
      orientation={orientation}
      className={cn(
        'group/tabs flex gap-2 data-horizontal:flex-col',
        className,
      )}
      {...props}
    />
  );
}

const tabsListVariants = cva(
  'group/tabs-list inline-flex min-h-(--control-height) w-fit items-center justify-center rounded-lg p-[var(--tabs-inset,3px)] text-muted-foreground data-vertical:h-fit data-vertical:flex-col data-[variant=line]:rounded-none',
  {
    variants: {
      variant: {
        default: 'bg-muted',
        line: 'gap-1 bg-transparent',
        // A bar's tabs: an inset bar, a well on the canvas with a hairline whose current tab is a
        // raised thumb that slides to it, so the tabs read as one choice of several.
        pill: 'relative isolate h-(--control-height-sm) min-h-0 gap-0.5 rounded-[calc(var(--radius-control)+3px)] border bg-[var(--color-canvas)] p-0.5',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

function TabsList({
  children,
  className,
  variant = 'default',
  ...props
}: TabsPrimitive.List.Props & VariantProps<typeof tabsListVariants>) {
  return (
    <TabsPrimitive.List
      data-slot='tabs-list'
      data-variant={variant}
      className={cn(tabsListVariants({ variant }), className)}
      {...props}
    >
      {children}
      {variant === 'pill'
        ? (
            // The thumb sits behind the tabs, so the current one's text stays on top of it.
            <TabsPrimitive.Indicator
              data-slot='tabs-indicator'
              className='absolute top-(--active-tab-top) left-(--active-tab-left) -z-10 h-(--active-tab-height) w-(--active-tab-width) rounded-[var(--radius-control)] bg-popover shadow-[var(--shadow-thumb)] ring-1 ring-border transition-[left,width] duration-[var(--dur-short)] ease-[var(--ease-out)] motion-reduce:transition-none'
            />
          )
        : null}
    </TabsPrimitive.List>
  );
}

function TabsTrigger({ className, ...props }: TabsPrimitive.Tab.Props) {
  return (
    <TabsPrimitive.Tab
      data-slot='tabs-trigger'
      className={cn(
        'relative inline-flex min-h-[calc(var(--control-height)-2*var(--tabs-inset,3px))] flex-1 items-center justify-center gap-1.5 rounded-md border border-transparent px-3 py-1 text-sm font-medium whitespace-nowrap text-foreground/60 transition-[background-color,color,border-color,box-shadow] data-vertical:w-full data-vertical:justify-start hover:text-foreground outline-none disabled:pointer-events-none disabled:opacity-50 has-data-[icon=inline-end]:pr-1 has-data-[icon=inline-start]:pl-1 aria-disabled:pointer-events-none aria-disabled:opacity-50 dark:text-muted-foreground dark:hover:text-foreground group-data-[variant=default]/tabs-list:data-active:shadow-sm group-data-[variant=line]/tabs-list:data-active:shadow-none [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4',
        'group-data-[variant=line]/tabs-list:bg-transparent group-data-[variant=line]/tabs-list:data-active:bg-transparent dark:group-data-[variant=line]/tabs-list:data-active:border-transparent dark:group-data-[variant=line]/tabs-list:data-active:bg-transparent',
        'data-active:border-border data-active:bg-card data-active:text-foreground',
        // In the inset bar the other tabs take the hover wash; the current one shows the thumb behind it.
        'group-data-[variant=pill]/tabs-list:h-full group-data-[variant=pill]/tabs-list:min-h-0 group-data-[variant=pill]/tabs-list:px-2.5 group-data-[variant=pill]/tabs-list:text-muted-foreground dark:group-data-[variant=pill]/tabs-list:text-muted-foreground group-data-[variant=pill]/tabs-list:hover:text-foreground group-data-[variant=pill]/tabs-list:not-data-active:hover:bg-[var(--color-hover)] group-data-[variant=pill]/tabs-list:focus-visible:border-primary',
        'group-data-[variant=pill]/tabs-list:data-active:border-transparent group-data-[variant=pill]/tabs-list:data-active:bg-transparent group-data-[variant=pill]/tabs-list:data-active:text-foreground group-data-[variant=pill]/tabs-list:data-active:shadow-none dark:group-data-[variant=pill]/tabs-list:data-active:bg-transparent dark:group-data-[variant=pill]/tabs-list:data-active:text-foreground group-data-[variant=pill]/tabs-list:focus-visible:data-active:border-primary',
        'after:absolute after:bg-foreground after:opacity-0 after:transition-opacity data-horizontal:after:inset-x-0 data-horizontal:after:bottom-[-5px] data-horizontal:after:h-0.5 data-vertical:after:inset-y-0 data-vertical:after:-right-1 data-vertical:after:w-0.5 group-data-[variant=line]/tabs-list:data-active:after:opacity-100',
        className,
      )}
      {...props}
    />
  );
}

function TabsContent({ className, ...props }: TabsPrimitive.Panel.Props) {
  return (
    <TabsPrimitive.Panel
      data-slot='tabs-content'
      className={cn('flex-1 text-sm outline-none', className)}
      {...props}
    />
  );
}

export { Tabs, TabsContent, TabsList, TabsTrigger };
