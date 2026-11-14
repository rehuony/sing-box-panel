import type { Prompt } from '@shared/components/terminal';

import { useLocation } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';

import type { NodeStatus } from '@/utils/fleet-summary';
import type { ShellScope } from '@/utils/shell-windows';

import { queries } from '@/api/queries';
import { scopeOf } from '@/utils/shell-windows';
import { useApiClient } from '@/api/api-client-context';

export interface ShellScopeState {
  /** The command line's prompt: `panel@fleet` or `panel@<node>`, in the current window. */
  prompt: Prompt;
  scope: ShellScope;
  /** The fleet in the panel's order, as the fleet stream keeps it. */
  items: readonly NodeStatus[];
  /** The node a node scope shows, once the fleet is read. */
  node: NodeStatus | undefined;
}

/** The scope, window and fleet the status line and the command line work in. */
export function useShellScope(): ShellScopeState {
  const location = useLocation();
  const client = useApiClient();
  const fleet = useQuery(queries.fleetStatus(client));
  const items = fleet.data?.items ?? [];
  const scope = scopeOf(location.pathname);
  const node = scope.kind === 'node' ? items.find(item => item.node.id === scope.nodeId) : undefined;
  const host = scope.kind === 'node' ? node?.node.name ?? scope.nodeId : 'fleet';
  return { items, node, prompt: { host, kind: 'shell', path: scope.window ?? '~', user: 'panel' }, scope };
}
