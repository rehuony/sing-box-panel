import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, within } from '@testing-library/react';

import type { TelemetryState } from '@/components/app-shell/use-telemetry';
import type { DashboardStreamSnapshot, MetricsSnapshot, PanelLog } from '@/api/api-client';

import i18n from '@/i18n';
import { DashboardPage } from '@/pages/dashboard-page/dashboard-page';
import { TelemetryContext } from '@/components/app-shell/telemetry-context';
import { createTelemetryStore } from '@/components/app-shell/use-telemetry';
import {
  testDashboardSnapshot,
  testMetrics,
  testRuntimeStatus,
} from '@/tests/api/mock-api-client';

const activityEntry: PanelLog = {
  id: 'runtime:recovered', time: new Date(2026, 9, 16, 16, 20).toISOString(),
  source: 'runtime', level: 'info', code: 'recovery_succeeded', message: 'recovery_succeeded',
  status: 'running', metadata: {},
};

afterEach(async () => {
  await i18n.changeLanguage('en');
});

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
  it('defaults to live peaks, retains them through history refresh, and separates historical averages', () => {
    const store = show({ dashboard: {
      ...testDashboardSnapshot,
      history_24h: {
        ...testDashboardSnapshot.history_24h,
        buckets: [{ ...testDashboardSnapshot.history_24h.buckets[0]!, download_bytes: 300 * 1024 ** 3 }],
      },
    } });
    expect(screen.getByRole('tab', { name: 'Live' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('heading', { name: 'Traffic（B/s）' })).toBeVisible();
    act(() => store.setState({ liveTail: [{
      at: Date.parse(testDashboardSnapshot.collected_at) + 2000,
      downloadBytesPerSecond: 1024 ** 2, uploadBytesPerSecond: 0, connections: 10,
    }] }));
    expect(screen.getByRole('heading', { name: 'Traffic（MB/s）' })).toBeVisible();
    act(() => store.setState({ dashboardSnapshot: {
      ...store.getState().dashboardSnapshot!,
      persisted_through: new Date(Date.parse(testDashboardSnapshot.collected_at) + 4000).toISOString(),
    } }));
    expect(screen.getByRole('heading', { name: 'Traffic（MB/s）' })).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: '24h' }));
    expect(screen.getByRole('heading', { name: 'Traffic（GB/s）' })).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: '1h' }));
    expect(screen.getByRole('heading', { name: 'Traffic（B/s）' })).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: 'Live' }));
    expect(screen.getByRole('heading', { name: 'Traffic（MB/s）' })).toBeVisible();
  });

  it('uses live metrics readiness independently of history readiness', () => {
    const store = show({ dashboard: null, metrics: null });
    const traffic = within(screen.getByRole('tabpanel'));
    expect(traffic.getByText('Waiting for live samples…')).toBeVisible();
    act(() => store.setState({ snapshot: testMetrics }));
    expect(traffic.queryByText('Waiting for live samples…')).not.toBeInTheDocument();
    expect(traffic.getByText('No samples in this range')).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: '1h' }));
    expect(within(screen.getByRole('tabpanel')).getByText('Loading matching history…')).toBeVisible();
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

  it('localizes recent events, preserves unknown messages, and links the entire row to panel logs', async () => {
    const store = show({ dashboard: {
      ...testDashboardSnapshot,
      activity: { items: [
        activityEntry,
        { ...activityEntry, id: 'panel:recovery', source: 'panel', level: 'warn', code: 'runtime.recovery_attempt', message: 'Automatic sing-box recovery attempt started' },
      ], total: 2 },
    } });
    const activity = screen.getByRole('region', { name: 'Recent activity' });
    const recovered = within(activity).getByRole('link', { name: /Core recovered/ });
    expect(within(recovered).getByText('INFO')).toBeVisible();
    expect(recovered.querySelector('time')).toHaveAttribute('datetime', activityEntry.time);
    for (const link of within(activity).getAllByRole('link')) {
      expect(link).toHaveAttribute('href', '/observability#logs-panel');
    }
    expect(within(activity).getByRole('link', { name: /Automatic recovery attempt started/ })).toHaveTextContent('WARN');

    await act(() => i18n.changeLanguage('zh-CN'));
    expect(within(activity).getByRole('link', { name: /核心已恢复/ })).toBeVisible();
    expect(within(activity).getByRole('link', { name: /开始尝试自动恢复核心/ })).toBeVisible();
    act(() => store.setState({ dashboardSnapshot: {
      ...testDashboardSnapshot,
      activity: { items: [{ ...activityEntry, code: 'future.event', message: 'A future event message' }], total: 1 },
    } }));
    expect(within(activity).getByRole('link', { name: /A future event message/ })).toBeVisible();
  });

  it.each([
    { name: 'today', date: new Date(2026, 9, 16, 16, 20), label: '16:20' },
    { name: 'another day', date: new Date(2026, 9, 15, 16, 20), label: '10/15, 16:20' },
    { name: 'another year', date: new Date(2025, 9, 16, 16, 20), label: '10/16/2025, 16:20' },
  ])('shows an unambiguous compact activity time for $name', ({ date, label }) => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 9, 16, 17, 6));
    const time = date.toISOString();
    show({ dashboard: {
      ...testDashboardSnapshot,
      activity: { items: [{ ...activityEntry, time }], total: 1 },
    } });
    const link = screen.getByRole('link', { name: /Core recovered/ });
    expect(link.querySelector('time')).toHaveAttribute('datetime', time);
    expect(link.querySelector('time')?.textContent).toBe(label);
  });

  it('updates activity dates at local midnight without moving a delayed snapshot timeline', async () => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] });
    vi.setSystemTime(new Date(2026, 9, 16, 23, 59, 59));
    show({ dashboard: {
      ...testDashboardSnapshot,
      collected_at: new Date(2026, 9, 16, 16, 21).toISOString(),
      activity: { items: [activityEntry], total: 1 },
    } });
    const time = screen.getByRole('link', { name: /Core recovered/ }).querySelector('time');
    const timeline = screen.getByRole('region', { name: 'Runtime timeline' });
    const ticks = Array.from(timeline.querySelectorAll('time'), tick => tick.dateTime);
    expect(time?.textContent).toBe('16:20');

    await act(() => vi.advanceTimersByTimeAsync(2000));

    expect(time?.textContent).toBe('10/16, 16:20');
    expect(Array.from(timeline.querySelectorAll('time'), tick => tick.dateTime)).toEqual(ticks);
  });

  it('exposes the full activity message and timestamp on keyboard focus', async () => {
    const message = 'Unrecognized recovery event with enough context to exceed a compact activity row';
    show({ dashboard: {
      ...testDashboardSnapshot,
      activity: { items: [{ ...activityEntry, code: 'future.event', message }], total: 1 },
    } });
    const link = screen.getByRole('link', { name: new RegExp(message) });
    act(() => screen.getByRole('link', { name: 'View all' }).focus());
    await userEvent.tab();
    const tooltip = await screen.findByRole('tooltip');
    expect(link).toHaveFocus();
    expect(tooltip).toHaveTextContent(message);
    expect(tooltip).toHaveTextContent('10/16/2026, 16:20:00');
  });

  it('anchors timeline ticks and accessible intervals to the snapshot until a new snapshot arrives', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 9, 17, 12));
    const end = new Date(2026, 9, 16, 17, 6).getTime();
    const dashboard = { ...testDashboardSnapshot, collected_at: new Date(end).toISOString() };
    const store = show({ dashboard });
    const card = screen.getByRole('heading', { name: 'Runtime timeline' }).closest('section')!;
    const expectedTicks = Array.from({ length: 5 }, (_, index) => (
      new Date(end - (24 - 6 * index) * 3_600_000).toISOString()
    ));
    expect(Array.from(card.querySelectorAll('time'), time => time.dateTime)).toEqual(expectedTicks);
    expect(card.querySelector('time')).toHaveTextContent('10/15');
    expect(card.querySelectorAll('time')[4]).toHaveTextContent('10/16');

    const timeline = within(card).getByRole('group', { name: 'Observed runtime state across the selected range' });
    const acrossMidnight = within(timeline).getByRole('button', { name: /10\/15.*23:36.*10\/16.*00:06.*running/ });
    act(() => within(timeline).getByRole('button', { name: /10\/15.*23:06.*23:36.*running/ }).focus());
    await userEvent.tab();
    expect(acrossMidnight).toHaveFocus();
    expect(await screen.findByRole('tooltip')).toHaveTextContent(acrossMidnight.getAttribute('aria-label')!);
    act(() => acrossMidnight.blur());

    vi.setSystemTime(new Date(2026, 9, 18, 12));
    act(() => store.setState({ dashboardSnapshot: { ...dashboard } }));
    expect(Array.from(card.querySelectorAll('time'), time => time.dateTime)).toEqual(expectedTicks);
    act(() => store.setState({ dashboardSnapshot: {
      ...dashboard, collected_at: new Date(end + 3_600_000).toISOString(),
    } }));
    expect(Array.from(card.querySelectorAll('time'), time => time.dateTime)).toEqual(
      expectedTicks.map(value => new Date(Date.parse(value) + 3_600_000).toISOString()),
    );
  });

  it('distinguishes loading activity from an empty completed snapshot', () => {
    const store = show({ dashboard: null });
    const activity = within(screen.getByRole('region', { name: 'Recent activity' }));
    expect(activity.getByText('Loading recent activity…')).toBeVisible();
    expect(activity.queryByText('No activity records.')).not.toBeInTheDocument();
    act(() => store.setState({ dashboardSnapshot: testDashboardSnapshot }));
    expect(activity.getByText('No activity records.')).toBeVisible();
    expect(activity.queryByText('Loading recent activity…')).not.toBeInTheDocument();
  });
});
