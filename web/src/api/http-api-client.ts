import type { ApiClient } from './api-client';
import type { HttpApiOptions } from './http/shared';

import { createCoreHttpApi } from './http/core';
import { createTasksHttpApi } from './http/tasks';
import { createHttpApiContext } from './http/shared';
import { createSessionHttpApi } from './http/session';
import { createCanonicalHttpApi } from './http/canonical';
import { createSubscriptionHttpApi } from './http/subscription';
import { createObservabilityHttpApi } from './http/observability';

export interface HttpApiClientOptions extends HttpApiOptions {}

export function createHttpApiClient(options: HttpApiClientOptions = {}): ApiClient {
  const context = createHttpApiContext(options);
  return {
    ...createSessionHttpApi(context),
    ...createCanonicalHttpApi(context),
    ...createTasksHttpApi(context),
    ...createCoreHttpApi(context),
    ...createSubscriptionHttpApi(context),
    ...createObservabilityHttpApi(context),
  } as ApiClient;
}
