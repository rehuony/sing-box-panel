import type { ComponentProps, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Button } from '@shared/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

/** Whether a bar action shows its icon alone, its label hidden but still naming it. */
function iconOnly(trigger: Element | undefined): boolean {
  const label = trigger?.querySelector<HTMLElement>('[data-slot="strip-label"]');
  return !label || label.offsetWidth <= 1;
}

/**
 * Names a bar's action in a tooltip while the action shows its icon alone;
 * while its label shows, the tooltip stays shut. It holds the action's
 * `TooltipTrigger`, whose label carries `data-slot='strip-label'`.
 */
export function StripTooltip({ children, label }: { children: ReactNode; label: string }) {
  return (
    <Tooltip onOpenChange={(open, details) => {
      if (open && !iconOnly(details.trigger)) details.cancel();
    }}>
      {children}
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * A bar's action, in a title strip, a card's toolbar or a card's title bar:
 * a flat compact button with its icon and label. On a phone, in a toolbar's
 * actions narrower than 24rem, or where its bar hides the label for room, it
 * keeps only its icon, square, the label still naming it and a tooltip
 * showing it.
 */
export function StripAction({ className, icon, label, labelClassName, ...props }: Omit<ComponentProps<typeof Button>, 'children' | 'size' | 'variant'> & {
  /** The icon, marked `aria-hidden` and `data-icon='inline-start'`. */
  icon: ReactNode;
  label: string;
  /** Hides the label where the bar runs short of room, as by a container query; pair it with a square `className`. */
  labelClassName?: string;
}) {
  return (
    <StripTooltip label={label}>
      <TooltipTrigger render={<Button className={cn('max-sm:w-(--control-height-sm) max-sm:px-0 @max-[24rem]/strip-actions:w-(--control-height-sm) @max-[24rem]/strip-actions:px-0', className)} size='sm' type='button' variant='flat' {...props} />}>
        {icon}
        <span className={cn('max-sm:sr-only @max-[24rem]/strip-actions:sr-only', labelClassName)} data-slot='strip-label'>{label}</span>
      </TooltipTrigger>
    </StripTooltip>
  );
}
