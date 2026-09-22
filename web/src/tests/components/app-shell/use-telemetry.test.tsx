import type { PropsWithChildren } from 'react';

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import type { ApiClient, RuntimeStatus } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { useTelemetry } from '@/components/app-shell/use-telemetry';
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

  it('retains the last dashboard snapshot while reconnecting', async () => {
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
