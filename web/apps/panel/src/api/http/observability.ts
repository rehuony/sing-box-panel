import type { HttpApiContext } from './shared';
import type { ApiClient, CoreLogChunk, CoreLogFile,
  DashboardStreamSnapshot,
  MetricsSnapshot,
  NodeSettings,
  PanelLogPage } from '../api-client';

import { ApiRequestError } from '../api-client';
import { readJSONEvents } from './event-stream';

export function createObservabilityHttpApi(context: HttpApiContext) {
  const { baseUrl, buildQuery, fetcher, nodeUrl, openEventStream, request, writeHeaders, writeJSONHeaders } = context;
  return {
    listCoreLogFiles(signal) {
      return request<{ items: CoreLogFile[] }>(fetcher, `${nodeUrl()}/core/logs/files`, {
        method: 'GET',
        signal,
      });
    },
    deleteCoreLogFile(file, signal) {
      return request<void>(fetcher, `${nodeUrl()}/core/logs/files${buildQuery({ file })}`, {
        method: 'DELETE',
        headers: writeHeaders(),
        signal,
      });
    },
    clearCoreLog(file, signal) {
      return request<void>(fetcher, `${nodeUrl()}/core/logs/content${buildQuery({ file })}`, {
        method: 'DELETE',
        signal,
        headers: writeHeaders(),
      });
    },
    readCoreLog(file, offset = -1, generation, signal) {
      return request<CoreLogChunk>(
        fetcher,
        `${nodeUrl()}/core/logs/content${buildQuery({ file, offset, generation: generation || undefined })}`,
        { method: 'GET', signal },
      );
    },
    async* streamCoreLog(file, offset = -1, generation, signal) {
      const response = await openEventStream(
        fetcher,
        `${nodeUrl()}/core/logs/stream${buildQuery({ file, offset, generation: generation || undefined })}`,
        { method: 'GET', signal },
      );
      yield* readJSONEvents<CoreLogChunk>(response, 'output');
    },
    listPanelLogs(filter = {}, signal) {
      return request<PanelLogPage>(
        fetcher,
        `${baseUrl}/logs/panel${buildQuery({ offset: filter.offset, before_time: filter.beforeTime, before_id: filter.beforeID, limit: filter.limit ?? 10, search: filter.search, search_codes: filter.searchCodes?.length ? filter.searchCodes.join(',') : undefined, level: filter.level, since: filter.since, until: filter.until })}`,
        { method: 'GET', signal },
      );
    },
    async* streamMetrics(signal) {
      const response = await openEventStream(fetcher, `${nodeUrl()}/metrics/stream`, {
        method: 'GET',
        signal,
      });
      let receivedSnapshot = false;
      for await (const snapshot of readJSONEvents<{
        metrics: MetricsSnapshot;
        runtime: import('../api-client').RuntimeStatus;
      }>(response, 'metrics')) {
        receivedSnapshot = true;
        yield snapshot;
      }
      if (!receivedSnapshot && !signal?.aborted) {
        throw new ApiRequestError('The metrics stream closed before its initial snapshot.', {
          code: 'stream_invalid',
          status: 200,
        });
      }
    },
    async* streamDashboard(signal) {
      const response = await openEventStream(fetcher, `${nodeUrl()}/dashboard/stream`, {
        method: 'GET',
        signal,
      });
      let receivedSnapshot = false;
      for await (const snapshot of readJSONEvents<DashboardStreamSnapshot>(response, 'dashboard')) {
        receivedSnapshot = true;
        yield snapshot;
      }
      if (!receivedSnapshot && !signal?.aborted) {
        throw new ApiRequestError('The dashboard stream closed before its initial snapshot.', {
          code: 'stream_invalid',
          status: 200,
        });
      }
    },
    getMetrics(signal) {
      return request<MetricsSnapshot>(fetcher, `${nodeUrl()}/metrics`, {
        method: 'GET',
        signal,
      });
    },
    getNodeSettings(signal) {
      return request<NodeSettings>(fetcher, `${nodeUrl()}/settings`, { method: 'GET', signal });
    },
    saveNodeSettings(input, signal) {
      return request<NodeSettings>(fetcher, `${nodeUrl()}/settings`, {
        method: 'PUT',
        signal,
        headers: writeJSONHeaders(),
        body: JSON.stringify(input),
      });
    },
  } satisfies Partial<ApiClient>;
}
