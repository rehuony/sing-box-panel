import { use } from 'react';

import type { ApiClient } from '../api-client';

import { ApiClientContext } from './api-client-context';

export function useApiClient(): ApiClient {
  const client = use(ApiClientContext);

  if (client === null) {
    throw new Error('useApiClient must be used within ApiClientProvider');
  }

  return client;
}
