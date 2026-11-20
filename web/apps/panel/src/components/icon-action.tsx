import type { ComponentProps } from 'react';

import { Button } from '@shared/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

/**
 * A ghost icon action named by its tooltip: compact in a table row, embedded
 * inside a field.
 */
export function IconAction({ children, label, size = 'icon-sm', ...props }: Omit<ComponentProps<typeof Button>, 'aria-label' | 'size' | 'variant'> & {
  label: string;
  size?: 'icon-sm' | 'icon-xs';
}) {
  return (
    <Tooltip>
      <TooltipTrigger render={<Button {...props} aria-label={label} size={size} type='button' variant='ghost' />}>{children}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
