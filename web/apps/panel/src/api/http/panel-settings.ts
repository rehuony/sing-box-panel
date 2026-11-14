import type { HttpApiContext } from './shared';
import type { ApiClient, PanelBackup, PanelRestoreResult, PanelSettingsSaveResult, PanelSettingsView, SystemUpdate } from '../api-client';

export function createPanelSettingsHttpApi(context: HttpApiContext) {
  const { baseUrl, fetcher, request, writeHeaders, writeJSONHeaders } = context;
  return {
    getSystemUpdate(signal) {
      return request<SystemUpdate>(fetcher, `${baseUrl}/system/update`, { method: 'GET', signal });
    },
    checkSystemUpdate(signal) {
      return request<SystemUpdate>(fetcher, `${baseUrl}/system/update/check`, { method: 'POST', headers: writeHeaders(), signal });
    },
    startSystemUpdate(signal) {
      return request<SystemUpdate>(fetcher, `${baseUrl}/system/update`, { method: 'POST', headers: writeHeaders(), signal });
    },
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
    async savePanelSettings(input, signal) {
      const result = await request<PanelSettingsSaveResult>(fetcher, `${baseUrl}/panel/settings`, {
        method: 'PUT', signal, headers: writeJSONHeaders(), body: JSON.stringify(input),
      });
      if (result.reauthentication_required) context.invalidateSession();
      return result;
    },
  } satisfies Pick<ApiClient, 'getPanelSettings' | 'savePanelSettings' | 'exportPanelBackup' | 'restorePanelBackup'
  | 'getSystemUpdate' | 'checkSystemUpdate' | 'startSystemUpdate'>;
}
