import type { HttpApiContext } from './shared';
import type { ApiClient, LogClearFilter, LogEntry, LogFilter, LogPage, MetricsSnapshot, TrafficPeriod, TrafficPeriodFilter, TrafficPeriodPage } from '../api-client';

export function createObservabilityHttpApi(context: HttpApiContext) {
  const {
    baseUrl, buildQuery, fetcher, request, writeHeaders,
  } = context;
  return {
    listLogs(filter: LogFilter = {}, signal) {
      const query = buildQuery({
        source: filter.source,
        level: filter.level,
        since: filter.since,
        limit: filter.limit ?? 50,
        after_time: filter.afterTime,
        after_id: filter.afterID,
      });
      return request<LogPage>(fetcher, `${baseUrl}/logs${query}`, {
        method: 'GET',
        signal,
      });
    },
    getLog(entryID, signal) {
      return request<LogEntry>(fetcher, `${baseUrl}/logs/${encodeURIComponent(entryID)}`, {
        method: 'GET',
        signal,
      });
    },
    getMetrics(signal) {
      return request<MetricsSnapshot>(fetcher, `${baseUrl}/metrics`, {
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
