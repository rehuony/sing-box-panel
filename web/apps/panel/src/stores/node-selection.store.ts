import { createContext, use } from 'react';

import type { FleetNode, SystemStatus } from '@/api/api-client';

import i18n from '@/i18n';

export interface NodeSelectionValue {
  error: unknown;
  nodes: FleetNode[];
  /** The node that node-scoped pages manage, or null before one exists. */
  selected: FleetNode | null;
  select: (nodeID: string) => void;
  status: 'pending' | 'error' | 'success';
}

export const NodeSelectionContext = createContext<NodeSelectionValue | null>(null);

export function useNodeSelection(): NodeSelectionValue {
  const value = use(NodeSelectionContext);
  if (value === null) {
    throw new Error(i18n.t('common.providerRequired', { hook: 'useNodeSelection', provider: 'NodeSelectionProvider' }));
  }
  return value;
}

/** The platform the node reported when it connected, which its cores must match. */
export function nodePlatform(node: FleetNode | null): SystemStatus['platform'] | undefined {
  return node && node.os !== '' && node.arch !== '' ? { os: node.os, arch: node.arch } : undefined;
}
