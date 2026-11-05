import { createContext, use } from 'react';

import type { FleetNode, SystemStatus } from '@/api/api-client';

import i18n from '@/i18n';

/** The node whose workspace is open; its views read and command only this node. */
export const CurrentNodeContext = createContext<FleetNode | null>(null);

export function useCurrentNode(): FleetNode {
  const node = use(CurrentNodeContext);
  if (node === null) {
    throw new Error(i18n.t('common.providerRequired', { hook: 'useCurrentNode', provider: 'CurrentNodeContext' }));
  }
  return node;
}

/** The platform the node reported when it connected, which its cores must match. */
export function nodePlatform(node: FleetNode | null): SystemStatus['platform'] | undefined {
  return node && node.os !== '' && node.arch !== '' ? { os: node.os, arch: node.arch } : undefined;
}
