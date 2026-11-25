import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';

/** The log window's views: the panel log, and every node's runtime states. */
export type PanelLogView = 'logs' | 'states';

/** The view the address names; the log is the default. */
export function usePanelLogView(): PanelLogView {
  const [params] = useSearchParams();
  return params.get('view') === 'states' ? 'states' : 'logs';
}

export interface NodeFilter {
  /** The node a node's overview opened the window for; empty for every node. */
  id: string;
  name: string;
  clear: () => void;
}

/** The node filter in the address, which a node's overview sets so it can be shared and cleared. */
export function useNodeFilter(): NodeFilter {
  const client = useApiClient();
  const [params, setParams] = useSearchParams();
  const id = params.get('node') ?? '';
  const name = useQuery({
    ...queries.fleetStatus(client),
    enabled: id !== '',
    select: status => status.items.find(item => item.node.id === id)?.node.name,
  }).data ?? id;
  return {
    clear: () => setParams((current) => {
      const next = new URLSearchParams(current);
      next.delete('node');
      return next;
    }, { replace: true }),
    id,
    name,
  };
}
