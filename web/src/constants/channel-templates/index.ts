import type { SubscriptionFormat } from '@/api/api-client';

import loon from '../../../../internal/sing-box-panel/subscription/templates/loon.conf?raw';
import mihomo from '../../../../internal/sing-box-panel/subscription/templates/mihomo.yaml?raw';
import singBox from '../../../../internal/sing-box-panel/subscription/templates/sing-box.json?raw';

// Shared with the server: missing templates use these complete bases.
export const channelTemplateDefaults: Record<SubscriptionFormat, string> = {
  'sing-box': singBox,
  mihomo,
  loon,
};
