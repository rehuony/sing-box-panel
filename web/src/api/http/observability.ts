import type { HttpApiContext } from './shared';
import type { ApiClient, CoreLogChunk, CoreLogFile,
  DashboardStreamSnapshot,
  LogClearFilter,
  LogEntry,
  LogFilter,
  LogPage,
  LogStreamEvent,
  MetricsHistory,
  MetricsSnapshot,
  PanelLogPage,
  TrafficPeriod,
  TrafficPeriodFilter,
  TrafficPeriodPage } from '../api-client';

import { ApiRequestError } from '../api-client';
import { readJSONEvents } from './event-stream';

function invalidLogStream(detail: string): ApiRequestError {
  return new ApiRequestError(detail, { code: 'log_stream_invalid', status: 200 });
}

function parseLogEvent(frame: string): LogStreamEvent | undefined {
  let eventType = 'message';
  let eventID = '';
  const data: string[] = [];

  for (const line of frame.split(/\r\n|\r|\n/)) {
    if (line === '' || line.startsWith(':')) continue;
    const separator = line.indexOf(':');
    const field = separator < 0 ? line : line.slice(0, separator);
    let value = separator < 0 ? '' : line.slice(separator + 1);
    if (value.startsWith(' ')) value = value.slice(1);

    if (field === 'event') eventType = value;
    else if (field === 'id') eventID = value;
    else if (field === 'data') data.push(value);
  }

  if (eventType !== 'log' || data.length === 0) return undefined;
  if (eventID === '') throw invalidLogStream('A durable log event did not include its cursor.');

  let entry: unknown;
  try {
    entry = JSON.parse(data.join('\n'));
  } catch {
    throw invalidLogStream('A durable log event contained invalid JSON.');
  }
  if (entry === null || Array.isArray(entry) || typeof entry !== 'object') {
    throw invalidLogStream('A durable log event did not contain a log entry object.');
  }
  return { id: eventID, entry: entry as LogEntry };
}

async function* readLogEvents(response: Response): AsyncGenerator<LogStreamEvent> {
  const contentType = response.headers.get('Content-Type')?.split(';', 1)[0].trim().toLowerCase();
  if (contentType !== 'text/event-stream') {
    throw invalidLogStream('The durable log endpoint did not return an event stream.');
  }
  if (response.body === null) throw invalidLogStream('The durable log response did not include a stream body.');

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  const boundaryPattern = /(?:\r\n|\r|\n){2}/;

  try {
    while (true) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });

      let boundary = boundaryPattern.exec(buffer);
      while (boundary !== null) {
        const frame = buffer.slice(0, boundary.index);
        buffer = buffer.slice(boundary.index + boundary[0].length);
        const event = parseLogEvent(frame);
        if (event !== undefined) yield event;
        boundary = boundaryPattern.exec(buffer);
      }
      if (done) return;
    }
  } finally {
    try {
      await reader.cancel();
    } catch {
      // The transport may already be closed or aborted.
    }
    reader.releaseLock();
  }
}

export function createObservabilityHttpApi(context: HttpApiContext) {
  const { baseUrl, buildQuery, fetcher, openEventStream, request, writeHeaders } = context;
  return {
    listCoreLogFiles(signal) {
      return request<{ items: CoreLogFile[] }>(fetcher, `${baseUrl}/core/logs/files`, {
        method: 'GET',
        signal,
      });
    },
    readCoreLog(file, offset = -1, signal) {
      return request<CoreLogChunk>(
        fetcher,
        `${baseUrl}/core/logs/content${buildQuery({ file, offset })}`,
        { method: 'GET', signal },
      );
    },
    async* streamCoreLog(file, offset = -1, signal) {
      const response = await openEventStream(
        fetcher,
        `${baseUrl}/core/logs/stream${buildQuery({ file, offset })}`,
        { method: 'GET', signal },
      );
      yield* readJSONEvents<CoreLogChunk>(response, 'output');
    },
    listPanelLogs(filter = {}, signal) {
      return request<PanelLogPage>(
        fetcher,
        `${baseUrl}/logs/panel${buildQuery({ before_time: filter.beforeTime, before_id: filter.beforeID, limit: filter.limit ?? 10, search: filter.search, level: filter.level, since: filter.since, until: filter.until })}`,
        { method: 'GET', signal },
      );
    },
    listLogs(filter: LogFilter = {}, signal) {
      const query = buildQuery({
        source: filter.source,
        level: filter.level,
        code: filter.code,
        since: filter.since,
        until: filter.until,
        limit: filter.limit ?? 50,
        after_time: filter.afterTime,
        after_id: filter.afterID,
      });
      return request<LogPage>(fetcher, `${baseUrl}/logs${query}`, {
        method: 'GET',
        signal,
      });
    },
    async* streamLogs(filter = {}, signal) {
      const query = buildQuery({
        source: filter.source,
        level: filter.level,
        code: filter.code,
        since: filter.since,
        after_time: filter.afterTime,
        after_id: filter.afterID,
        limit: filter.limit ?? 50,
      });
      const response = await openEventStream(fetcher, `${baseUrl}/logs/stream${query}`, {
        method: 'GET',
        headers:
          filter.lastEventID === undefined ? undefined : { 'Last-Event-ID': filter.lastEventID },
        signal,
      });
      yield* readLogEvents(response);
    },
    getLog(entryID, signal) {
      return request<LogEntry>(fetcher, `${baseUrl}/logs/${encodeURIComponent(entryID)}`, {
        method: 'GET',
        signal,
      });
    },
    async* streamMetrics(signal) {
      const response = await openEventStream(fetcher, `${baseUrl}/metrics/stream`, {
        method: 'GET',
        signal,
      });
      yield* readJSONEvents<{
        metrics: MetricsSnapshot;
        runtime: import('../api-client').RuntimeStatus;
      }>(response, 'metrics');
    },
    async* streamDashboard(signal) {
      const response = await openEventStream(fetcher, `${baseUrl}/dashboard/stream`, {
        method: 'GET',
        signal,
      });
      yield* readJSONEvents<DashboardStreamSnapshot>(response, 'dashboard');
    },
    getMetrics(signal) {
      return request<MetricsSnapshot>(fetcher, `${baseUrl}/metrics`, {
        method: 'GET',
        signal,
      });
    },
    getMetricsHistory(filter, signal) {
      const query = buildQuery({
        from: filter.from,
        to: filter.to,
        bucket_seconds: filter.bucketSeconds,
        activation_bundle_id: filter.activationBundleID,
      });
      return request<MetricsHistory>(fetcher, `${baseUrl}/metrics/history${query}`, {
        method: 'GET',
        signal,
      });
    },
    getTrafficStatus(signal) {
      return request<MetricsSnapshot>(fetcher, `${baseUrl}/traffic/status`, {
        method: 'GET',
        signal,
      });
    },
    listTrafficPeriods(filter: TrafficPeriodFilter = {}, signal) {
      const query = buildQuery({
        activation_bundle_id: filter.activationBundleID,
        from: filter.from,
        to: filter.to,
        before_time: filter.beforeTime,
        before_id: filter.beforeID,
        limit: filter.limit ?? 50,
      });
      return request<TrafficPeriodPage>(fetcher, `${baseUrl}/traffic/periods${query}`, {
        method: 'GET',
        signal,
      });
    },
    getTrafficPeriod(periodID, signal) {
      return request<TrafficPeriod>(
        fetcher,
        `${baseUrl}/traffic/periods/${encodeURIComponent(periodID)}`,
        { method: 'GET', signal },
      );
    },
    clearLogs(filter: LogClearFilter = {}, signal) {
      const query = buildQuery({ before: filter.before, source: filter.source });
      return request<{ deleted: number }>(fetcher, `${baseUrl}/logs${query}`, {
        method: 'DELETE',
        headers: writeHeaders(),
        signal,
      });
    },
    deleteLog(entryID, signal) {
      return request<{ id: string; deleted: true }>(
        fetcher,
        `${baseUrl}/logs/${encodeURIComponent(entryID)}`,
        { method: 'DELETE', headers: writeHeaders(), signal },
      );
    },
  } satisfies Partial<ApiClient>;
}
