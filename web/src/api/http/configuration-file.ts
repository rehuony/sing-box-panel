import type { HttpApiContext } from './shared';
import type { ApiClient, ConfigurationFile } from '../api-client';

export function createConfigurationFileHttpApi(context: HttpApiContext) {
  const { baseUrl, fetcher, request, writeJSONHeaders } = context;
  return {
    getConfigurationFile(signal) {
      return request<ConfigurationFile>(fetcher, `${baseUrl}/config/file`, {
        method: 'GET',
        signal,
      });
    },
    saveConfigurationFile(input, signal) {
      return request<ConfigurationFile>(fetcher, `${baseUrl}/config/file`, {
        method: 'PUT',
        signal,
        headers: writeJSONHeaders(),
        body: JSON.stringify(input),
      });
    },
  } satisfies Pick<
    ApiClient,
    'getConfigurationFile' | 'saveConfigurationFile'
  >;
}
