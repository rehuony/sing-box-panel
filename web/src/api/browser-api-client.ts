import type { ApiClient } from './api-client';

import { createHttpApiClient } from './http-api-client';

export async function createBrowserApiClient(basePath: string): Promise<ApiClient> {
  if (import.meta.env.MODE === 'demo') {
    const { createDemoApiClient } = await import('@/api/demo/create-demo-api-client');
    return createDemoApiClient();
  }

  return createHttpApiClient({ baseUrl: `${basePath}/api/v1` });
}
