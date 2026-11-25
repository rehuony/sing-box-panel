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
/** The entries a log request reads, counts or clears: the panel's own, about no node, or one node's. */
export type LogScope = { nodeID: string } | { panel: true };
export interface PanelLogFilter {
  limit?: number;
  since?: string;
  until?: string;
  offset?: number;
  search?: string;
  level?: LogLevel;
  scope?: LogScope;
  beforeID?: string;
  beforeTime?: string;
  searchCodes?: string[];
}
