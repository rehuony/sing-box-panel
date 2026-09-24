import type { ApiClient } from './api-client';
import type { HttpApiOptions } from './http/shared';

import { createCoreHttpApi } from './http/core';
import { createHttpApiContext } from './http/shared';
import { createSessionHttpApi } from './http/session';
import { createFilesystemHttpApi } from './http/filesystem';
import { createSubscriptionHttpApi } from './http/subscription';
import { createObservabilityHttpApi } from './http/observability';
import { createPanelSettingsHttpApi } from './http/panel-settings';
import { createConfigurationFileHttpApi } from './http/configuration-file';

export interface HttpApiClientOptions extends HttpApiOptions {}

export function createHttpApiClient(options: HttpApiClientOptions = {}): ApiClient {
  const context = createHttpApiContext(options);
  return {
    ...createSessionHttpApi(context),
    ...createFilesystemHttpApi(context),
    ...createPanelSettingsHttpApi(context),
    ...createConfigurationFileHttpApi(context),
    ...createCoreHttpApi(context),
    ...createSubscriptionHttpApi(context),
    ...createObservabilityHttpApi(context),
  } as ApiClient;
}
