import type { HttpApiContext } from './shared';
import type { ApiClient, PanelSettingsView } from '../api-client';

export function createPanelSettingsHttpApi(context: HttpApiContext) {
  const { baseUrl, fetcher, request, writeJSONHeaders } = context;
  return {
    getPanelSettings(signal) {
      return request<PanelSettingsView>(fetcher, `${baseUrl}/panel/settings`, { method: 'GET', signal });
    },
    savePanelSettings(input, signal) {
      return request<PanelSettingsView>(fetcher, `${baseUrl}/panel/settings`, {
        method: 'PUT', signal, headers: writeJSONHeaders(), body: JSON.stringify(input),
      });
    },
  } satisfies Pick<ApiClient, 'getPanelSettings' | 'savePanelSettings'>;
}
