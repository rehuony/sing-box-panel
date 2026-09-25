import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { TelemetryState } from '@/components/app-shell/use-telemetry';
import type { ApiClient, MetricsSnapshot, RuntimeStatus } from '@/api/api-client';

import { setAppLanguage } from '@/i18n';
import { ThemeProvider } from '@/theme';
import { SidebarProvider } from '@/components/ui/sidebar';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { TelemetryBanner } from '@/components/app-shell/telemetry-banner';
import { TelemetryContext } from '@/components/app-shell/telemetry-context';
import { TelemetryProvider } from '@/components/app-shell/telemetry-provider';
import {
  createMockApiClient,
  testDashboardSnapshot,
  testMetrics,
} from '@/tests/api/mock-api-client';

function runtimeIdentity(processStartToken = 'process-8124') {
  return {
    pid: 8124,
    process_start_token: processStartToken,
    exact_core_version: '1.13.19',
    core_artifact_id: 'core_1',
    archive_sha256: 'b'.repeat(64),
    binary_sha256: 'd'.repeat(64),
    activation_bundle_id: 'bundle_18',
    started_at: new Date(Date.now() - 3_600_000).toISOString(),
  };
}

function runningStatus(processStartToken = 'process-8124'): RuntimeStatus {
  return {
    enabled_core: { core_artifact_id: 'core_1', exact_core_version: '1.13.19' },
    desired_running: true,
    applied_bundle_id: 'bundle_18',
    target_generation: 3,
    observation_state: 'running',
    running: runtimeIdentity(processStartToken),
  } as RuntimeStatus;
}

const stoppedStatus: RuntimeStatus = {
  enabled_core: { core_artifact_id: 'core_1', exact_core_version: '1.13.19' },
  desired_running: false,
  applied_bundle_id: 'bundle_18',
  target_generation: 4,
  observation_state: 'stopped',
};

function trafficSnapshot(
  sampledAt: string,
  uploadTotal: number,
  downloadTotal: number,
): MetricsSnapshot {
  return {
    ...testMetrics,
    latest_sample: {
      id: uploadTotal,
      activation_bundle_id: 'bundle_18',
      pid: 8124,
      process_start_token: 'process-8124',
      sampled_at: sampledAt,
      memory_bytes: 94_371_840,
      active_connections: 37,
      upload_total: uploadTotal,
      download_total: downloadTotal,
      upload_delta: uploadTotal,
      download_delta: downloadTotal,
      coverage: 'complete',
      accepted: true,
    },
  };
}

function waitForAbort(signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal?.aborted) resolve();
    else signal?.addEventListener('abort', () => resolve(), { once: true });
  });
}

function renderBanner(client: ApiClient) {
  client.streamMetrics = vi.fn(async function* (signal) {
    const runtime = await client.getRuntimeStatus(signal);
    for (let index = 0; index < 2; index++) {
      const metrics = await client.getTrafficStatus(signal);
      yield { metrics, runtime };
    }
    await waitForAbort(signal);
  });
  return render(
    <ApiClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider delay={0}>
          <SidebarProvider>
            <TelemetryProvider>
              <TelemetryBanner />
            </TelemetryProvider>
          </SidebarProvider>
        </TooltipProvider>
      </ThemeProvider>
    </ApiClientProvider>,
  );
}

describe('telemetryBanner', () => {
  beforeEach(async () => {
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
  });

  afterEach(async () => {
    vi.useRealTimers();
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
  });

  it.each(['en', 'zh-CN'] as const)('shows dashboard reconnect feedback inside the shared toolbar and clears it on recovery in %s', async (language) => {
    await setAppLanguage(language);
    const client = createMockApiClient();
    const telemetry: TelemetryState = {
      acceptRuntimeStatus: vi.fn(),
      dashboardSnapshot: testDashboardSnapshot,
      dashboardStale: true,
      dashboardError: null,
      runtimeError: null,
      trafficError: null,
      runtimeStatus: stoppedStatus,
      snapshot: testMetrics,
      rates: { uploadBytesPerSecond: 0, downloadBytesPerSecond: 0 },
    };
    const view = (value: TelemetryState) => (
      <ApiClientProvider client={client}>
        <TooltipProvider>
          <SidebarProvider>
            <TelemetryContext value={value}><TelemetryBanner /></TelemetryContext>
          </SidebarProvider>
        </TooltipProvider>
      </ApiClientProvider>
    );
    const { rerender } = render(view(telemetry));
    const message = language === 'en'
      ? 'Dashboard updates are interrupted. Keeping the last snapshot while reconnecting.'
      : '仪表盘数据暂时中断，正在保留现有数据并重新连接。';
    const notice = screen.getByRole('status');
    const trigger = screen.getByRole('button', { name: message });
    expect(screen.getByRole('banner')).toContainElement(notice);
    expect(notice).toContainElement(trigger);
    expect(screen.getByRole('button', { name: language === 'en' ? 'Start' : '启动' })).toBeEnabled();
    await userEvent.click(trigger);
    expect(await screen.findByRole('tooltip')).toHaveTextContent(message);
    await userEvent.keyboard('{Escape}');

    rerender(view({ ...telemetry, dashboardStale: false }));
    expect(screen.getByRole('status')).toBe(notice);
    expect(notice).toBeEmptyDOMElement();
    expect(screen.queryByRole('button', { name: message })).not.toBeInTheDocument();

    rerender(view({ ...telemetry, dashboardSnapshot: null }));
    expect(notice).toBeEmptyDOMElement();
  });

  it('renders stale runtime and traffic evidence as unknown instead of stopped', async () => {
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue({
        desired_running: true,
        target_generation: 3,
        observation_state: 'stale',
      }),
      getTrafficStatus: vi.fn().mockResolvedValue({
        ...testMetrics,
        available: false,
        reason_code: 'stale_collector_sample',
      }),
    });

    const { container } = renderBanner(client);

    expect(await screen.findByTitle('Runtime evidence is stale')).toBeInTheDocument();
    expect(screen.queryByText('Stopped')).not.toBeInTheDocument();
    expect(screen.queryByText('6 KB')).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Uptime: 0s. Runtime evidence is stale' })).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Upload: 0 B/s' })).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Download: 0 B/s' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Stop' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument();
    expect(container.querySelector('.telemetry-banner__identity [data-slot="separator"]')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Refresh runtime and traffic status' })).not.toBeInTheDocument();
  });

  it.each(['en', 'zh-CN'] as const)('keeps units in full and compact metrics before telemetry is available in %s', async language => {
    await setAppLanguage(language);
    const client = createMockApiClient();
    const telemetry: TelemetryState = {
      acceptRuntimeStatus: vi.fn(),
      dashboardSnapshot: null,
      dashboardStale: true,
      dashboardError: null,
      runtimeError: null,
      trafficError: null,
      runtimeStatus: null,
      snapshot: null,
      rates: { uploadBytesPerSecond: null, downloadBytesPerSecond: null },
    };
    const view = (value: TelemetryState) => (
      <ApiClientProvider client={client}>
        <TooltipProvider>
          <SidebarProvider>
            <TelemetryContext value={value}><TelemetryBanner /></TelemetryContext>
          </SidebarProvider>
        </TooltipProvider>
      </ApiClientProvider>
    );
    const { rerender } = render(view(telemetry));
    const uptime = screen.getByRole('group', { name: language === 'en' ? /^Uptime: 0s/ : /^运行时长: 0秒/ });
    expect(uptime.querySelector('.telemetry-metric__full')).toHaveTextContent(language === 'en' ? '0s' : '0秒');
    expect(uptime.querySelector('.telemetry-metric__compact')).toHaveTextContent('0s');
    const upload = screen.getByRole('group', { name: language === 'en' ? 'Upload: 0 B/s' : '上行: 0 B/s' });
    const download = screen.getByRole('group', { name: language === 'en' ? 'Download: 0 B/s' : '下行: 0 B/s' });
    for (const metric of [upload, download]) {
      expect(metric.querySelector('.telemetry-metric__full')).toHaveTextContent('0 B/s');
      expect(metric.querySelector('.telemetry-metric__compact')).toHaveTextContent('0B/s');
    }

    // The first valid sample still has no rate until a second sample arrives.
    rerender(view({ ...telemetry, runtimeStatus: runningStatus(), snapshot: testMetrics }));
    expect(upload).toHaveAccessibleName(language === 'en' ? 'Upload: 0 B/s' : '上行: 0 B/s');
    expect(download).toHaveAccessibleName(language === 'en' ? 'Download: 0 B/s' : '下行: 0 B/s');
  });

  it('shows verified identity, uptime and fresh rates', async () => {
    const first = trafficSnapshot('2026-08-30T11:00:00Z', 1_000, 2_000);
    const second = trafficSnapshot('2026-08-30T11:00:10Z', 3_000, 5_000);
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(runningStatus()),
      getTrafficStatus: vi.fn()
        .mockResolvedValueOnce(first)
        .mockResolvedValue(second),
    });

    renderBanner(client);

    expect(await screen.findByText('Running')).toBeInTheDocument();
    expect(screen.getByText('v1.13.19')).toBeInTheDocument();
    expect(screen.queryByText('arm64')).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: /Uptime: 1h.*Started/ })).toBeInTheDocument();
    expect(screen.queryByText('Version')).not.toBeInTheDocument();
    expect(screen.queryByText('Uptime')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stop' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Restart' })).toBeInTheDocument();
    expect(screen.queryByText('6 KB')).not.toBeInTheDocument();

    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    expect(await screen.findByText('200 B/s')).toBeInTheDocument();
    expect(screen.getByText('300 B/s')).toBeInTheDocument();
    // SSE heartbeats may carry the same persisted sample between collector ticks.
    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    expect(screen.getByText('200 B/s')).toBeInTheDocument();
    expect(screen.getByText('300 B/s')).toBeInTheDocument();
  });

  it.each(['en', 'zh-CN'] as const)('retains the enabled version and shows zero stopped metrics in %s', async (language) => {
    await setAppLanguage(language);
    renderBanner(createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(stoppedStatus),
      getTrafficStatus: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
    }));
    expect(await screen.findByText('v1.13.19')).toBeInTheDocument();
    expect(screen.getByRole('group', { name: language === 'en' ? /^Uptime: 0s/ : /^运行时长: 0秒/ })).toBeInTheDocument();
    expect(screen.getByRole('group', { name: language === 'en' ? 'Upload: 0 B/s' : '上行: 0 B/s' })).toBeInTheDocument();
    expect(screen.getByRole('group', { name: language === 'en' ? 'Download: 0 B/s' : '下行: 0 B/s' })).toBeInTheDocument();
    expect(screen.queryByText(/B\/秒/)).not.toBeInTheDocument();
  });

  it('does not request architecture for the status bar', async () => {
    const client = createMockApiClient({
      getSystemStatus: vi.fn().mockRejectedValue(new Error('Unavailable')),
      getRuntimeStatus: vi.fn().mockResolvedValue(runningStatus()),
    });
    renderBanner(client);
    expect(await screen.findByText('v1.13.19')).toBeInTheDocument();
    expect(client.getSystemStatus).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Stop' })).toBeEnabled();
  });

  it('waits for start to finish and verifies the returned runtime', async () => {
    const user = userEvent.setup();
    let finish!: (status: RuntimeStatus) => void;
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(stoppedStatus),
      getTrafficStatus: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
      startRuntime: vi.fn(() => new Promise<RuntimeStatus>(resolve => {
        finish = resolve;
      })),
    });
    renderBanner(client);
    await user.click(await screen.findByRole('button', { name: 'Start' }));
    expect(client.startRuntime).toHaveBeenCalledTimes(1);
    expect(screen.getByText('Start in progress…')).toBeInTheDocument();
    await act(async () => finish(runningStatus()));
    expect(await screen.findByText('Start verified')).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('requires confirmation for stop and restart actions', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn()
        .mockResolvedValueOnce(runningStatus('process-old'))
        .mockResolvedValue(runningStatus('process-new')),
      getTrafficStatus: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
      restartRuntime: vi.fn().mockResolvedValue(runningStatus('process-new')),
    });

    renderBanner(client);
    await screen.findByText('Running');
    await user.click(screen.getByRole('button', { name: 'Stop' }));
    expect(await screen.findByRole('heading', { name: 'Stop the sing-box runtime?' })).toBeInTheDocument();
    expect(client.stopRuntime).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    await user.click(screen.getByRole('button', { name: 'Stop' }));
    await user.click(await screen.findByRole('button', { name: 'Close' }));
    expect(client.stopRuntime).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Restart' }));
    expect(await screen.findByRole('heading', { name: 'Restart the sing-box runtime?' })).toBeInTheDocument();
    expect(client.restartRuntime).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Restart sing-box' }));

    expect(client.restartRuntime).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('Restart verified')).toBeInTheDocument();
  });

  it('keeps personalization and account actions out of the runtime toolbar', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(stoppedStatus),
      getTrafficStatus: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
    });

    renderBanner(client);
    await screen.findByText('Stopped');
    expect(screen.getByRole('button', { name: 'Start' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Stop' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Open language menu' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'More panel actions' }));
    expect(await screen.findByRole('menuitem', { name: 'Start' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Refresh runtime and traffic' })).not.toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Sign out' })).not.toBeInTheDocument();
  });

  it.each(['start', 'stop', 'restart'] as const)('replaces controls with %s feedback until it expires', async (action) => {
    vi.useFakeTimers();
    const label = action[0]!.toUpperCase() + action.slice(1);
    let resolveOperation!: (status: RuntimeStatus) => void;
    const operationRequest = new Promise<RuntimeStatus>((resolve) => {
      resolveOperation = resolve;
    });
    const initialRuntime = action === 'start' ? stoppedStatus : runningStatus('previous-process');
    const nextRuntime = action === 'stop' ? stoppedStatus : runningStatus('next-process');
    const runtimeAction = vi.fn().mockReturnValue(operationRequest);
    const getRuntimeStatus = vi.fn().mockResolvedValue(initialRuntime);
    const client = createMockApiClient({
      [`${action}Runtime`]: runtimeAction,
      getRuntimeStatus,
      getTrafficStatus: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
    });
    const { container } = renderBanner(client);
    await act(async () => Promise.resolve());

    if (action === 'restart') {
      fireEvent.click(screen.getByRole('button', { name: 'More panel actions' }));
      fireEvent.click(screen.getByRole('menuitem', { name: label }));
    } else {
      fireEvent.click(screen.getByRole('button', { name: label }));
    }
    if (action !== 'start') fireEvent.click(screen.getByRole('button', { name: `${label} sing-box` }));

    const actions = container.querySelector('.telemetry-banner__actions')!;
    expect(screen.getByText(`${label} in progress…`)).toBeInTheDocument();
    expect(actions.querySelector('button')).toBeNull();
    expect(actions.querySelectorAll('[data-slot="spinner"]')).toHaveLength(1);

    await act(async () => resolveOperation(initialRuntime));
    expect(screen.getByText(`Verifying ${label}…`)).toBeInTheDocument();
    expect(actions.querySelector('button')).toBeNull();

    getRuntimeStatus.mockResolvedValue(nextRuntime);
    await act(async () => vi.advanceTimersByTimeAsync(750));
    expect(screen.getByText(`${label} verified`)).toBeInTheDocument();
    expect(actions.querySelector('button')).toBeNull();
    expect(actions.querySelector('[data-slot="spinner"]')).toBeNull();

    await act(async () => vi.advanceTimersByTimeAsync(1_999));
    expect(screen.getByText(`${label} verified`)).toBeInTheDocument();
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(screen.queryByText(`${label} verified`)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: action === 'stop' ? 'Start' : 'Stop' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'More panel actions' })).toBeInTheDocument();
    expect(runtimeAction).toHaveBeenCalledTimes(1);
  });

  it('clears failure feedback and restores controls for retry', async () => {
    vi.useFakeTimers();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(stoppedStatus),
      getTrafficStatus: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
      startRuntime: vi.fn().mockRejectedValue(new Error('Unable to start')),
    });
    renderBanner(client);
    await act(async () => Promise.resolve());
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Start' })));

    expect(screen.getByText('Start failed')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'More panel actions' })).not.toBeInTheDocument();
    await act(async () => vi.advanceTimersByTimeAsync(4_999));
    expect(screen.getByText('Start failed')).toBeInTheDocument();
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(screen.queryByText('Start failed')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start' })).toBeInTheDocument();
  });
});
