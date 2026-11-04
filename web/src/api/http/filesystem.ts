import type { HttpApiContext } from './shared';
import type { ApiClient, FilesystemPage, FilesystemSelection } from '../api-client';

export function createFilesystemHttpApi({ fetcher, nodeUrl, request, buildQuery }: HttpApiContext) {
  return {
    listFilesystemEntries(query = {}, signal) {
      return request<FilesystemPage>(fetcher, `${nodeUrl()}/filesystem/entries${buildQuery(query)}`, { method: 'GET', signal });
    },
    resolveFilesystemPath(input, signal) {
      return request<FilesystemSelection>(fetcher, `${nodeUrl()}/filesystem/resolve${buildQuery(input)}`, { method: 'GET', signal });
    },
  } satisfies Pick<ApiClient, 'listFilesystemEntries' | 'resolveFilesystemPath'>;
}
