import type { HttpApiContext } from './shared';
import type { ApiClient, PanelBackup, PanelRestoreResult, PanelSettingsView } from '../api-client';

export function createPanelSettingsHttpApi(context: HttpApiContext) {
  const { baseUrl, fetcher, request, writeJSONHeaders } = context;
  return {
    exportPanelBackup(signal) {
      return request<PanelBackup>(fetcher, `${baseUrl}/panel/backup`, { method: 'GET', signal });
    },
    restorePanelBackup(input, signal) {
      return request<PanelRestoreResult>(fetcher, `${baseUrl}/panel/restore`, {
        method: 'POST', signal, headers: writeJSONHeaders(), body: JSON.stringify(input),
      });
    },
    getPanelSettings(signal) {
      return request<PanelSettingsView>(fetcher, `${baseUrl}/panel/settings`, { method: 'GET', signal });
    },
    savePanelSettings(input, signal) {
      return request<PanelSettingsView>(fetcher, `${baseUrl}/panel/settings`, {
        method: 'PUT', signal, headers: writeJSONHeaders(), body: JSON.stringify(input),
      });
    },
  } satisfies Pick<ApiClient, 'getPanelSettings' | 'savePanelSettings' | 'exportPanelBackup' | 'restorePanelBackup'>;
}
