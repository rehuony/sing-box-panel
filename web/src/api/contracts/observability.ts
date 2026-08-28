import type { JsonObject } from './common';
import type { MonitoringTier } from './core';

export type LogSource = 'panel' | 'core' | 'task' | 'security';
export type LogLevel = 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';

export interface LogEntry {
  id: string;
  code: string;
  time: string;
  level: LogLevel;
  message: string;
  source: LogSource;
  metadata: JsonObject;
}

export interface LogPage {
  items: LogEntry[];
  next?: { time: string; id: string };
}

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

export interface TrafficPeriod {
  id: string;
  created_at: string;
  period_end: string;
  counters: JsonObject;
  period_start: string;
  inbound_bytes: number;
  outbound_bytes: number;
  activation_bundle_id?: string;
}

export interface MetricsSnapshot {
  available: boolean;
  collected_at: string;
  quota_bytes?: number;
  quota_exceeded: boolean;
  applied_bundle_id?: string;
  monitoring_tier?: MonitoringTier;
  current_traffic_period?: TrafficPeriod;
  reason_code?:
    | 'not_applied'
    | 'process_only'
    | 'no_collector_sample'
    | 'stale_collector_sample'
    | string;
  latest_sample?: {
    sampled_at: string;
    memory_bytes: number;
    active_connections: number;
    upload_total: number;
    download_total: number;
  };
}

export interface TrafficPeriodPage {
  items: TrafficPeriod[];
}

export interface TrafficPeriodFilter {
  to?: string;
  from?: string;
  limit?: number;
  activationBundleID?: string;
}
