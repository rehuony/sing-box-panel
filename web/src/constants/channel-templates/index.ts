import type { SubscriptionFormat } from '@/api/api-client';

import loon from '../../../../api/templates/loon.conf?raw';
import mihomo from '../../../../api/templates/mihomo.yaml?raw';
import singBox from '../../../../api/templates/sing-box.json?raw';

// Shared with the server: missing templates use these complete bases.
export const channelTemplateDefaults: Record<SubscriptionFormat, string> = {
  'sing-box': singBox,
  mihomo,
  loon,
};
