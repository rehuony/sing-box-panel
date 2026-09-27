import type { ReactNode } from 'react';

import { useEffect, useMemo } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';

import type { ApiClient } from '../api-client';

import { ApiClientContext } from './api-client-context';
import { createQueryClient, resetQuerySession, withQueryInvalidation } from '../query-client';

export interface ApiClientProviderProps {
  client: ApiClient;
  children: ReactNode;
}

export function ApiClientProvider({ children, client }: ApiClientProviderProps) {
  const cache = useMemo(createQueryClient, [client]);
  const api = useMemo(() => withQueryInvalidation(client, cache), [client, cache]);
  useEffect(() => {
    const unsubscribe = client.subscribeSessionInvalidated(() => resetQuerySession(cache));
    return () => {
      unsubscribe();
      resetQuerySession(cache);
    };
  }, [client, cache]);
  return (
    <QueryClientProvider client={cache}>
      <ApiClientContext value={api}>{children}</ApiClientContext>
    </QueryClientProvider>
  );
}
