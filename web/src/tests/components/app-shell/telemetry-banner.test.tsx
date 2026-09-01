import userEvent from '@testing-library/user-event';
import { act, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ApiClient, MetricsSnapshot, RuntimeStatus, Task } from '@/api/api-client';

import { setAppLanguage } from '@/i18n';
import { ThemeProvider } from '@/theme';
import { SidebarProvider } from '@/components/ui/sidebar';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { TelemetryBanner } from '@/components/app-shell/telemetry-banner';
import { TelemetryProvider } from '@/components/app-shell/telemetry-provider';
import {
  createMockApiClient,
  testMetrics,
  testTask,
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
    desired_running: true,
    applied_bundle_id: 'bundle_18',
    target_generation: 3,
    observation_state: 'running',
    running: runtimeIdentity(processStartToken),
  } as RuntimeStatus;
}

const stoppedStatus: RuntimeStatus = {
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

function renderBanner(client: ApiClient) {
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
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
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

    expect(await screen.findByText('Runtime evidence is stale')).toBeInTheDocument();
    expect(screen.queryByText('Stopped')).not.toBeInTheDocument();
    expect(screen.queryByText('6 KB')).not.toBeInTheDocument();
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(5);
    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Stop' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument();
    expect(container.querySelector('.telemetry-banner__identity [data-slot="separator"]')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Refresh runtime and traffic status' })).not.toBeInTheDocument();
  });

  it('shows verified identity, uptime, fresh rates and current-period total', async () => {
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
    expect(screen.getByText('1.13.19')).toBeInTheDocument();
    expect(screen.getByRole('group', { name: /Version: 1\.13\.19/ })).toBeInTheDocument();
    expect(screen.getByRole('group', { name: /Uptime: 1h.*Started/ })).toBeInTheDocument();
    expect(screen.queryByText('Version')).not.toBeInTheDocument();
    expect(screen.queryByText('Uptime')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stop' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Restart' })).toBeInTheDocument();
    expect(screen.getByText('6 KB')).toBeInTheDocument();

    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    expect(await screen.findByText('200 B/s')).toBeInTheDocument();
    expect(screen.getByText('300 B/s')).toBeInTheDocument();
  });

  it('tracks the durable start task and verifies the observed runtime', async () => {
    const user = userEvent.setup();
    const queuedTask: Task = {
      ...testTask,
      id: 'task_runtime_start',
      kind: 'runtime-start',
      status: 'queued',
    };
    const getTask = vi.fn()
      .mockResolvedValueOnce({ ...queuedTask, status: 'running' })
      .mockResolvedValueOnce({ ...queuedTask, status: 'succeeded' });
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn()
        .mockResolvedValueOnce(stoppedStatus)
        .mockResolvedValue(runningStatus()),
      getTrafficStatus: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
      getTask,
      startRuntime: vi.fn().mockResolvedValue(queuedTask),
    });

    renderBanner(client);
    await user.click(await screen.findByRole('button', { name: 'Start' }));

    expect(client.startRuntime).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('task_runtime_start')).toBeInTheDocument();
    expect(await screen.findByText('Start verified', {}, { timeout: 3_500 })).toBeInTheDocument();
    expect(getTask).toHaveBeenCalledTimes(2);
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('requires confirmation for stop and restart actions', async () => {
    const user = userEvent.setup();
    const restartTask: Task = {
      ...testTask,
      id: 'task_runtime_restart',
      kind: 'runtime-restart',
      status: 'succeeded',
    };
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn()
        .mockResolvedValueOnce(runningStatus('process-old'))
        .mockResolvedValue(runningStatus('process-new')),
      getTrafficStatus: vi.fn().mockResolvedValue({ ...testMetrics, available: false }),
      restartRuntime: vi.fn().mockResolvedValue(restartTask),
    });

    renderBanner(client);
    await screen.findByText('Running');
    await user.click(screen.getByRole('button', { name: 'Stop' }));
    expect(await screen.findByRole('heading', { name: 'Stop the sing-box runtime?' })).toBeInTheDocument();
    expect(client.stopRuntime).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    await user.click(screen.getByRole('button', { name: 'Restart' }));
    expect(await screen.findByRole('heading', { name: 'Restart the sing-box runtime?' })).toBeInTheDocument();
    expect(client.restartRuntime).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Restart sing-box' }));

    expect(client.restartRuntime).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('Restart verified')).toBeInTheDocument();
  });

  it('switches the explicit language without exposing account actions in the header', async () => {
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
    await user.click(screen.getByRole('button', { name: 'Open language menu' }));
    await user.click(await screen.findByRole('menuitemradio', { name: '简体中文' }));

    await waitFor(() => expect(document.documentElement.lang).toBe('zh-CN'));
    expect(window.localStorage.getItem('sing-box-panel.language')).toBe('zh-CN');
    expect(screen.queryByRole('button', { name: '打开账户菜单' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '退出登录' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '更多面板操作' }));
    expect(await screen.findByRole('menuitem', { name: '启动' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: '刷新运行与流量状态' })).not.toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: '退出登录' })).not.toBeInTheDocument();
  });
});
