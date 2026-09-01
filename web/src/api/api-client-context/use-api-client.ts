import { use } from 'react';

import i18n from '@/i18n';

import type { ApiClient } from '../api-client';

import { ApiClientContext } from './api-client-context';

export function useApiClient(): ApiClient {
  const client = use(ApiClientContext);

  if (client === null) {
    throw new Error(i18n.t('common.providerRequired', {
      hook: 'useApiClient',
      provider: 'ApiClientProvider',
    }));
  }

  return client;
}
