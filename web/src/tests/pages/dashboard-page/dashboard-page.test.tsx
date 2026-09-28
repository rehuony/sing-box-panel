import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';

import type { TelemetryState } from '@/components/app-shell/use-telemetry';

import '@/i18n';
import type { DashboardStreamSnapshot, MetricsSnapshot } from '@/api/api-client';

import { DashboardPage } from '@/pages/dashboard-page/dashboard-page';
import { TelemetryContext } from '@/components/app-shell/telemetry-context';
import { createTelemetryStore } from '@/components/app-shell/use-telemetry';
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
  const telemetry: Partial<TelemetryState> = {
    acceptRuntimeStatus: vi.fn(),
    dashboardError: null,
    dashboardSnapshot: dashboard,
    liveStale: false,
    rates: { downloadBytesPerSecond: null, uploadBytesPerSecond: null },
    runtimeError: null,
    runtimeStatus: testRuntimeStatus,
    snapshot: metrics,
    trafficError: null,
  };
  const store = createTelemetryStore(telemetry);
  render(
    <MemoryRouter>
      <TelemetryContext value={store}>
        <DashboardPage />
      </TelemetryContext>
    </MemoryRouter>,
  );
  return store;
}

describe('dashboard evidence', () => {
  it('updates the traffic heading from live peaks and the selected history range', () => {
    const store = show({ dashboard: {
      ...testDashboardSnapshot,
      history_24h: {
        ...testDashboardSnapshot.history_24h,
        buckets: [{ ...testDashboardSnapshot.history_24h.buckets[0]!, download_bytes: 300 * 1024 ** 3 }],
      },
    } });
    expect(screen.getByRole('heading', { name: 'Traffic（B/s）' })).toBeVisible();
    act(() => store.setState({ liveTail: [{
      at: Date.parse(testDashboardSnapshot.collected_at) + 2000,
      downloadBytesPerSecond: 1024 ** 2, uploadBytesPerSecond: 0, connections: 10,
    }] }));
    expect(screen.getByRole('heading', { name: 'Traffic（MB/s）' })).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: '24h' }));
    expect(screen.getByRole('heading', { name: 'Traffic（GB/s）' })).toBeVisible();
    act(() => store.setState({ liveTail: [] }));
    fireEvent.click(screen.getByRole('tab', { name: '1h' }));
    expect(screen.getByRole('heading', { name: 'Traffic（B/s）' })).toBeVisible();
  });

  it('shows host metrics and recorded usage while live collection is unavailable', () => {
    show({ metrics: {
      ...testMetrics,
      available: false,
      reason_code: 'process_only',
      monitoring_tier: 'process_only',
      host: {
        sampled_at: new Date().toISOString(), cpu_count: 2, cpu_percent: 12.8,
        load_one: 0.1, memory_total: 1000, memory_used: 500, disk_total: 2000, disk_used: 500,
      },
    } });
    expect(screen.getByText('12.8%')).toBeVisible();
    expect(screen.getByText('6 KB')).toBeVisible();
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

  it('keeps usage unknown until the first successful metrics response', () => {
    show({ metrics: null });
    const card = screen.getByText('Period traffic').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('Usage unknown');
  });

  it('shows zero recorded usage for an empty period', () => {
    show({ metrics: { ...testMetrics, available: false,
      current_traffic_period: { ...testMetrics.current_traffic_period, inbound_bytes: 0, outbound_bytes: 0 },
    } });
    const card = screen.getByText('Period traffic').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('0 B');
    expect(card.querySelector('small')).toHaveTextContent('0 / ∞ GiB');
  });

  it('preserves recorded usage and quota when collection is stale', () => {
    show({ metrics: { ...testMetrics, available: false, reason_code: 'stale_collector_sample', quota_bytes: 12_288 } });
    const card = screen.getByText('Period traffic').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('6 KB');
    expect(card.querySelector('small')).toHaveTextContent('Used 50.0% · quota 12 KB');
  });

  it('keeps the configured finite quota presentation', () => {
    show({ metrics: { ...testMetrics, quota_bytes: 12_288 } });
    const card = screen.getByText('Period traffic').closest('section')!;
    expect(card.querySelector('small')).toHaveTextContent('Used 50.0% · quota 12 KB');
  });
});
