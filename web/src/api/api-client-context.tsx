import { createContext, type ReactNode, useContext } from 'react';

import type { ApiClient } from './api-client';

const ApiClientContext = createContext<ApiClient | null>(null);

export interface ApiClientProviderProps {
  children: ReactNode;
  client: ApiClient;
}

export function ApiClientProvider({ children, client }: ApiClientProviderProps) {
  return (
    <ApiClientContext.Provider value={client}>
      {children}
    </ApiClientContext.Provider>
  );
}

export function useApiClient(): ApiClient {
  const client = useContext(ApiClientContext);

  if (client === null) {
    throw new Error('useApiClient must be used within ApiClientProvider');
  }

  return client;
}
