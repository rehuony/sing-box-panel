import { useCallback, useEffect, useRef, useState } from 'react';

import type { ApiClient, SubscriptionToken } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

// Owned by the subscriptions page, so tab panels can discard dialogs and secrets
// without discarding metadata or restarting an in-flight list request.
export function useSubscriptionTokenPage(active: boolean) {
  const client = useApiClient();
  const [items, setItems] = useState<SubscriptionToken[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [reload, setReload] = useState(0);
  const [completed, setCompleted] = useState<{ key: string; error: unknown } | null>(null);
  const requestKey = `${pageSize}:${page}:${reload}`;
  const requestRef = useRef<{ client: ApiClient; key: string; controller: AbortController } | null>(null);

  useEffect(() => {
    const previous = requestRef.current;
    if (!active) return;
    if (previous?.client === client && previous.key === requestKey && !previous.controller.signal.aborted) return;
    previous?.controller.abort();
    const controller = new AbortController();
    requestRef.current = { client, key: requestKey, controller };
    async function load() {
      // Let mount cleanup cancel discarded StrictMode requests before sending.
      await Promise.resolve();
      if (controller.signal.aborted) return;
      setCompleted(null);
      try {
        const result = await client.listSubscriptionTokens(
          { limit: pageSize, offset: (page - 1) * pageSize }, controller.signal,
        );
        if (controller.signal.aborted) return;
        setItems(result.items);
        setTotal(result.total);
        setPage(current => Math.min(current, Math.max(1, Math.ceil(result.total / pageSize))));
        setCompleted({ key: requestKey, error: null });
      } catch (error) {
        if (controller.signal.aborted) return;
        requestRef.current = null;
        setCompleted({ key: requestKey, error });
      }
    }
    void load();
    // Deactivating a tab deliberately keeps this request alive. Only changing
    // its query/client or leaving the owning page cancels it.
  }, [active, client, page, pageSize, requestKey]);

  useEffect(() => () => requestRef.current?.controller.abort(), [client]);

  const refresh = useCallback(() => {
    setPage(1);
    setReload(value => value + 1);
  }, []);
  const changePageSize = useCallback((value: number) => {
    setPageSize(value);
    setPage(1);
  }, []);
  const loading = completed?.key !== requestKey;
  return {
    items, page, pageSize, loading,
    total,
    pages: Math.max(1, Math.ceil(total / pageSize)),
    error: loading ? null : completed?.error,
    setPage,
    setPageSize: changePageSize,
    refresh,
  };
}
