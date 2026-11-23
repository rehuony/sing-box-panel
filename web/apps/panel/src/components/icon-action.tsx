import type { ComponentProps } from 'react';

import { Button } from '@shared/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

/**
 * An icon action named by its tooltip: ghost and compact in a table row or
 * embedded inside a field, flat in a title strip.
 */
export function IconAction({ children, label, size = 'icon-sm', variant = 'ghost', ...props }: Omit<ComponentProps<typeof Button>, 'aria-label' | 'size' | 'variant'> & {
  label: string;
  size?: 'icon-sm' | 'icon-xs';
  variant?: 'ghost' | 'flat';
}) {
  return (
    <Tooltip>
      <TooltipTrigger render={<Button {...props} aria-label={label} size={size} type='button' variant={variant} />}>{children}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
