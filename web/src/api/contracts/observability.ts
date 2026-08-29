import type { LogEntry } from '../generated';

export type {
  LogEntry,
  LogPage,
  MetricsSnapshot,
  TrafficPeriod,
  TrafficPeriodPage,
} from '../generated';

export type LogSource = LogEntry['source'];
export type LogLevel = LogEntry['level'];

export interface LogFilter {
  limit?: number;
  since?: string;
  afterID?: string;
  level?: LogLevel;
  afterTime?: string;
  source?: LogSource;
}

export interface LogClearFilter {
  before?: string;
  source?: LogSource;
}

export interface TrafficPeriodFilter {
  to?: string;
  from?: string;
  limit?: number;
  activationBundleID?: string;
}
