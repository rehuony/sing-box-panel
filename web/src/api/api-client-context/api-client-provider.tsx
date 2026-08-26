import type { ReactNode } from 'react';

import type { ApiClient } from '../api-client';

import { ApiClientContext } from './api-client-context';

export interface ApiClientProviderProps {
  client: ApiClient;
  children: ReactNode;
}

export function ApiClientProvider({ children, client }: ApiClientProviderProps) {
  return <ApiClientContext value={client}>{children}</ApiClientContext>;
}
