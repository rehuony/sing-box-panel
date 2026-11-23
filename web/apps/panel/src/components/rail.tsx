import type { ComponentProps, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

/**
 * A side list on its own card: an optional top row, then items that scroll
 * inside it, so the rail keeps its size whatever it holds.
 */
export function Rail({ children, className, label, top }: {
  children: ReactNode;
  label: string;
  /** A control above the items, such as a picker the list depends on. */
  top?: ReactNode;
  className?: string;
}) {
  return (
    <nav
      aria-label={label}
      className={cn('flex min-h-0 min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]', className)}
    >
      {top ? <div className='flex-none border-b p-1.5'>{top}</div> : null}
      <ul className='flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto overscroll-contain p-1.5'>{children}</ul>
    </nav>
  );
}

/** A heading between a rail's items, with an optional count or action at its end. */
export function RailHeading({ children, end }: { children: ReactNode; end?: ReactNode }) {
  return (
    <li className='mt-1.5 flex h-7 flex-none items-center justify-between border-t ps-2.5 pe-1.5 pt-1.5 text-xs text-[var(--color-text-secondary)] first:mt-0 first:border-t-0 first:pt-0'>
      <span className='truncate'>{children}</span>
      {end}
    </li>
  );
}

/** A count at the end of a rail item, in the data face. */
export function RailCount({ children }: { children: ReactNode }) {
  return <span className='font-[family-name:var(--font-data)] text-xs text-[var(--color-text-faint)] tabular-nums'>{children}</span>;
}

/**
 * One rail item: an icon, its name in full and what it counts, with an
 * optional second line. With one, the icon sits in a tile beside both lines
 * and the count centres on them. Its actions take the count's place while the
 * pointer or focus is on the item, so nothing moves and the name keeps its
 * width.
 */
export function RailItem({ actions, current, details, end, icon, marker, name, sub, ...props }: Omit<ComponentProps<'button'>, 'children'> & {
  name: string;
  icon?: ReactNode;
  /** Whether the item is the one shown. */
  current: boolean;
  /** A mark after the name, such as a warning dot. */
  marker?: ReactNode;
  /** What the item counts, at its end. */
  end?: ReactNode;
  /** A second line of data. */
  sub?: ReactNode;
  /** Icon actions that replace the end while the item is hovered or focused. */
  actions?: ReactNode;
  /** The item's details, in a tooltip beside it. */
  details?: ReactNode;
}) {
  const endSlot = (
    <span className={cn('flex h-6 flex-none items-center justify-end', actions && 'w-12 group-focus-within/rail-item:invisible group-hover/rail-item:invisible [@media(hover:none)]:invisible')}>
      {end}
    </span>
  );
  const button = (
    <button
      {...props}
      aria-current={current || undefined}
      className={cn(
        'w-full min-w-0 cursor-pointer rounded-[var(--radius-control)] pe-1.5 text-left text-sm outline-none transition-colors duration-[var(--dur-micro)] ease-out hover:bg-[var(--color-hover)] focus-visible:ring-1 focus-visible:ring-primary focus-visible:ring-inset aria-[current=true]:bg-accent',
        sub ? cn('grid h-13 items-center gap-x-2.5 ps-2', icon ? 'grid-cols-[2rem_minmax(0,1fr)_auto]' : 'grid-cols-[minmax(0,1fr)_auto]') : 'flex h-10 items-center gap-2 ps-2.5',
      )}
      type='button'
    >
      {sub
        ? (
            <>
              {icon
                ? (
                    <span aria-hidden='true' className='flex size-8 items-center justify-center rounded-[var(--radius-control)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-4'>
                      {icon}
                    </span>
                  )
                : null}
              <span className='flex min-w-0 flex-col gap-0.5'>
                <span className='flex min-w-0 items-center gap-1.5'>
                  <span className='min-w-0 truncate font-medium'>{name}</span>
                  {marker}
                </span>
                <span className='truncate text-xs text-[var(--color-text-secondary)]'>{sub}</span>
              </span>
              {endSlot}
            </>
          )
        : (
            <>
              {icon ? <span aria-hidden='true' className='flex flex-none text-muted-foreground [&_svg]:size-4'>{icon}</span> : null}
              <span className='min-w-0 flex-1 truncate font-medium'>{name}</span>
              {marker}
              {endSlot}
            </>
          )}
    </button>
  );
  return (
    <li className='group/rail-item relative flex-none'>
      {details
        ? (
            <Tooltip>
              <TooltipTrigger render={button} />
              <TooltipContent className='max-w-80' side='right'>{details}</TooltipContent>
            </Tooltip>
          )
        : button}
      {actions
        ? (
            // Without hover, as on touch screens, the actions stay in the count's place.
            <span className={cn('invisible absolute end-1.5 flex h-6 items-center group-focus-within/rail-item:visible group-hover/rail-item:visible [@media(hover:none)]:visible', sub ? 'top-3.5' : 'top-2')}>
              {actions}
            </span>
          )
        : null}
    </li>
  );
}
