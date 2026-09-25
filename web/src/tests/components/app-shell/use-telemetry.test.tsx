import type { PropsWithChildren } from 'react';

import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import type { ApiClient, RuntimeStatus } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { useTelemetry } from '@/components/app-shell/use-telemetry';
import { PanelSettingsContext } from '@/stores/panel-settings.store';
import {
  createMockApiClient,
  testDashboardSnapshot,
  testMetrics,
  testRuntimeStatus,
} from '@/tests/api/mock-api-client';

function wrapper(client: ApiClient) {
  return function ApiWrapper({ children }: PropsWithChildren) {
    return <ApiClientProvider client={client}>{children}</ApiClientProvider>;
  };
}

function waitForAbort(signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal?.aborted) resolve();
    else signal?.addEventListener('abort', () => resolve(), { once: true });
  });
}

describe('useTelemetry', () => {
  afterEach(() => vi.useRealTimers());

  it('hydrates live and dashboard streams without periodic GET requests', async () => {
    const client = createMockApiClient({
      streamMetrics: vi.fn(async function* (signal) {
        yield { metrics: testMetrics, runtime: testRuntimeStatus };
        await waitForAbort(signal);
      }),
      streamDashboard: vi.fn(async function* (signal) {
        yield testDashboardSnapshot;
        await waitForAbort(signal);
      }),
    });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });

    await waitFor(() => expect(result.current.dashboardSnapshot).toBe(testDashboardSnapshot));
    expect(result.current.snapshot).toBe(testMetrics);
    expect(result.current.runtimeStatus).toBe(testRuntimeStatus);
    expect(client.getRuntimeStatus).not.toHaveBeenCalled();
    expect(client.getTrafficStatus).not.toHaveBeenCalled();
    expect(client.getMetricsHistory).not.toHaveBeenCalled();
    expect(client.getRuntimeHistory).not.toHaveBeenCalled();
    expect(client.listPanelLogs).not.toHaveBeenCalled();
  });

  it('refreshes metrics once after a settings save without restarting the stream', async () => {
    const client = createMockApiClient({
      streamMetrics: vi.fn(async function* (signal) {
        yield { metrics: testMetrics, runtime: testRuntimeStatus };
        await waitForAbort(signal);
      }),
    });
    const view = await client.getPanelSettings();
    let updateRevision: (value: number) => void = () => {};
    const next = { ...testMetrics, quota_bytes: 800 * 2 ** 30 };
    vi.mocked(client.getMetrics).mockResolvedValue(next);
    function SettingsWrapper({ children }: PropsWithChildren) {
      const [revision, setRevision] = useState(view.revision);
      updateRevision = setRevision;
      return (
        <ApiClientProvider client={client}>
          <PanelSettingsContext value={{
            view: { ...view, revision }, error: null, reload: vi.fn(), preview: vi.fn(),
            accept: vi.fn(), save: client.savePanelSettings,
          }}>
            {children}
          </PanelSettingsContext>
        </ApiClientProvider>
      );
    }
    const { result, rerender } = renderHook(() => useTelemetry(), { wrapper: SettingsWrapper });
    await waitFor(() => expect(result.current.snapshot).toBe(testMetrics));
    expect(client.getMetrics).not.toHaveBeenCalled();
    act(() => updateRevision(view.revision + 1));
    await waitFor(() => expect(result.current.snapshot?.quota_bytes).toBe(next.quota_bytes));
    rerender();
    expect(client.getMetrics).toHaveBeenCalledOnce();
    expect(client.streamMetrics).toHaveBeenCalledOnce();
  });

  it('keeps normal dashboard stream rotation quiet when the next snapshot arrives promptly', async () => {
    vi.useFakeTimers();
    let calls = 0;
    const client = createMockApiClient({
      streamDashboard: vi.fn(async function* (signal) {
        yield testDashboardSnapshot;
        if (++calls > 1) await waitForAbort(signal);
      }),
    });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => Promise.resolve());
    expect(result.current.dashboardStale).toBe(false);

    await act(async () => vi.advanceTimersByTimeAsync(1_000));
    expect(client.streamDashboard).toHaveBeenCalledTimes(2);
    await act(async () => vi.advanceTimersByTimeAsync(5_000));
    expect(result.current.dashboardStale).toBe(false);
    expect(result.current.dashboardError).toBeNull();
    expect(result.current.dashboardSnapshot).toBe(testDashboardSnapshot);
  });

  it('reports a stalled clean reconnect after the grace period and clears it on recovery', async () => {
    vi.useFakeTimers();
    let resume!: () => void;
    const pending = new Promise<void>(resolve => {
      resume = resolve;
    });
    let calls = 0;
    const next = { ...testDashboardSnapshot, collected_at: '2026-09-25T00:00:00Z' };
    const client = createMockApiClient({
      streamDashboard: vi.fn(async function* (signal) {
        if (++calls === 1) {
          yield testDashboardSnapshot;
          return;
        }
        await pending;
        yield next;
        await waitForAbort(signal);
      }),
    });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => Promise.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(4_999));
    expect(result.current.dashboardStale).toBe(false);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(result.current.dashboardStale).toBe(true);
    expect(result.current.dashboardSnapshot).toBe(testDashboardSnapshot);
    expect(result.current.dashboardError).toBeNull();

    await act(async () => resume());
    expect(result.current.dashboardSnapshot).toBe(next);
    expect(result.current.dashboardStale).toBe(false);
  });

  it('cancels the dashboard retry and grace timers on unmount', async () => {
    vi.useFakeTimers();
    const client = createMockApiClient({
      streamDashboard: vi.fn(async function* () {
        yield testDashboardSnapshot;
      }),
    });
    const { unmount } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => Promise.resolve());
    unmount();
    expect(vi.getTimerCount()).toBe(0);
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(client.streamDashboard).toHaveBeenCalledOnce();
  });

  it('reports an interrupted stream immediately and retains the last dashboard snapshot while reconnecting', async () => {
    vi.useFakeTimers();
    let calls = 0;
    const streamDashboard = vi.fn(async function* (signal?: AbortSignal) {
      calls += 1;
      if (calls === 1) {
        yield testDashboardSnapshot;
        throw new Error('stream interrupted');
      }
      await waitForAbort(signal);
    });
    const client = createMockApiClient({ streamDashboard });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(result.current.dashboardSnapshot).toBe(testDashboardSnapshot);
    expect(result.current.dashboardStale).toBe(true);
    expect(result.current.dashboardError).toBeInstanceOf(Error);
    await act(async () => vi.advanceTimersByTimeAsync(1_000));
    expect(streamDashboard).toHaveBeenCalledTimes(2);
    expect(result.current.dashboardSnapshot).toBe(testDashboardSnapshot);
  });

  it('reports initial dashboard failures and clears the error after a successful reconnect', async () => {
    vi.useFakeTimers();
    let calls = 0;
    const failure = new Error('The dashboard snapshot could not be collected.');
    const client = createMockApiClient({
      streamDashboard: vi.fn(async function* (signal) {
        if (++calls === 1) throw failure;
        yield testDashboardSnapshot;
        await waitForAbort(signal);
      }),
    });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });

    await act(async () => Promise.resolve());
    expect(result.current.dashboardSnapshot).toBeNull();
    expect(result.current.dashboardError).toBe(failure);
    expect(result.current.dashboardStale).toBe(true);

    await act(async () => vi.advanceTimersByTimeAsync(1_000));
    expect(result.current.dashboardSnapshot).toBe(testDashboardSnapshot);
    expect(result.current.dashboardError).toBeNull();
    expect(result.current.dashboardStale).toBe(false);
  });

  it('accepts authoritative runtime results from lifecycle operations', async () => {
    const client = createMockApiClient();
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    const stopped = { observation_state: 'stopped' } as RuntimeStatus;

    act(() => result.current.acceptRuntimeStatus(stopped));
    expect(result.current.runtimeStatus).toBe(stopped);
    expect(result.current.runtimeError).toBeNull();
  });

  it('aborts both stream subscriptions on unmount', async () => {
    let metricsSignal: AbortSignal | undefined;
    let dashboardSignal: AbortSignal | undefined;
    const client = createMockApiClient({
      streamMetrics: vi.fn(async function* (signal) {
        metricsSignal = signal;
        await waitForAbort(signal);
      }),
      streamDashboard: vi.fn(async function* (signal) {
        dashboardSignal = signal;
        await waitForAbort(signal);
      }),
    });
    const { unmount } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => Promise.resolve());
    unmount();

    expect(metricsSignal?.aborted).toBe(true);
    expect(dashboardSignal?.aborted).toBe(true);
  });
});
