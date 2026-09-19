import type { HttpApiContext } from './shared';
import type { ApiClient, ConfigurationFile, JsonObject } from '../api-client';

export function createConfigurationFileHttpApi(context: HttpApiContext) {
  const { baseUrl, fetcher, request, writeJSONHeaders } = context;
  return {
    newInboundDefaults(type, signal) {
      return request<JsonObject>(fetcher, `${baseUrl}/config/inbound-defaults`, {
        method: 'POST',
        signal,
        headers: writeJSONHeaders(),
        body: JSON.stringify({ type }),
      });
    },
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
    'getConfigurationFile' | 'saveConfigurationFile' | 'newInboundDefaults'
  >;
}
