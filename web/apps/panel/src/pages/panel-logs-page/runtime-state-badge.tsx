import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';

import type { RuntimeTransitionState } from '@/api/api-client';

const variants = {
  failed: 'destructive',
  running: 'success',
  stopped: 'secondary',
  unknown: 'secondary',
} as const satisfies Record<RuntimeTransitionState, string>;

/** A runtime transition's state in the state colours: running success, failed danger, stopped and unknown muted. */
export function RuntimeStateBadge({ state }: { state: RuntimeTransitionState }) {
  const { t } = useTranslation();
  return <Badge variant={variants[state]}>{t(`productLogs.states.${state}`)}</Badge>;
}
