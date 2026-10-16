import type { PropsWithChildren } from 'react';

import { useStore } from 'zustand';
import { useEffect, useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import type { ApiClient, MetricsSnapshot, RuntimeStatus } from '@/api/api-client';

import { StreamRotation } from '@/api/http/event-stream';
import { ApiClientProvider } from '@/api/api-client-context';
import { PanelSettingsContext } from '@/stores/panel-settings.store';
import { useTelemetryStore } from '@/components/app-shell/use-telemetry';
import {
  createMockApiClient,
  testDashboardSnapshot,
  testMetrics,
  testRuntimeStatus,
} from '@/tests/api/mock-api-client';

function useTelemetry() {
  const store = useTelemetryStore();
  useEffect(() => store.getState().retainDashboard(), [store]);
  return useStore(store);
}

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

function liveMetrics(second: number, rate: number, collectedSecond = second): MetricsSnapshot {
  const base = Date.parse('2026-10-16T00:00:00Z');
  return {
    ...testMetrics,
    collected_at: new Date(base + collectedSecond * 1000).toISOString(),
    live_sample: {
      activation_bundle_id: 'bundle_18', pid: 8124, process_start_token: 'process-8124',
      sampled_at: new Date(base + second * 1000).toISOString(),
      memory_bytes: 1000, active_connections: 2, accepted: true,
      upload_total: second * 10000, download_total: second * 20000,
      upload_bytes_per_second: rate, download_bytes_per_second: rate * 2,
    },
  };
}

describe('useTelemetry', () => {
  afterEach(() => vi.useRealTimers());

  it('uses collector rates immediately and after skipped stream samples, preserving process boundaries', async () => {
    let next!: () => void;
    const pending = new Promise<void>(resolve => {
      next = resolve;
    });
    let restart!: () => void;
    const restarted = new Promise<void>(resolve => {
      restart = resolve;
    });
    const client = createMockApiClient({
      streamMetrics: vi.fn(async function* (signal) {
        yield { metrics: liveMetrics(0, 200), runtime: testRuntimeStatus };
        await pending;
        yield { metrics: liveMetrics(4, 900), runtime: testRuntimeStatus };
        await restarted;
        const metrics = liveMetrics(5, 500);
        metrics.live_sample!.process_start_token = 'restarted-process';
        yield { metrics, runtime: testRuntimeStatus };
        await waitForAbort(signal);
      }),
    });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await waitFor(() => expect(result.current.rates.uploadBytesPerSecond).toBe(200));
    await act(async () => next());
    expect(result.current.rates).toEqual({ uploadBytesPerSecond: 900, downloadBytesPerSecond: 1800 });
    await act(async () => restart());
    expect(result.current.rates.uploadBytesPerSecond).toBe(500);
    expect(result.current.liveTail.at(-2)?.uploadBytesPerSecond).toBeNull();
    expect(result.current.liveTail.at(-1)?.uploadBytesPerSecond).toBe(500);
  });

  it('expires rates without new samples and rejects old samples inside fresh envelopes', async () => {
    vi.useFakeTimers();
    let repeat!: () => void;
    const repeated = new Promise<void>(resolve => {
      repeat = resolve;
    });
    let recover!: () => void;
    const recovered = new Promise<void>(resolve => {
      recover = resolve;
    });
    const client = createMockApiClient({
      streamMetrics: vi.fn(async function* (signal) {
        yield { metrics: liveMetrics(0, 200), runtime: testRuntimeStatus };
        await repeated;
        yield { metrics: liveMetrics(0, 200, 4), runtime: testRuntimeStatus };
        await recovered;
        yield { metrics: liveMetrics(5, 0), runtime: testRuntimeStatus };
        await waitForAbort(signal);
      }),
    });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => {});
    expect(result.current.rates.uploadBytesPerSecond).toBe(200);
    await act(async () => vi.advanceTimersByTimeAsync(3000));
    expect(result.current.rates.uploadBytesPerSecond).toBeNull();
    await act(async () => repeat());
    expect(result.current.liveStale).toBe(false);
    expect(result.current.rates.uploadBytesPerSecond).toBeNull();
    await act(async () => recover());
    expect(result.current.rates.uploadBytesPerSecond).toBe(0);
  });

  it('shows the first fresh collector rate after reconnect without drawing across the interruption', async () => {
    vi.useFakeTimers();
    let calls = 0;
    const client = createMockApiClient({
      streamMetrics: vi.fn(async function* (signal) {
        const first = ++calls === 1;
        yield { metrics: liveMetrics(first ? 0 : 1, first ? 200 : 400), runtime: testRuntimeStatus };
        if (first) throw new Error('stream interrupted');
        await waitForAbort(signal);
      }),
    });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => vi.advanceTimersByTimeAsync(1000));
    expect(result.current.rates.uploadBytesPerSecond).toBe(400);
    expect(result.current.liveTail.map(point => point.uploadBytesPerSecond)).toEqual([200, null, 400]);
  });

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
        else throw new StreamRotation();
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

  it('separates a failed reconnect from data freshness and clears both on recovery', async () => {
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
    expect(result.current.dashboardStale).toBe(false);
    expect(result.current.dashboardSnapshot).toBe(testDashboardSnapshot);
    expect(result.current.dashboardError).toBeInstanceOf(Error);
    await act(async () => vi.advanceTimersByTimeAsync(41_000));
    expect(result.current.dashboardStale).toBe(true);

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

  it('reports an interrupted stream after the grace period and retains the last dashboard snapshot while reconnecting', async () => {
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
    expect(result.current.dashboardStale).toBe(false);
    expect(result.current.dashboardError).toBeNull();
    await act(async () => vi.advanceTimersByTimeAsync(5_000));
    expect(result.current.dashboardError).toBeInstanceOf(Error);
    await act(async () => vi.advanceTimersByTimeAsync(1_000));
    expect(streamDashboard).toHaveBeenCalledTimes(2);
    expect(result.current.dashboardSnapshot).toBe(testDashboardSnapshot);
  });

  it('suppresses a transient initial dashboard failure and clears the error after a successful reconnect', async () => {
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
    expect(result.current.dashboardError).toBeNull();
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
  it('keeps live samples continuous across tab switches while pausing history', async () => {
    let background!: () => void;
    const hidden = new Promise<void>(resolve => {
      background = resolve;
    });
    let foreground!: () => void;
    const visible = new Promise<void>(resolve => {
      foreground = resolve;
    });
    const client = createMockApiClient({
      streamMetrics: vi.fn(async function* (signal) {
        yield { metrics: liveMetrics(0, 200), runtime: testRuntimeStatus };
        await hidden;
        for (let second = 1; second <= 5; second++) {
          yield { metrics: liveMetrics(second, 200 + second), runtime: testRuntimeStatus };
        }
        await visible;
        yield { metrics: liveMetrics(6, 206), runtime: testRuntimeStatus };
        await waitForAbort(signal);
      }),
    });
    const { result } = renderHook(() => useTelemetryStore(), { wrapper: wrapper(client) });
    await waitFor(() => expect(result.current.getState().rates.uploadBytesPerSecond).toBe(200));
    expect(client.streamDashboard).not.toHaveBeenCalled();
    let release!: () => void;
    act(() => {
      release = result.current.getState().retainDashboard();
    });
    await waitFor(() => expect(client.streamDashboard).toHaveBeenCalledOnce());
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    act(() => document.dispatchEvent(new Event('visibilitychange')));
    expect(client.streamMetrics.mock.calls[0][0]?.aborted).toBe(false);
    expect(client.streamDashboard.mock.calls[0][0]?.aborted).toBe(true);
    await act(async () => background());
    expect(result.current.getState().rates.uploadBytesPerSecond).toBe(205);
    visibility.mockReturnValue('visible');
    act(() => document.dispatchEvent(new Event('visibilitychange')));
    await act(async () => foreground());
    expect(client.streamMetrics).toHaveBeenCalledOnce();
    expect(result.current.getState().liveTail.map(point => point.uploadBytesPerSecond))
      .toEqual([200, 201, 202, 203, 204, 205, 206]);
    act(() => release());
    expect(client.streamDashboard.mock.calls[1][0]?.aborted).toBe(true);
    visibility.mockRestore();
  });
});
