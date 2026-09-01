import { useCallback, useEffect, useRef, useState } from 'react';

import type { MetricsSnapshot, RuntimeStatus } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

const POLL_INTERVAL_MS = 10_000;

type TrafficSample = NonNullable<MetricsSnapshot['latest_sample']>;

export type RuntimeStatusEvidence = Omit<RuntimeStatus, 'running'> & {
  running?: NonNullable<RuntimeStatus['running']> & { started_at?: string };
};

export interface TrafficRates {
  uploadBytesPerSecond: number | null;
  downloadBytesPerSecond: number | null;
}

export interface TelemetryState {
  rates: TrafficRates;
  refreshing: boolean;
  refresh: () => Promise<void>;
  runtimeError: unknown | null;
  trafficError: unknown | null;
  snapshot: MetricsSnapshot | null;
  runtimeStatus: RuntimeStatusEvidence | null;
  acceptRuntimeStatus: (status: RuntimeStatus) => void;
}

const emptyRates: TrafficRates = {
  downloadBytesPerSecond: null,
  uploadBytesPerSecond: null,
};

export function deriveTrafficRates(
  previous: TrafficSample | null,
  current: TrafficSample | undefined,
): TrafficRates {
  if (
    previous === null
    || current === undefined
    || !previous.accepted
    || !current.accepted
    || previous.activation_bundle_id !== current.activation_bundle_id
    || previous.process_start_token !== current.process_start_token
  ) {
    return emptyRates;
  }

  const elapsedSeconds
    = (new Date(current.sampled_at).getTime() - new Date(previous.sampled_at).getTime()) / 1_000;
  const uploadDelta = current.upload_total - previous.upload_total;
  const downloadDelta = current.download_total - previous.download_total;

  if (
    !Number.isFinite(elapsedSeconds)
    || elapsedSeconds <= 0
    || uploadDelta < 0
    || downloadDelta < 0
  ) {
    return emptyRates;
  }

  return {
    downloadBytesPerSecond: downloadDelta / elapsedSeconds,
    uploadBytesPerSecond: uploadDelta / elapsedSeconds,
  };
}

export function useTelemetry(): TelemetryState {
  const client = useApiClient();
  const controllerRef = useRef<AbortController | null>(null);
  const refreshPromiseRef = useRef<Promise<void> | null>(null);
  const previousSampleRef = useRef<TrafficSample | null>(null);
  const refreshRequestRef = useRef(0);
  const runtimeRequestRef = useRef(0);
  const trafficRequestRef = useRef(0);
  const [snapshot, setSnapshot] = useState<MetricsSnapshot | null>(null);
  const [runtimeStatus, setRuntimeStatus] = useState<RuntimeStatusEvidence | null>(null);
  const [rates, setRates] = useState<TrafficRates>(emptyRates);
  const [runtimeError, setRuntimeError] = useState<unknown | null>(null);
  const [trafficError, setTrafficError] = useState<unknown | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const acceptRuntimeStatus = useCallback((status: RuntimeStatus) => {
    runtimeRequestRef.current += 1;
    setRuntimeError(null);
    setRuntimeStatus(status as RuntimeStatusEvidence);
  }, []);

  const refresh = useCallback(() => {
    if (refreshPromiseRef.current !== null) return refreshPromiseRef.current;

    const refreshRequest = refreshRequestRef.current + 1;
    const runtimeRequest = runtimeRequestRef.current + 1;
    const trafficRequest = trafficRequestRef.current + 1;
    const controller = new AbortController();
    refreshRequestRef.current = refreshRequest;
    runtimeRequestRef.current = runtimeRequest;
    trafficRequestRef.current = trafficRequest;
    controllerRef.current = controller;
    setRefreshing(true);
    const request = (async () => {
      try {
        const [trafficResult, runtimeResult] = await Promise.allSettled([
          client.getTrafficStatus(controller.signal),
          client.getRuntimeStatus(controller.signal),
        ]);

        if (trafficRequestRef.current === trafficRequest && trafficResult.status === 'fulfilled') {
          const next = trafficResult.value;
          const acceptedSample = next.available && next.latest_sample?.accepted === true
            ? next.latest_sample
            : undefined;
          setRates(deriveTrafficRates(previousSampleRef.current, acceptedSample));
          previousSampleRef.current = acceptedSample ?? null;
          setSnapshot(next);
          setTrafficError(null);
        } else if (trafficRequestRef.current === trafficRequest) {
          previousSampleRef.current = null;
          setRates(emptyRates);
          setSnapshot(null);
          setTrafficError(trafficResult.status === 'rejected' ? trafficResult.reason : null);
        }
        if (runtimeRequestRef.current === runtimeRequest && runtimeResult.status === 'fulfilled') {
          setRuntimeStatus(runtimeResult.value as RuntimeStatusEvidence);
          setRuntimeError(null);
        } else if (runtimeRequestRef.current === runtimeRequest) {
          setRuntimeStatus(null);
          setRuntimeError(runtimeResult.status === 'rejected' ? runtimeResult.reason : null);
        }
      } catch (error) {
        if (trafficRequestRef.current === trafficRequest) {
          previousSampleRef.current = null;
          setRates(emptyRates);
          setSnapshot(null);
          setTrafficError(error);
        }
        if (runtimeRequestRef.current === runtimeRequest) {
          setRuntimeStatus(null);
          setRuntimeError(error);
        }
      } finally {
        if (controllerRef.current === controller) controllerRef.current = null;
        if (refreshRequestRef.current === refreshRequest) setRefreshing(false);
      }
    })();

    refreshPromiseRef.current = request;
    void request.finally(() => {
      if (refreshPromiseRef.current === request) refreshPromiseRef.current = null;
    });
    return request;
  }, [client]);

  useEffect(() => {
    let disposed = false;
    let polling = false;
    let timer: number | undefined;

    function clearTimer() {
      if (timer === undefined) return;
      window.clearTimeout(timer);
      timer = undefined;
    }

    function schedulePoll() {
      if (disposed) return;
      clearTimer();
      timer = window.setTimeout(() => void poll(), POLL_INTERVAL_MS);
    }

    async function poll() {
      if (disposed || polling) return;
      polling = true;
      clearTimer();
      try {
        await refresh();
      } finally {
        polling = false;
        schedulePoll();
      }
    }

    function refreshWhenVisible() {
      if (document.visibilityState === 'visible') void poll();
    }

    document.addEventListener('visibilitychange', refreshWhenVisible);
    void poll();
    return () => {
      disposed = true;
      refreshRequestRef.current += 1;
      runtimeRequestRef.current += 1;
      trafficRequestRef.current += 1;
      clearTimer();
      controllerRef.current?.abort();
      controllerRef.current = null;
      refreshPromiseRef.current = null;
      document.removeEventListener('visibilitychange', refreshWhenVisible);
    };
  }, [refresh]);

  return {
    acceptRuntimeStatus,
    rates,
    refresh,
    refreshing,
    runtimeError,
    runtimeStatus,
    snapshot,
    trafficError,
  };
}
