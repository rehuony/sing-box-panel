import type { PropsWithChildren } from 'react';

import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ApiClient, MetricsSnapshot, RuntimeStatus } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { useTelemetry } from '@/components/app-shell/use-telemetry';
import { createMockApiClient, testMetrics } from '@/tests/api/mock-api-client';

function wrapper(client: ApiClient) {
  return function ApiWrapper({ children }: PropsWithChildren) {
    return <ApiClientProvider client={client}>{children}</ApiClientProvider>;
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

const runningStatus = {
  desired_running: true,
  target_generation: 1,
  observation_state: 'running',
} as RuntimeStatus;

describe('useTelemetry', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible',
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('polls every ten seconds after the previous request completes', async () => {
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(runningStatus),
      getTrafficStatus: vi.fn().mockResolvedValue(testMetrics),
    });

    renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => Promise.resolve());
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(1);
    expect(client.getTrafficStatus).toHaveBeenCalledTimes(1);

    await act(async () => vi.advanceTimersByTimeAsync(9_999));
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(1);

    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(2);
    expect(client.getTrafficStatus).toHaveBeenCalledTimes(2);
  });

  it('refreshes immediately when the page becomes visible', async () => {
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue(runningStatus),
      getTrafficStatus: vi.fn().mockResolvedValue(testMetrics),
    });

    renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => Promise.resolve());

    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(2);
    expect(client.getTrafficStatus).toHaveBeenCalledTimes(2);
  });

  it('does not overlap a visibility refresh with an in-flight poll', async () => {
    const runtime = deferred<RuntimeStatus>();
    const traffic = deferred<MetricsSnapshot>();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockReturnValue(runtime.promise),
      getTrafficStatus: vi.fn().mockReturnValue(traffic.promise),
    });

    renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => Promise.resolve());
    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    await act(async () => vi.advanceTimersByTimeAsync(20_000));

    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(1);
    expect(client.getTrafficStatus).toHaveBeenCalledTimes(1);

    await act(async () => {
      runtime.resolve(runningStatus);
      traffic.resolve(testMetrics);
      await Promise.all([runtime.promise, traffic.promise]);
    });
  });

  it('clears old evidence after either source fails', async () => {
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn()
        .mockResolvedValueOnce(runningStatus)
        .mockRejectedValueOnce(new Error('runtime unavailable')),
      getTrafficStatus: vi.fn()
        .mockResolvedValueOnce(testMetrics)
        .mockRejectedValueOnce(new Error('traffic unavailable')),
    });
    const { result } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });

    await act(async () => Promise.resolve());
    expect(result.current.runtimeStatus).toBe(runningStatus);
    expect(result.current.snapshot).toBe(testMetrics);

    await act(async () => {
      document.dispatchEvent(new Event('visibilitychange'));
      await Promise.resolve();
    });
    expect(result.current.runtimeStatus).toBeNull();
    expect(result.current.snapshot).toBeNull();
    expect(result.current.runtimeError).toBeInstanceOf(Error);
    expect(result.current.trafficError).toBeInstanceOf(Error);
  });

  it('aborts the active request and removes visibility listeners on unmount', async () => {
    let runtimeSignal: AbortSignal | undefined;
    let trafficSignal: AbortSignal | undefined;
    const runtime = deferred<RuntimeStatus>();
    const traffic = deferred<MetricsSnapshot>();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockImplementation((signal) => {
        runtimeSignal = signal;
        return runtime.promise;
      }),
      getTrafficStatus: vi.fn().mockImplementation((signal) => {
        trafficSignal = signal;
        return traffic.promise;
      }),
    });

    const { unmount } = renderHook(() => useTelemetry(), { wrapper: wrapper(client) });
    await act(async () => Promise.resolve());
    unmount();

    expect(runtimeSignal?.aborted).toBe(true);
    expect(trafficSignal?.aborted).toBe(true);
    document.dispatchEvent(new Event('visibilitychange'));
    expect(client.getRuntimeStatus).toHaveBeenCalledTimes(1);
    expect(client.getTrafficStatus).toHaveBeenCalledTimes(1);

    runtime.reject(new DOMException('Aborted', 'AbortError'));
    traffic.reject(new DOMException('Aborted', 'AbortError'));
    await Promise.allSettled([runtime.promise, traffic.promise]);
  });
});
