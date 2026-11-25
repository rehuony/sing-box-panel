import type { LogEntry } from '../generated';
import type { RuntimeTransitionState } from './core';

export type {
  CoreLogChunk,
  CoreLogFile,
  FleetRuntimeHistoryPage,
  LogClearResult,
  LogUsage,
  NodeRuntimeTransition,
  PanelLogPage,
  RecentMetrics,
} from '../generated';

export type LogLevel = LogEntry['level'];

export type {
  DashboardStreamSnapshot,
  LogEntry,
  MetricsHistory,
  MetricsSnapshot,
  NodeSettings,
  TrafficPeriod,
  TrafficSample,
} from '../generated';

export interface FleetRuntimeHistoryFilter {
  limit?: number;
  /** Keeps only this node's transitions. */
  nodeID?: string;
  offset?: number;
  /** Part of a node's name or of a reason. */
  search?: string;
  /** Reasons whose translated names match the search. */
  searchCodes?: string[];
  state?: RuntimeTransitionState;
}
export interface PanelLogFilter {
  limit?: number;
  since?: string;
  until?: string;
  /** Keeps only the entries about this node. */
  nodeID?: string;
  offset?: number;
  search?: string;
  level?: LogLevel;
  beforeID?: string;
  beforeTime?: string;
  searchCodes?: string[];
}
