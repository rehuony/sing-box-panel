import type { HttpApiContext } from './shared';
import type {
  ApiClient,
  FleetCommandEvent,
  FleetCommandResult,
  FleetCommandSummary,
  FleetNode,
  FleetStatus,
  FleetStreamFrame,
  NodeEnrollment,
} from '../api-client';

import { ApiRequestError } from '../api-client';
import { readJSONEvents, readNamedJSONEvents } from './event-stream';

/** A fleet stream's first frame carries every node, so it may be larger than a node's frames. */
const fleetFrameByteLimit = 16 * 1_048_576;

export function createFleetHttpApi(context: HttpApiContext) {
  const { baseUrl, fetcher, openEventStream, request, writeHeaders, writeJSONHeaders } = context;
  const nodeUrl = (nodeID: string) => `${baseUrl}/nodes/${encodeURIComponent(nodeID)}`;
  return {
    listFleetNodes(signal) {
      return request<{ items: FleetNode[] }>(fetcher, `${baseUrl}/nodes`, { method: 'GET', signal });
    },
    getFleetStatus(signal) {
      return request<FleetStatus>(fetcher, `${baseUrl}/fleet/status`, { method: 'GET', signal });
    },
    async* streamFleetStatus(signal) {
      const response = await openEventStream(fetcher, `${baseUrl}/fleet/stream`, { method: 'GET', signal });
      let receivedSnapshot = false;
      for await (const frame of readJSONEvents<FleetStreamFrame>(response, 'fleet', fleetFrameByteLimit)) {
        receivedSnapshot = true;
        yield frame;
      }
      if (!receivedSnapshot && !signal?.aborted) {
        throw new ApiRequestError('The fleet stream closed before its initial snapshot.', { code: 'stream_invalid', status: 200 });
      }
    },
    async* runFleetCommand(command, signal) {
      const response = await openEventStream(fetcher, `${baseUrl}/fleet/commands`, {
        method: 'POST', body: JSON.stringify(command), headers: writeJSONHeaders(), signal,
      });
      const events = readNamedJSONEvents<FleetCommandResult | FleetCommandSummary>(response, ['result', 'done']);
      for await (const { data, event } of events) {
        yield (event === 'done' ? { done: data as FleetCommandSummary } : { result: data as FleetCommandResult }) satisfies FleetCommandEvent;
      }
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
  } satisfies Pick<ApiClient, 'listFleetNodes' | 'getFleetStatus' | 'streamFleetStatus' | 'runFleetCommand' | 'createNode'
  | 'renameNode' | 'deleteNode' | 'createNodeEnrollment'>;
}
