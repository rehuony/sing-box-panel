import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type {
  MetricsHistory, RuntimeHistoryFilter, RuntimeHistoryPage, RuntimeStatus, TaskPage,
} from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { ControlPlaneContext } from '@/stores/control-plane.store';
import { DashboardPage } from '@/pages/dashboard-page/dashboard-page';
import {
  createMockApiClient,
  testDashboardContext,
} from '@/tests/api/mock-api-client';

const buckets = [
  {
    from: '2026-08-30T01:00:00Z',
    to: '2026-08-30T01:15:00Z',
    upload_bytes: 1_024,
    download_bytes: 2_048,
    memory_bytes_avg: 64_000_000,
    memory_bytes_peak: 70_000_000,
    active_connections_avg: 8,
    active_connections_peak: 12,
    sample_count: 30,
    coverage: 'complete' as const,
  },
  {
    from: '2026-08-30T01:15:00Z',
    to: '2026-08-30T01:30:00Z',
    upload_bytes: null,
    download_bytes: null,
    memory_bytes_avg: null,
    memory_bytes_peak: null,
    active_connections_avg: null,
    active_connections_peak: null,
    sample_count: 0,
    coverage: 'missing' as const,
  },
];

const metricsHistory: MetricsHistory = {
  from: '2026-08-30T01:00:00Z',
  to: '2026-08-30T01:30:00Z',
  bucket_seconds: 900,
  buckets,
};

const runtimeHistoryPage: RuntimeHistoryPage = {
  history_started_at: '2026-08-30T01:00:00Z',
  items: [
    { id: 1, state: 'running', reason: 'started', occurred_at: '2026-08-30T01:00:00Z' },
    { id: 2, state: 'unknown', reason: 'inspection_lost', occurred_at: '2026-08-30T01:15:00Z' },
  ],
};

function createDashboardClient() {
  const client = createMockApiClient();
  client.getMetricsHistory = vi.fn().mockResolvedValue(metricsHistory);
  client.getRuntimeHistory = vi.fn().mockResolvedValue(runtimeHistoryPage);
  client.getRuntimeStatus = vi.fn().mockResolvedValue({
    desired_running: true,
    target_generation: 9,
    observation_state: 'running',
    applied_bundle_id: 'bundle_18',
  });
  return client;
}

function LocationProbe() {
  const location = useLocation();
  return <output data-testid='location-probe'>{`${location.pathname}${location.search}`}</output>;
}

function renderDashboard(client = createDashboardClient()) {
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <ControlPlaneContext value={{
          status: 'ready',
          context: testDashboardContext,
          message: null,
          refresh: vi.fn().mockResolvedValue(undefined),
          setViewVersion: vi.fn(),
          viewVersion: testDashboardContext.view.exactVersion,
        }}>
          <DashboardPage />
          <LocationProbe />
        </ControlPlaneContext>
      </ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}

function chartDomain(chart: HTMLElement): [number, number] {
  return [Number(chart.dataset.domainMin), Number(chart.dataset.domainMax)];
}

describe('dashboardPage', () => {
  it('renders one compact history workbench without legacy control-plane panels', async () => {
    const client = renderDashboard();

    expect(await screen.findByRole('heading', { name: 'History' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Traffic' })).toHaveAttribute('data-active');
    expect(screen.getByRole('button', { name: '24h' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getAllByText('3 KB').length).toBeGreaterThan(0);
    expect(screen.getByText('50%')).toBeInTheDocument();
    expect(screen.queryByText('Control plane')).not.toBeInTheDocument();
    expect(screen.queryByText('Control chain')).not.toBeInTheDocument();
    expect(client.getMetrics).not.toHaveBeenCalled();
    expect(client.getMetricsHistory).toHaveBeenCalledWith(
      expect.objectContaining({ bucketSeconds: 900 }),
      expect.any(AbortSignal),
    );
  });

  it('keeps missing buckets empty and offers a keyboard-operable table fallback', async () => {
    renderDashboard();
    const tableToggle = await screen.findByText('Accessible data table');
    await userEvent.click(tableToggle);

    const rows = screen.getAllByRole('row');
    expect(within(rows[2]).getByText('Missing')).toBeInTheDocument();
    expect(within(rows[2]).getAllByText('—')).toHaveLength(4);
    expect(screen.queryByText(/^0 B$/)).not.toBeInTheDocument();

    const chart = screen.getByRole('application', {
      name: /History chart/,
    });
    chart.focus();
    await userEvent.keyboard('{ArrowRight}');
    expect(screen.getAllByText('Missing').length).toBeGreaterThan(0);
    await userEvent.keyboard('{Enter}');
    expect(screen.getByTestId('location-probe')).toHaveTextContent('/observability?from=2026-08-30T01%3A15%3A00Z');
  });

  it('keeps range controls inside the uPlot work area without a brush', async () => {
    renderDashboard();
    const chart = await screen.findByRole('application', { name: /History chart/ });

    expect(chart).toHaveAttribute('data-chart-library', 'uplot');
    expect(within(chart).getByRole('button', { name: '24h' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(chart).getByText(/Ctrl \+ scroll to zoom/)).toBeInTheDocument();
    expect(chart.querySelector('.recharts-brush, .recharts-brush-traveller')).toBeNull();
  });

  it('leaves ordinary wheel scrolling alone and gates zoom and pan behind modifiers', async () => {
    renderDashboard();
    const chart = await screen.findByRole('application', { name: /History chart/ });
    const initialDomain = chartDomain(chart);

    const ordinaryWheel = new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -120,
    });
    fireEvent(chart, ordinaryWheel);
    expect(ordinaryWheel.defaultPrevented).toBe(false);
    expect(chartDomain(chart)).toEqual(initialDomain);

    const zoomWheel = new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      clientX: 160,
      ctrlKey: true,
      deltaY: -120,
    });
    fireEvent(chart, zoomWheel);
    expect(zoomWheel.defaultPrevented).toBe(true);
    await waitFor(() => {
      const zoomedDomain = chartDomain(chart);
      expect(zoomedDomain[1] - zoomedDomain[0]).toBeLessThan(initialDomain[1] - initialDomain[0]);
    });

    const beforePan = chartDomain(chart);
    const panWheel = new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: 80,
      shiftKey: true,
    });
    fireEvent(chart, panWheel);
    expect(panWheel.defaultPrevented).toBe(true);
    await waitFor(() => expect(chartDomain(chart)[0]).toBeGreaterThan(beforePan[0]));
  });

  it('supports keyboard zoom, pan, cursor inspection, and reset', async () => {
    renderDashboard();
    const user = userEvent.setup();
    const chart = await screen.findByRole('application', { name: /History chart/ });
    const initialDomain = chartDomain(chart);
    chart.focus();

    await user.keyboard('+');
    const zoomedDomain = chartDomain(chart);
    expect(zoomedDomain[1] - zoomedDomain[0]).toBeLessThan(initialDomain[1] - initialDomain[0]);

    await user.keyboard('{Shift>}{ArrowRight}{/Shift}');
    expect(chartDomain(chart)[0]).toBeGreaterThan(zoomedDomain[0]);

    await user.keyboard('{ArrowRight}');
    expect(screen.getAllByText('Missing').length).toBeGreaterThan(0);

    await user.keyboard('{Home}');
    expect(chartDomain(chart)).toEqual(initialDomain);
  });

  it('filters history by bundle and links a bucket to matching observability logs', async () => {
    const client = renderDashboard();
    const user = userEvent.setup();
    await screen.findByRole('heading', { name: 'History' });
    await waitFor(() => expect(client.getMetricsHistory).toHaveBeenCalledTimes(1));
    await waitFor(() => {
      expect(client.getRuntimeHistory).toHaveBeenCalledTimes(1);
      expect(client.getRuntimeStatus).toHaveBeenCalledTimes(1);
      expect(client.listTasks).toHaveBeenCalledTimes(1);
    });
    await user.type(screen.getByPlaceholderText('All bundles'), 'bundle_18');
    expect(client.getMetricsHistory).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', { name: 'Apply filter' }));

    await waitFor(() => expect(client.getMetricsHistory).toHaveBeenLastCalledWith(
      expect.objectContaining({ activationBundleID: 'bundle_18' }),
      expect.any(AbortSignal),
    ));
    expect(client.getRuntimeHistory).toHaveBeenCalledTimes(1);
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(1);
    expect(client.listTasks).toHaveBeenCalledTimes(1);

    await user.click(screen.getByText('Accessible data table'));
    const links = screen.getAllByRole('button', { name: /8\/30\/26/ });
    expect(links[0]).toBeInTheDocument();
  });

  it('hides stale samples until a changed bundle filter has matching history', async () => {
    const client = createDashboardClient();
    let resolveFilteredHistory: (history: MetricsHistory) => void = () => {};
    client.getMetricsHistory = vi.fn()
      .mockResolvedValueOnce(metricsHistory)
      .mockImplementationOnce(() => new Promise<MetricsHistory>((resolve) => {
        resolveFilteredHistory = resolve;
      }));
    renderDashboard(client);
    const user = userEvent.setup();

    expect((await screen.findAllByText('3 KB')).length).toBeGreaterThan(0);
    await user.type(screen.getByPlaceholderText('All bundles'), 'bundle_18');
    await user.click(screen.getByRole('button', { name: 'Apply filter' }));

    expect(await screen.findByText('History filters changed. Waiting for matching samples.')).toBeInTheDocument();
    expect(screen.queryByText('3 KB')).not.toBeInTheDocument();
    expect(client.getRuntimeHistory).toHaveBeenCalledTimes(1);
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(1);
    expect(client.listTasks).toHaveBeenCalledTimes(1);

    resolveFilteredHistory({
      ...metricsHistory,
      buckets: [{ ...buckets[0], upload_bytes: 4_096 }],
    });
    expect((await screen.findAllByText('6 KB')).length).toBeGreaterThan(0);
    expect(screen.queryByText('History filters changed. Waiting for matching samples.')).not.toBeInTheDocument();
  });

  it('refreshes range-bound history without reloading the current task snapshot', async () => {
    const client = createDashboardClient();
    let resolveRuntimeHistory: (history: RuntimeHistoryPage) => void = () => {};
    client.getRuntimeHistory = vi.fn()
      .mockResolvedValueOnce(runtimeHistoryPage)
      .mockImplementationOnce(() => new Promise<RuntimeHistoryPage>((resolve) => {
        resolveRuntimeHistory = resolve;
      }));
    renderDashboard(client);
    const user = userEvent.setup();

    await screen.findByRole('img', { name: 'Observed runtime state across the selected range' });
    await waitFor(() => expect(client.listTasks).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('button', { name: '6h' }));

    expect(await screen.findByText('The time range changed. Waiting for matching runtime history.')).toBeInTheDocument();
    await waitFor(() => expect(client.getMetricsHistory).toHaveBeenLastCalledWith(
      expect.objectContaining({ bucketSeconds: 300 }),
      expect.any(AbortSignal),
    ));
    expect(client.getRuntimeHistory).toHaveBeenCalledTimes(2);
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(1);
    expect(client.listTasks).toHaveBeenCalledTimes(1);

    resolveRuntimeHistory(runtimeHistoryPage);
    await waitFor(() => expect(screen.queryByText('The time range changed. Waiting for matching runtime history.')).not.toBeInTheDocument());
  });

  it('hides stale runtime evidence and tasks during an explicit snapshot refresh', async () => {
    const client = createDashboardClient();
    const initialStatus: RuntimeStatus = {
      desired_running: true,
      target_generation: 9,
      observation_state: 'running',
      applied_bundle_id: 'bundle_18',
    };
    let resolveRuntimeStatus: (status: RuntimeStatus) => void = () => {};
    let resolveTaskPage: (page: TaskPage) => void = () => {};
    client.getRuntimeStatus = vi.fn()
      .mockResolvedValueOnce(initialStatus)
      .mockImplementationOnce(() => new Promise<RuntimeStatus>((resolve) => {
        resolveRuntimeStatus = resolve;
      }));
    client.listTasks = vi.fn()
      .mockResolvedValueOnce({ items: [] })
      .mockImplementationOnce(() => new Promise<TaskPage>((resolve) => {
        resolveTaskPage = resolve;
      }));
    renderDashboard(client);
    const user = userEvent.setup();

    expect(await screen.findByText('bundle_18')).toBeInTheDocument();
    expect(await screen.findByText('No task records.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Refresh' }));

    expect(await screen.findByText('Runtime evidence is refreshing; the previous snapshot is hidden.')).toBeInTheDocument();
    expect(screen.getByText('Recent tasks are refreshing; the previous snapshot is hidden.')).toBeInTheDocument();
    expect(screen.queryByText('bundle_18')).not.toBeInTheDocument();
    expect(screen.queryByText('No task records.')).not.toBeInTheDocument();
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(2);
    expect(client.listTasks).toHaveBeenCalledTimes(2);

    resolveRuntimeStatus({ ...initialStatus, applied_bundle_id: 'bundle_19', target_generation: 10 });
    resolveTaskPage({ items: [] });
    expect(await screen.findByText('bundle_19')).toBeInTheDocument();
    expect(await screen.findByText('No task records.')).toBeInTheDocument();
    expect(screen.queryByText('Recent tasks are refreshing; the previous snapshot is hidden.')).not.toBeInTheDocument();
  });

  it('renders unknown runtime as a hatched interval excluded from availability', async () => {
    renderDashboard();
    const timeline = await screen.findByRole('img', { name: 'Observed runtime state across the selected range' });
    expect(timeline.querySelector('[data-state=\'unknown\']')).not.toBeNull();
    expect(screen.getByText('Unknown intervals are excluded from availability.')).toBeInTheDocument();
  });

  it('keeps the prefix unknown when pagination stops at the 4096-transition client cap', async () => {
    const client = createDashboardClient();
    let pageIndex = 0;
    client.getRuntimeHistory = vi.fn(async (filter: RuntimeHistoryFilter = {}) => {
      const rangeStart = Date.parse(filter.from ?? '');
      const currentPage = pageIndex;
      pageIndex += 1;
      const occurredAt = new Date(rangeStart + (8 - currentPage) * 3_600_000).toISOString();
      const highestID = (8 - currentPage) * 512;
      const items = Array.from({ length: 512 }, (_value, index) => ({
        id: highestID - index,
        occurred_at: occurredAt,
        reason: 'heartbeat',
        state: 'running' as const,
      }));
      return {
        history_started_at: new Date(rangeStart - 3_600_000).toISOString(),
        items,
        next: { id: items[items.length - 1].id, occurred_at: occurredAt },
        preceding: {
          id: 5_000,
          occurred_at: new Date(rangeStart - 1_800_000).toISOString(),
          reason: 'started',
          state: 'running' as const,
        },
      };
    });

    renderDashboard(client);
    const timeline = await screen.findByRole('img', { name: 'Observed runtime state across the selected range' });
    await waitFor(() => expect(client.getRuntimeHistory).toHaveBeenCalledTimes(8));
    const firstSegment = timeline.querySelector(':scope > span');
    expect(firstSegment).toHaveAttribute('data-state', 'unknown');
    expect(firstSegment).toHaveStyle({ flexGrow: 3_600_000 });
    expect(screen.getByText(/4096-transition display limit/)).toBeInTheDocument();
  });

  it('surfaces runtime status failure without discarding history', async () => {
    const client = createDashboardClient();
    client.getRuntimeStatus = vi.fn().mockRejectedValue(new Error('runtime status failed'));

    renderDashboard(client);

    expect(await screen.findByText('runtime status failed')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'History' })).toBeInTheDocument();
    expect(screen.getAllByText('3 KB').length).toBeGreaterThan(0);
  });
});
