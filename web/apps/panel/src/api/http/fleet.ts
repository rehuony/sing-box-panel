import type { HttpApiContext } from './shared';
import type { ApiClient, FleetNode } from '../api-client';

export function createFleetHttpApi({ baseUrl, fetcher, request }: HttpApiContext) {
  return {
    listFleetNodes(signal) {
      return request<{ items: FleetNode[] }>(fetcher, `${baseUrl}/nodes`, { method: 'GET', signal });
    },
  } satisfies Pick<ApiClient, 'listFleetNodes'>;
}
