import type { SubscriptionFormat } from '@/api/api-client';

import loon from './loon.conf?raw';
import mihomo from './mihomo.yaml?raw';
import singBox from './sing-box.json?raw';

// Seeds for the editor only. Saved configurations are always used as-is.
export const channelTemplateDefaults: Record<SubscriptionFormat, string> = {
  'sing-box': singBox,
  mihomo,
  loon,
};
