import { use, useCallback, useEffect, useRef, useState } from 'react';

import type { DashboardStreamSnapshot, MetricsSnapshot, RuntimeStatus } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { PanelSettingsContext } from '@/stores/panel-settings.store';

const MAX_RECONNECT_DELAY_MS = 30_000;

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
  dashboardStale: boolean;
  runtimeError: unknown | null;
  trafficError: unknown | null;
  dashboardError: unknown | null;
  snapshot: MetricsSnapshot | null;
  runtimeStatus: RuntimeStatusEvidence | null;
  dashboardSnapshot: DashboardStreamSnapshot | null;
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
  const settings = use(PanelSettingsContext);
  const settingsRevision = settings?.view?.revision;
  const previousSettingsRevisionRef = useRef<number | undefined>(undefined);
  const previousSampleRef = useRef<TrafficSample | null>(null);
  const [snapshot, setSnapshot] = useState<MetricsSnapshot | null>(null);
  const [runtimeStatus, setRuntimeStatus] = useState<RuntimeStatusEvidence | null>(null);
  const [rates, setRates] = useState<TrafficRates>(emptyRates);
  const [runtimeError, setRuntimeError] = useState<unknown | null>(null);
  const [trafficError, setTrafficError] = useState<unknown | null>(null);
  const [dashboardSnapshot, setDashboardSnapshot] = useState<DashboardStreamSnapshot | null>(null);
  const [dashboardError, setDashboardError] = useState<unknown | null>(null);
  const [dashboardStale, setDashboardStale] = useState(true);

  useEffect(() => {
    const previous = previousSettingsRevisionRef.current;
    previousSettingsRevisionRef.current = settingsRevision;
    if (previous === undefined || previous === settingsRevision) return;
    const controller = new AbortController();
    void client.getMetrics(controller.signal).then(next => {
      if (!controller.signal.aborted) {
        setTrafficError(null);
        setSnapshot(current => current && Date.parse(current.collected_at) > Date.parse(next.collected_at)
          ? current
          : next);
      }
    }).catch(error => {
      if (!controller.signal.aborted) setTrafficError(error);
    });
    return () => controller.abort();
  }, [client, settingsRevision]);

  const acceptRuntimeStatus = useCallback((status: RuntimeStatus) => {
    setRuntimeError(null);
    setRuntimeStatus(status as RuntimeStatusEvidence);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    let retry: ReturnType<typeof setTimeout> | undefined;
    let attempt = 0;
    async function connect() {
      try {
        for await (const event of client.streamMetrics(controller.signal)) {
          if (controller.signal.aborted) return;
          attempt = 0;
          const sample
            = event.metrics.available && event.metrics.latest_sample?.accepted
              ? event.metrics.latest_sample
              : undefined;
          if (!sample || sample.id !== previousSampleRef.current?.id) {
            setRates(deriveTrafficRates(previousSampleRef.current, sample));
          }
          previousSampleRef.current = sample ?? null;
          setSnapshot(current => current && Date.parse(current.collected_at) > Date.parse(event.metrics.collected_at)
            ? current
            : event.metrics);
          setRuntimeStatus(event.runtime);
          setTrafficError(null);
          setRuntimeError(null);
        }
      } catch (error) {
        if (!controller.signal.aborted) {
          setTrafficError(error);
          setRuntimeError(error);
        }
      }
      if (!controller.signal.aborted) {
        const delay = Math.min(1_000 * 2 ** attempt++, MAX_RECONNECT_DELAY_MS);
        retry = setTimeout(() => void connect(), delay);
      }
    }
    void connect();
    return () => {
      controller.abort();
      clearTimeout(retry);
    };
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    let retry: ReturnType<typeof setTimeout> | undefined;
    let attempt = 0;
    async function connect() {
      try {
        for await (const next of client.streamDashboard(controller.signal)) {
          if (controller.signal.aborted) return;
          attempt = 0;
          setDashboardSnapshot(next);
          setDashboardError(null);
          setDashboardStale(false);
        }
      } catch (error) {
        if (!controller.signal.aborted) setDashboardError(error);
      }
      if (!controller.signal.aborted) {
        setDashboardStale(true);
        const delay = Math.min(1_000 * 2 ** attempt++, MAX_RECONNECT_DELAY_MS);
        retry = setTimeout(() => void connect(), delay);
      }
    }
    void connect();
    return () => {
      controller.abort();
      clearTimeout(retry);
    };
  }, [client]);

  return {
    acceptRuntimeStatus,
    dashboardError,
    dashboardSnapshot,
    dashboardStale,
    rates,
    runtimeError,
    runtimeStatus,
    snapshot,
    trafficError,
  };
}
