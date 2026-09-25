import { describe, expect, it } from 'vitest';

import type { RuntimeStatus } from '@/api/api-client';

import { runtimeMatchesAction } from '@/components/app-shell/use-runtime-control';
import { formatRate, formatUptime } from '@/components/app-shell/telemetry-format';

const labels = { day: 'd', hour: 'h', minute: 'm', second: 's' };

function runningStatus(processStartToken = 'process-new'): RuntimeStatus {
  return {
    desired_running: true,
    target_generation: 3,
    observation_state: 'running',
    running: {
      pid: 8124,
      process_start_token: processStartToken,
      exact_core_version: '1.13.19',
      core_artifact_id: 'core_1',
      archive_sha256: 'b'.repeat(64),
      binary_sha256: 'd'.repeat(64),
      activation_bundle_id: 'bundle_18',
      started_at: '2026-08-30T10:00:00Z',
    },
  };
}

describe('runtime evidence helpers', () => {
  it('formats uptime from the verified started_at timestamp', () => {
    const now = new Date('2026-08-30T12:00:00Z').getTime();
    expect(formatUptime('2026-08-29T09:45:00Z', now, labels)).toBe('1d 2h');
    expect(formatUptime(undefined, now, labels)).toBe('0s');
    expect(formatUptime('invalid', now, labels)).toBe('0s');
    expect(formatUptime('2026-08-30T12:01:00Z', now, labels)).toBe('0s');
    expect(formatUptime(undefined, now, { ...labels, second: '秒' })).toBe('0秒');
  });

  it.each([null, Number.NaN, Number.POSITIVE_INFINITY, -1, 0])('formats an unavailable or zero rate (%s) with its unit', value => {
    expect(formatRate(value)).toBe('0 B/s');
    expect(formatRate(value, 'zh-CN')).toBe('0 B/s');
  });

  it('requires a new process token when verifying a restart', () => {
    expect(runtimeMatchesAction('restart', runningStatus('process-new'), 'process-old')).toBe(true);
    expect(runtimeMatchesAction('restart', runningStatus('process-old'), 'process-old')).toBe(false);
    expect(runtimeMatchesAction('start', runningStatus(), undefined)).toBe(true);
    expect(runtimeMatchesAction('stop', {
      desired_running: false,
      target_generation: 4,
      observation_state: 'stopped',
    })).toBe(true);
  });
});
