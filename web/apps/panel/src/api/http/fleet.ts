import type { HttpApiContext } from './shared';
import type { ApiClient, FleetNode, NodeEnrollment } from '../api-client';

export function createFleetHttpApi({ baseUrl, fetcher, request, writeHeaders, writeJSONHeaders }: HttpApiContext) {
  const nodeUrl = (nodeID: string) => `${baseUrl}/nodes/${encodeURIComponent(nodeID)}`;
  return {
    listFleetNodes(signal) {
      return request<{ items: FleetNode[] }>(fetcher, `${baseUrl}/nodes`, { method: 'GET', signal });
    },
    createNode(name, signal) {
      return request<NodeEnrollment>(fetcher, `${baseUrl}/nodes`, {
        method: 'POST', body: JSON.stringify({ name }), headers: writeJSONHeaders(), signal,
      });
    },
    renameNode(nodeID, name, signal) {
      return request<FleetNode>(fetcher, nodeUrl(nodeID), {
        method: 'PUT', body: JSON.stringify({ name }), headers: writeJSONHeaders(), signal,
      });
    },
    deleteNode(nodeID, signal) {
      return request<void>(fetcher, nodeUrl(nodeID), { method: 'DELETE', headers: writeHeaders(), signal });
    },
    createNodeEnrollment(nodeID, signal) {
      return request<NodeEnrollment>(fetcher, `${nodeUrl(nodeID)}/enrollment`, {
        method: 'POST', headers: writeHeaders(), signal,
      });
    },
  } satisfies Pick<ApiClient, 'listFleetNodes' | 'createNode' | 'renameNode' | 'deleteNode' | 'createNodeEnrollment'>;
}
