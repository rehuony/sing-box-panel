import type { ComponentProps, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Button } from '@shared/components/ui/button';

/**
 * A bar's action, in a title strip, a card's toolbar or a card's title bar:
 * a flat compact button with its icon and label. On a phone it keeps only
 * its icon, square, the label still naming it.
 */
export function StripAction({ className, icon, label, ...props }: Omit<ComponentProps<typeof Button>, 'children' | 'size' | 'variant'> & {
  /** The icon, marked `aria-hidden` and `data-icon='inline-start'`. */
  icon: ReactNode;
  label: string;
}) {
  return (
    <Button className={cn('max-sm:w-(--control-height-sm) max-sm:px-0', className)} size='sm' type='button' variant='flat' {...props}>
      {icon}
      <span className='max-sm:sr-only'>{label}</span>
    </Button>
  );
}
