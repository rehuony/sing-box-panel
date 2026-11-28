import userEvent from '@testing-library/user-event';
import { setAppLanguage } from '@shared/i18n/language';
import { toast } from '@shared/components/ui/toast-manager';
import { TooltipProvider } from '@shared/components/ui/tooltip';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';

import type { TelemetryState } from '@/components/node-workspace';
import type { ApiClient, MetricsSnapshot, RuntimeStatus } from '@/api/api-client';

import { ThemeProvider } from '@/theme';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { NodeStrip } from '@/components/node-workspace/node-strip';
import { createTelemetryStore, TelemetryContext, TelemetryProvider } from '@/components/node-workspace';
import {
  createMockApiClient,
  testDashboardSnapshot,
  testMetrics,
  testNode,
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
    collected_at: sampledAt,
    live_sample: {
      activation_bundle_id: 'bundle_18',
      pid: 8124,
      process_start_token: 'process-8124',
      sampled_at: sampledAt,
      memory_bytes: 94_371_840,
      active_connections: 37,
      upload_total: uploadTotal,
      download_total: downloadTotal,
      upload_bytes_per_second: 200,
      download_bytes_per_second: 300,
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
      const metrics = await client.getMetrics(signal);
      yield { metrics, runtime };
    }
    await waitForAbort(signal);
  });
  return render(
    <ApiClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider delay={0}>
          <TestRouter>
            <TelemetryProvider online>
              <NodeStrip node={testNode} />
            </TelemetryProvider>
          </TestRouter>
        </TooltipProvider>
      </ThemeProvider>
    </ApiClientProvider>,
  );
}

describe('node strip', () => {
  beforeEach(async () => {
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
  });

  afterEach(async () => {
    vi.useRealTimers();
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
  });

  it.each(['en', 'zh-CN'] as const)('shows one reconnect toast per outage and closes it on recovery in %s', async (language) => {
    await setAppLanguage(language);
    const client = createMockApiClient();
    const telemetry: Partial<TelemetryState> = {
      acceptRuntimeStatus: vi.fn(),
      dashboardSnapshot: testDashboardSnapshot,
      liveStale: true,
      dashboardError: null,
      runtimeError: null,
      trafficError: null,
      runtimeStatus: stoppedStatus,
      snapshot: testMetrics,
      rates: { uploadBytesPerSecond: 0, downloadBytesPerSecond: 0 },
    };
    const view = (value: Partial<TelemetryState>) => (
      <ApiClientProvider client={client}>
        <TooltipProvider>
          <TestRouter>
            <TelemetryContext value={createTelemetryStore(value)}><NodeStrip node={testNode} /></TelemetryContext>
          </TestRouter>
        </TooltipProvider>
      </ApiClientProvider>
    );
    const addToast = vi.spyOn(toast, 'add');
    const closeToast = vi.spyOn(toast, 'close');
    const { rerender } = render(view(telemetry));
    const message = language === 'en'
      ? 'Live updates are interrupted. Keeping the last observation while reconnecting.'
      : '实时数据暂时中断，正在保留上次观测并重新连接。';
    await waitFor(() => expect(addToast).toHaveBeenCalledOnce());
    expect(addToast).toHaveBeenCalledWith(expect.objectContaining({ description: message, type: 'warning', timeout: 0 }));
    const id = addToast.mock.calls[0][0].id;
    expect(screen.queryByRole('button', { name: message })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: language === 'en' ? 'Start' : '启动' })).toBeEnabled();
    rerender(view({ ...telemetry }));
    expect(addToast).toHaveBeenCalledOnce();
    rerender(view({ ...telemetry, liveStale: false }));
    expect(closeToast).toHaveBeenCalledWith(id);
    rerender(view({ ...telemetry, dashboardSnapshot: null }));
    expect(addToast).toHaveBeenCalledOnce();
    addToast.mockRestore();
    closeToast.mockRestore();
  });

  it('renders stale runtime and traffic evidence as unknown instead of stopped', async () => {
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue({
        desired_running: true,
        target_generation: 3,
        observation_state: 'stale',
      }),
      getMetrics: vi.fn().mockResolvedValue({
        ...testMetrics,
        available: false,
        reason_code: 'stale_collector_sample',
      }),
    });

    renderBanner(client);

    // The state's hover names what it reads, then why.
    expect(await screen.findByTitle(/Runtime evidence is stale$/)).toHaveTextContent('not reported');
    expect(screen.queryByText('Stopped')).not.toBeInTheDocument();
    expect(screen.queryByText('6 KB')).not.toBeInTheDocument();
    expect(screen.queryByText(/^up /)).not.toBeInTheDocument();
    // Traffic it cannot know leaves the strip rather than reading "—".
    expect(screen.queryByText('net')).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Node edge' })).not.toHaveTextContent('—');
    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Stop' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Refresh runtime and traffic status' })).not.toBeInTheDocument();
  });

  it('shows verified identity, uptime and fresh rates', async () => {
    const first = trafficSnapshot('2026-08-30T11:00:00Z', 1_000, 2_000);
    const second = trafficSnapshot('2026-08-30T11:00:02Z', 1_400, 2_600);
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(runningStatus()),
      getMetrics: vi.fn()
        .mockResolvedValueOnce(first)
        .mockResolvedValue(second),
    });

    renderBanner(client);

    expect(await screen.findByText('Running')).toBeInTheDocument();
    expect(screen.getByText('v1.13.19')).toBeInTheDocument();
    expect(screen.queryByText('arm64')).not.toBeInTheDocument();
    expect(screen.getByTitle(/^Started/)).toHaveTextContent(/^up 1h/);
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

  it('keeps the newest traffic between samples instead of blanking it', async () => {
    vi.useFakeTimers();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(runningStatus()),
      getMetrics: vi.fn().mockResolvedValue(trafficSnapshot('2026-08-30T11:00:00Z', 1_000, 2_000)),
    });
    renderBanner(client);
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(screen.getByText('300 B/s')).toBeInTheDocument();
    await act(async () => vi.advanceTimersByTimeAsync(5_000));
    expect(screen.getByText('300 B/s')).toBeInTheDocument();
    expect(screen.getByText('200 B/s')).toBeInTheDocument();
  });

  it('waits for start to finish and verifies the returned runtime', async () => {
    const user = userEvent.setup();
    let finish!: (status: RuntimeStatus) => void;
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(stoppedStatus),
      getMetrics: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
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
      getMetrics: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
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
      getMetrics: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
    });
    const { container } = renderBanner(client);
    await act(async () => Promise.resolve());

    fireEvent.click(screen.getByRole('button', { name: label }));
    if (action !== 'start') fireEvent.click(screen.getByRole('button', { name: `${label} sing-box` }));

    const actions = container.querySelector('[data-slot="runtime-controls"]')!;
    expect(screen.getByText(`${label} in progress…`)).toBeInTheDocument();
    expect(actions.querySelector('button')).toBeNull();

    await act(async () => resolveOperation(initialRuntime));
    expect(screen.getByText(`Verifying ${label}…`)).toBeInTheDocument();
    expect(actions.querySelector('button')).toBeNull();

    getRuntimeStatus.mockResolvedValue(nextRuntime);
    await act(async () => vi.advanceTimersByTimeAsync(750));
    expect(screen.getByText(`${label} verified`)).toBeInTheDocument();
    expect(actions.querySelector('button')).toBeNull();

    await act(async () => vi.advanceTimersByTimeAsync(1_999));
    expect(screen.getByText(`${label} verified`)).toBeInTheDocument();
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(screen.queryByText(`${label} verified`)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: action === 'stop' ? 'Start' : 'Stop' })).toBeInTheDocument();
    expect(runtimeAction).toHaveBeenCalledTimes(1);
  });

  it('clears failure feedback and restores controls for retry', async () => {
    vi.useFakeTimers();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(stoppedStatus),
      getMetrics: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
      startRuntime: vi.fn().mockRejectedValue(new Error('Unable to start')),
    });
    renderBanner(client);
    await act(async () => Promise.resolve());
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Start' })));

    expect(screen.getByText('Start failed')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    await act(async () => vi.advanceTimersByTimeAsync(4_999));
    expect(screen.getByText('Start failed')).toBeInTheDocument();
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(screen.queryByText('Start failed')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start' })).toBeInTheDocument();
  });
});
