import type { ApiClient } from './api-client';
import type { HttpApiContext, HttpApiOptions } from './http/shared';

import { createCoreHttpApi } from './http/core';
import { createFleetHttpApi } from './http/fleet';
import { createSessionHttpApi } from './http/session';
import { createFilesystemHttpApi } from './http/filesystem';
import { createHttpApiContext, withNode } from './http/shared';
import { createSubscriptionHttpApi } from './http/subscription';
import { createObservabilityHttpApi } from './http/observability';
import { createPanelSettingsHttpApi } from './http/panel-settings';
import { createConfigurationFileHttpApi } from './http/configuration-file';

export interface HttpApiClientOptions extends HttpApiOptions {}

export function createHttpApiClient(options: HttpApiClientOptions = {}): ApiClient {
  const root = createHttpApiContext(options);
  // Every scope shares the root context's session and CSRF state.
  const build = (context: HttpApiContext, nodeID: string | null): ApiClient => ({
    supportsNativeChannelValidation: true,
    nodeID,
    forNode: (id: string) => build(withNode(root, id), id),
    ...createSessionHttpApi(context),
    ...createFleetHttpApi(context),
    ...createFilesystemHttpApi(context),
    ...createPanelSettingsHttpApi(context),
    ...createConfigurationFileHttpApi(context),
    ...createCoreHttpApi(context),
    ...createSubscriptionHttpApi(context),
    ...createObservabilityHttpApi(context),
  }) as ApiClient;
  return build(root, null);
}
