import { createStore, useStore } from 'zustand';
import { replaceEqualDeep } from '@tanstack/react-query';
import { use, useEffect, useRef, useState } from 'react';

import type { DashboardStreamSnapshot, MetricsSnapshot, RuntimeStatus } from '@/api/api-client';

import { followStream } from '@/api/stream-connection';
import { useApiClient } from '@/api/api-client-context';
import { usePageVisible } from '@/hooks/use-page-visible';
import { PanelSettingsContext } from '@/stores/panel-settings.store';

type TrafficSample = NonNullable<MetricsSnapshot['live_sample']>;
export type RuntimeStatusEvidence = Omit<RuntimeStatus, 'running'> & {
  running?: NonNullable<RuntimeStatus['running']> & { started_at?: string };
};
export interface TrafficRates {
  uploadBytesPerSecond: number | null;
  downloadBytesPerSecond: number | null;
}
export interface LivePoint extends TrafficRates {
  at: number;
  connections: number | null;
}
export interface TelemetryState {
  liveStale: boolean;
  rates: TrafficRates;
  liveTail: LivePoint[];
  dashboardStale: boolean;
  dashboardSubscribers: number;
  runtimeError: unknown | null;
  trafficError: unknown | null;
  dashboardError: unknown | null;
  snapshot: MetricsSnapshot | null;
  retainDashboard: () => () => void;
  runtimeStatus: RuntimeStatusEvidence | null;
  dashboardSnapshot: DashboardStreamSnapshot | null;
  acceptRuntimeStatus: (status: RuntimeStatus, ifCurrent?: RuntimeStatusEvidence | null) => void;
}
const emptyRates: TrafficRates = { downloadBytesPerSecond: null, uploadBytesPerSecond: null };

export function deriveTrafficRates(previous: TrafficSample | null, current: TrafficSample | undefined): TrafficRates {
  if (!previous?.accepted || !current?.accepted
    || previous.activation_bundle_id !== current.activation_bundle_id
    || previous.pid !== current.pid || previous.process_start_token !== current.process_start_token) {
    return emptyRates;
  }
  const seconds = (Date.parse(current.sampled_at) - Date.parse(previous.sampled_at)) / 1000;
  const upload = current.upload_total - previous.upload_total;
  const download = current.download_total - previous.download_total;
  if (!Number.isFinite(seconds) || seconds <= 0 || seconds > 3
    || !Number.isFinite(upload) || !Number.isFinite(download) || upload < 0 || download < 0) {
    return emptyRates;
  }
  return { uploadBytesPerSecond: upload / seconds, downloadBytesPerSecond: download / seconds };
}

export function createTelemetryStore(initial: Partial<TelemetryState> = {}) {
  return createStore<TelemetryState>((set) => ({
    rates: emptyRates, liveTail: [], liveStale: true, dashboardStale: true,
    runtimeError: null, trafficError: null, dashboardError: null,
    snapshot: null, runtimeStatus: null, dashboardSnapshot: null, dashboardSubscribers: 0,
    acceptRuntimeStatus: (status, ifCurrent) => set(current => {
      if (ifCurrent !== undefined && current.runtimeStatus !== ifCurrent) return current;
      return { runtimeStatus: replaceEqualDeep(current.runtimeStatus, status), runtimeError: null };
    }),
    retainDashboard: () => {
      set(s => ({ dashboardSubscribers: s.dashboardSubscribers + 1 }));
      let released = false;
      return () => {
        if (!released) {
          released = true;
          set(s => ({ dashboardSubscribers: s.dashboardSubscribers - 1 }));
        }
      };
    },
    ...initial,
  }));
}
export type TelemetryStore = ReturnType<typeof createTelemetryStore>;

/** The provider owns stable state; components subscribe only to their fields. */
export function useTelemetryStore(): TelemetryStore {
  const client = useApiClient();
  const settings = use(PanelSettingsContext);
  const revision = settings?.view?.revision;
  const previousRevisionRef = useRef(revision);
  const visible = usePageVisible();
  const [store] = useState(createTelemetryStore);
  const needsDashboard = useStore(store, s => s.dashboardSubscribers > 0);

  useEffect(() => {
    const previous = previousRevisionRef.current;
    previousRevisionRef.current = revision;
    if (previous === undefined || previous === revision || !visible) return;
    const controller = new AbortController();
    void client.getMetrics(controller.signal).then(snapshot => {
      if (!controller.signal.aborted && Date.parse(store.getState().snapshot?.collected_at ?? '1970-01-01') <= Date.parse(snapshot.collected_at)) store.setState({ snapshot });
    }).catch(() => undefined);
    return () => controller.abort();
  }, [client, store, revision, visible]);

  useEffect(() => {
    if (!visible) return;
    let previous: TrafficSample | null = null;
    let receivedAt = Date.now();
    const stop = followStream(signal => client.streamMetrics(signal), event => {
      receivedAt = Date.now();
      const state = store.getState();
      if (state.snapshot && Date.parse(event.metrics.collected_at) < Date.parse(state.snapshot.collected_at)) return;
      const sample = event.metrics.live_sample;
      const repeated = sample && sample.sampled_at === previous?.sampled_at
        && sample.pid === previous.pid && sample.process_start_token === previous.process_start_token
        && sample.activation_bundle_id === previous.activation_bundle_id;
      const rates = repeated ? state.rates : deriveTrafficRates(previous, sample);
      const at = sample ? Date.parse(sample.sampled_at) : Date.parse(event.metrics.collected_at);
      const point: LivePoint = {
        at, ...rates, connections: sample?.accepted && previous && rates.uploadBytesPerSecond !== null
          ? sample.active_connections
          : null,
      };
      const tail = repeated ? state.liveTail : [...state.liveTail.filter(p => p.at > at - 120_000 && p.at < at), point];
      previous = sample?.accepted ? sample : null;
      store.setState({ snapshot: event.metrics, runtimeStatus: replaceEqualDeep(state.runtimeStatus, event.runtime),
        rates, liveTail: tail, trafficError: null, runtimeError: null, liveStale: false });
    }, error => store.setState({ trafficError: error, runtimeError: error }), () => {
      previous = null;
    });
    const freshness = setInterval(() => {
      if (Date.now() - receivedAt > 10_000 && !store.getState().liveStale) {
        store.setState({ liveStale: true, rates: emptyRates });
      }
    }, 1000);
    return () => {
      stop();
      clearInterval(freshness);
    };
  }, [client, store, visible]);

  useEffect(() => {
    if (!visible || !needsDashboard) return;
    let receivedAt = Date.now();
    const stop = followStream(signal => client.streamDashboard(signal), dashboardSnapshot => {
      receivedAt = Date.now();
      const current = store.getState().dashboardSnapshot;
      if (current && Date.parse(current.collected_at) > Date.parse(dashboardSnapshot.collected_at)) return;
      store.setState({ dashboardSnapshot, dashboardError: null, dashboardStale: false });
    }, dashboardError => store.setState({ dashboardError }), () => {});
    const freshness = setInterval(() => {
      if (Date.now() - receivedAt > 45_000 && !store.getState().dashboardStale) {
        store.setState({ dashboardStale: true });
      }
    }, 1000);
    return () => {
      stop();
      clearInterval(freshness);
    };
  }, [client, store, visible, needsDashboard]);
  return store;
}
