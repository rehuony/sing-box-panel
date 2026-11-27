import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';

import type { FleetNode } from '@/api/api-client';

const variants = {
  online: 'success',
  offline: 'destructive',
} as const satisfies Record<FleetNode['state'], string>;

/**
 * The node's connection with the panel: online or offline. Where room runs
 * short its word can leave the dot alone, still naming the state to a reader
 * and on hover.
 */
export function NodeStateBadge({ className, labelClassName, state }: {
  state: FleetNode['state'];
  className?: string;
  labelClassName?: string;
}) {
  const { t } = useTranslation();
  const label = t(`nodes.state.${state}`);
  return (
    <Badge className={className} title={label} variant={variants[state]}>
      <span aria-hidden='true' className='size-1.5 flex-none rounded-full bg-current' />
      <span className={labelClassName}>{label}</span>
    </Badge>
  );
}
