import type { LogEntry } from '../generated';

export type {
  DashboardStreamSnapshot,
  LogEntry,
  MetricsHistory,
  MetricsSnapshot,
  NodeSettings,
  NodeTrafficUsage,
  TrafficPeriod,
  TrafficSample,
  TrafficStatus,
} from '../generated';

export type LogLevel = LogEntry['level'];

export type { CoreLogChunk, CoreLogFile, PanelLog, PanelLogPage } from '../generated';
export interface PanelLogFilter {
  limit?: number;
  since?: string;
  until?: string;
  offset?: number;
  search?: string;
  level?: LogLevel;
  beforeID?: string;
  beforeTime?: string;
  searchCodes?: string[];
}
