import type { ReactNode } from 'react';

import { useTranslation } from 'react-i18next';
import { useCallback, useMemo, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { withQueryInvalidation } from '@/api/query-client';
import { ApiClientContext } from '@/api/api-client-context/api-client-context';

import type { NodeSelectionValue } from './node-selection.store';

import { NodeSelectionContext } from './node-selection.store';

const storageKey = 'sing-box-panel.node';

function readStoredNode(): string | null {
  try {
    return window.localStorage.getItem(storageKey);
  } catch {
    return null;
  }
}

function writeStoredNode(nodeID: string) {
  try {
    window.localStorage.setItem(storageKey, nodeID);
  } catch {
    // The selection still applies to this page.
  }
}

/**
 * Selects the node that node-scoped pages manage. Its subtree waits for the
 * node list and is keyed by the node, so pages mount once for the selected
 * node and switching nodes discards every node's local page state.
 */
export function NodeSelectionProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const nodes = useQuery(queries.fleetNodes(client));
  const [stored, setStored] = useState(readStoredNode);
  const select = useCallback((nodeID: string) => {
    setStored(nodeID);
    writeStoredNode(nodeID);
  }, []);
  const items = useMemo(() => nodes.data?.items ?? [], [nodes.data]);
  const selected = items.find(node => node.id === stored) ?? items[0] ?? null;
  const selectedID = selected?.id ?? null;
  const scoped = useMemo(
    () => selectedID === null ? client : withQueryInvalidation(client.forNode(selectedID), cache),
    [client, cache, selectedID],
  );
  const value = useMemo<NodeSelectionValue>(() => ({
    status: nodes.status, error: nodes.error, nodes: items, selected, select,
  }), [nodes.status, nodes.error, items, selected, select]);

  if (nodes.isPending) return <LoadingState label={t('shell.loading.page')} />;
  return (
    <NodeSelectionContext value={value}>
      <ApiClientContext value={scoped} key={selectedID ?? ''}>{children}</ApiClientContext>
    </NodeSelectionContext>
  );
}
