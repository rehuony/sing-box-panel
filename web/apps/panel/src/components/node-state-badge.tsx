import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';

import type { FleetNode } from '@/api/api-client';

const variants = {
  online: 'success',
  offline: 'destructive',
  pending: 'secondary',
} as const satisfies Record<FleetNode['state'], string>;

/** The node's connection with the panel: online, offline, or not yet enrolled. */
export function NodeStateBadge({ state }: { state: FleetNode['state'] }) {
  const { t } = useTranslation();
  return (
    <Badge variant={variants[state]}>
      <span aria-hidden='true' className='size-1.5 rounded-full bg-current' />
      {t(`nodes.state.${state}`)}
    </Badge>
  );
}
