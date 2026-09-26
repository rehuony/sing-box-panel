import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import '@/i18n';
import type { TelemetryState } from '@/components/app-shell/use-telemetry';
import type { DashboardStreamSnapshot, MetricsSnapshot } from '@/api/api-client';

import { DashboardPage } from '@/pages/dashboard-page/dashboard-page';
import { TelemetryContext } from '@/components/app-shell/telemetry-context';
import {
  testDashboardSnapshot,
  testMetrics,
  testRuntimeStatus,
} from '@/tests/api/mock-api-client';

function show({
  dashboard = testDashboardSnapshot,
  metrics = testMetrics,
}: {
  dashboard?: DashboardStreamSnapshot | null;
  metrics?: MetricsSnapshot | null;
} = {}) {
  const telemetry: TelemetryState = {
    acceptRuntimeStatus: vi.fn(),
    dashboardError: null,
    dashboardSnapshot: dashboard,
    dashboardStale: false,
    rates: { downloadBytesPerSecond: null, uploadBytesPerSecond: null },
    runtimeError: null,
    runtimeStatus: testRuntimeStatus,
    snapshot: metrics,
    trafficError: null,
  };
  render(
    <MemoryRouter>
      <TelemetryContext value={telemetry}>
        <DashboardPage />
      </TelemetryContext>
    </MemoryRouter>,
  );
}

describe('dashboard evidence', () => {
  it('shows host metrics while core traffic is unavailable', () => {
    show({ metrics: {
      ...testMetrics,
      available: false,
      traffic_available: false,
      reason_code: 'process_only',
      monitoring_tier: 'process_only',
      host: {
        sampled_at: new Date().toISOString(), cpu_count: 2, cpu_percent: 12.8,
        load_one: 0.1, memory_total: 1000, memory_used: 500, disk_total: 2000, disk_used: 500,
      },
    } });
    expect(screen.getByText('12.8%')).toBeVisible();
    expect(screen.getByText('Usage unknown')).toBeVisible();
  });

  it('never replaces missing host readings with core process samples', () => {
    show();
    const card = screen.getByText('Host memory').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('—');
  });

  it('shows used traffic against infinity when quota is not configured', () => {
    show();
    const card = screen.getByText('Period traffic').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('6 KB');
    expect(card.querySelector('small')).toHaveTextContent('0 / ∞ GiB');
  });

  it('does not invent used traffic when evidence is missing', () => {
    show({ metrics: { ...testMetrics, traffic_available: false } });
    const card = screen.getByText('Period traffic').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('Usage unknown');
    expect(card.querySelector('small')).toHaveTextContent('— / ∞ GiB');
  });

  it('shows the configured quota alongside unknown usage and marks incomplete history', () => {
    show({ metrics: { ...testMetrics, available: false, traffic_available: false, quota_bytes: 500 * 2 ** 30 } });
    const card = screen.getByText('Period traffic').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('Usage unknown');
    expect(card.querySelector('small')).toHaveTextContent('500 GB');
  });

  it('marks partial monthly coverage without inventing missing usage', () => {
    show({ metrics: { ...testMetrics, traffic_coverage: 'partial' } });
    expect(screen.getByText('Period traffic').closest('section')).toHaveTextContent('Incomplete data');
  });

  it('keeps the configured finite quota presentation', () => {
    show({ metrics: { ...testMetrics, quota_bytes: 12_288 } });
    const card = screen.getByText('Period traffic').closest('section')!;
    expect(card.querySelector('small')).toHaveTextContent('Used 50.0% · quota 12 KB');
  });
});
