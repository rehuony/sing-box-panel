import type { LucideIcon } from 'lucide-react';

import { Gauge, MousePointer2, Shuffle } from 'lucide-react';

import type { ChannelRuleGroup } from '@/api/api-client';

/** Each strategy's icon, the same in the channel rail and the group's settings. */
export const GROUP_ICONS: Record<ChannelRuleGroup['type'], LucideIcon> = {
  'select': MousePointer2,
  'url-test': Gauge,
  'fallback': Shuffle,
};
