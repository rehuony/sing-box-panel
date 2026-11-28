import type { HttpApiContext } from './shared';
import type {
  ApiClient,
  PanelBackupInfo,
  PanelBackupList,
  PanelRestorePlan,
  PanelRestoreRequested,
  PanelRestoreStaged,
  PanelSettingsSaveResult,
  PanelSettingsView,
  SystemRestart,
  SystemStatus,
  SystemUpdate,
} from '../api-client';

import { ApiRequestError } from '../api-client';

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
    getSystemStatus(signal) {
      return request<SystemStatus>(fetcher, `${baseUrl}/system/status`, { method: 'GET', signal });
    },
    restartPanel(signal) {
      return request<SystemRestart>(fetcher, `${baseUrl}/system/restart`, { method: 'POST', headers: writeHeaders(), signal });
    },
    listPanelBackups(signal) {
      return request<PanelBackupList>(fetcher, `${baseUrl}/panel/backups`, { method: 'GET', signal });
    },
    createPanelBackup(input, signal) {
      return request<PanelBackupInfo>(fetcher, `${baseUrl}/panel/backups`, {
        method: 'POST', signal, headers: writeJSONHeaders(), body: JSON.stringify(input),
      });
    },
    panelBackupDownloadUrl(name) {
      return `${baseUrl}/panel/backups/${encodeURIComponent(name)}`;
    },
    deletePanelBackup(name, signal) {
      return request<void>(fetcher, `${baseUrl}/panel/backups/${encodeURIComponent(name)}`, { method: 'DELETE', headers: writeHeaders(), signal });
    },
    stagePanelBackup(name, input, signal) {
      return request<PanelRestoreStaged>(fetcher, `${baseUrl}/panel/backups/${encodeURIComponent(name)}/restore`, {
        method: 'POST', signal, headers: writeJSONHeaders(), body: JSON.stringify(input),
      });
    },
    uploadPanelRestore(file, passphrase, onProgress, signal) {
      // fetch reports no upload progress; a backup can take minutes to send.
      return new Promise<PanelRestoreStaged>((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        const form = new FormData();
        if (passphrase !== '') form.append('passphrase', passphrase);
        form.append('archive', file);
        xhr.open('POST', `${baseUrl}/panel/restores`);
        xhr.withCredentials = true;
        xhr.setRequestHeader('Accept', 'application/json');
        const headers = writeHeaders() as Record<string, string>;
        for (const [name, value] of Object.entries(headers)) xhr.setRequestHeader(name, value);
        xhr.upload.onprogress = event => onProgress(event.loaded, event.lengthComputable ? event.total : file.size);
        xhr.onerror = () => reject(new ApiRequestError('The upload failed.', { status: 0, code: 'network_error' }));
        xhr.onabort = () => reject(new DOMException('Upload canceled', 'AbortError'));
        xhr.onload = () => {
          let body: { code?: string; detail?: string; title?: string } | PanelRestoreStaged | undefined;
          try {
            body = JSON.parse(xhr.responseText) as typeof body;
          } catch {
            body = undefined;
          }
          if (xhr.status >= 200 && xhr.status < 300 && body) {
            resolve(body as PanelRestoreStaged);
            return;
          }
          if (xhr.status === 401) context.invalidateSession();
          const problem = (body ?? {}) as { code?: string; detail?: string; title?: string };
          reject(new ApiRequestError(problem.detail ?? problem.title ?? `Request failed with status ${xhr.status}`, {
            status: xhr.status, code: problem.code ?? 'request_failed',
          }));
        };
        signal?.addEventListener('abort', () => xhr.abort(), { once: true });
        xhr.send(form);
      });
    },
    discardPanelRestore(id, signal) {
      return request<void>(fetcher, `${baseUrl}/panel/restores/${encodeURIComponent(id)}`, { method: 'DELETE', headers: writeHeaders(), signal });
    },
    planPanelRestore(id, input, signal) {
      return request<PanelRestorePlan>(fetcher, `${baseUrl}/panel/restores/${encodeURIComponent(id)}/plan`, {
        method: 'POST', signal, headers: writeJSONHeaders(), body: JSON.stringify(input),
      });
    },
    applyPanelRestore(id, input, signal) {
      return request<PanelRestoreRequested>(fetcher, `${baseUrl}/panel/restores/${encodeURIComponent(id)}/apply`, {
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
  } satisfies Pick<ApiClient, 'getPanelSettings' | 'savePanelSettings'
  | 'listPanelBackups' | 'createPanelBackup' | 'panelBackupDownloadUrl' | 'deletePanelBackup'
  | 'stagePanelBackup' | 'uploadPanelRestore' | 'discardPanelRestore' | 'planPanelRestore' | 'applyPanelRestore'
  | 'getSystemUpdate' | 'checkSystemUpdate' | 'startSystemUpdate' | 'getSystemStatus' | 'restartPanel'>;
}
