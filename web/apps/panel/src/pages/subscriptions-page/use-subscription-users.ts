import { useCallback, useEffect, useRef, useState } from 'react';

import type { SubscriptionNodeCatalog, SubscriptionUser } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

import { loadSubscriptionUsers } from './load-subscription-users';

export interface SubscriptionUsersState {
  error: unknown;
  catalogError: unknown;
  users: SubscriptionUser[] | null;
  catalog: SubscriptionNodeCatalog | null;
  reload: (signal?: AbortSignal) => Promise<void>;
  reloadCatalog: (signal?: AbortSignal) => Promise<void>;
}

export function useSubscriptionUsers(): SubscriptionUsersState {
  const client = useApiClient();
  const [users, setUsers] = useState<SubscriptionUser[] | null>(null);
  const [catalog, setCatalog] = useState<SubscriptionNodeCatalog | null>(null);
  const [catalogError, setCatalogError] = useState<unknown>(null);
  const [error, setError] = useState<unknown>(null);
  const generationRef = useRef(0);
  const catalogGenerationRef = useRef(0);

  const reload = useCallback(async (signal?: AbortSignal) => {
    const generation = ++generationRef.current;
    setError(null);
    setUsers(null);
    try {
      const loadedUsers = await loadSubscriptionUsers(client, signal);
      if (!signal?.aborted && generation === generationRef.current) setUsers(loadedUsers);
    } catch (loadError) {
      if (!signal?.aborted && generation === generationRef.current) {
        setUsers(null);
        setError(loadError);
      }
    }
  }, [client]);

  const reloadCatalog = useCallback(async (signal?: AbortSignal) => {
    const generation = ++catalogGenerationRef.current;
    setCatalogError(null);
    setCatalog(null);
    try {
      const loadedCatalog = await client.getSubscriptionNodeCatalog(signal);
      if (!signal?.aborted && generation === catalogGenerationRef.current) setCatalog(loadedCatalog);
    } catch (loadError) {
      if (!signal?.aborted && generation === catalogGenerationRef.current) {
        setCatalog(null);
        setCatalogError(loadError);
      }
    }
  }, [client]);

  useEffect(() => {
    const usersController = new AbortController();
    const catalogController = new AbortController();
    void reload(usersController.signal);
    void reloadCatalog(catalogController.signal);
    return () => {
      usersController.abort();
      catalogController.abort();
    };
  }, [reload, reloadCatalog]);

  return { catalog, catalogError, error, reload, reloadCatalog, users };
}
