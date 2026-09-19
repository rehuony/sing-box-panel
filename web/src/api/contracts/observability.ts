import type { LogEntry } from '../generated';

export type {
  CoverageStatus,
  LogEntry,
  LogPage,
  MetricsHistory,
  MetricsHistoryBucket,
  MetricsSnapshot,
  TrafficPeriod,
  TrafficPeriodCursor,
  TrafficPeriodPage,
  TrafficSample,
} from '../generated';

export type LogSource = LogEntry['source'];
export type LogLevel = LogEntry['level'];

export interface LogFilter {
  code?: string;
  limit?: number;
  since?: string;
  until?: string;
  afterID?: string;
  level?: LogLevel;
  afterTime?: string;
  source?: LogSource;
}

export interface LogStreamFilter extends Omit<LogFilter, 'until'> {
  lastEventID?: string;
}

export interface LogStreamEvent {
  id: string;
  entry: LogEntry;
}

export interface LogClearFilter {
  before?: string;
  source?: LogSource;
}

export interface TrafficPeriodFilter {
  to?: string;
  from?: string;
  limit?: number;
  beforeID?: string;
  beforeTime?: string;
  activationBundleID?: string;
}

export interface MetricsHistoryFilter {
  to: string;
  from: string;
  bucketSeconds: number;
  activationBundleID?: string;
}

export type { CoreLogChunk, CoreLogFile, PanelLog, PanelLogPage } from '../generated';
export interface PanelLogFilter {
  limit?: number;
  since?: string;
  until?: string;
  search?: string;
  level?: LogLevel;
  beforeID?: string;
  beforeTime?: string;
}
