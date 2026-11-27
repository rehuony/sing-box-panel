import { createStore, useStore } from 'zustand';
import { replaceEqualDeep } from '@tanstack/react-query';
import { use, useEffect, useRef, useState } from 'react';

import type { DashboardStreamSnapshot, MetricsSnapshot, RecentMetrics, RuntimeStatus } from '@/api/api-client';

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
  /**
   * How long the point's rates cover, in milliseconds: a stored sample's
   * interval, or for a stream reading the time since the reading before it.
   */
  span?: number;
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
const idleRates: TrafficRates = { downloadBytesPerSecond: 0, uploadBytesPerSecond: 0 };
/** What the store holds before its node's first reading. */
const noReadings = {
  rates: emptyRates, liveTail: [], liveStale: true, dashboardStale: true,
  runtimeError: null, trafficError: null, dashboardError: null,
  snapshot: null, runtimeStatus: null, dashboardSnapshot: null,
} satisfies Partial<TelemetryState>;

/** A stored sample as a point of the live window. */
function storedPoint(sample: RecentMetrics['samples'][number]): LivePoint {
  return {
    at: Date.parse(sample.sampled_at),
    connections: sample.active_connections,
    downloadBytesPerSecond: sample.download_bytes_per_second,
    uploadBytesPerSecond: sample.upload_bytes_per_second,
    ...(sample.interval_seconds === null ? {} : { span: sample.interval_seconds * 1000 }),
  };
}

/** How far apart a span without a running core draws its known zeros: as far as stored samples are. */
const IDLE_STEP = 10_000;

/**
 * A span in which no core ran as known zeros, one every IDLE_STEP and one at
 * its end, so the window drops only its oldest ones as it moves on.
 */
function idlePoints(idle: RecentMetrics['idle'][number]): LivePoint[] {
  const from = Date.parse(idle.from);
  const to = Date.parse(idle.to);
  const zero = (at: number): LivePoint => ({
    at, connections: 0, downloadBytesPerSecond: 0, span: IDLE_STEP, uploadBytesPerSecond: 0,
  });
  const points: LivePoint[] = [];
  for (let at = from; at < to; at += IDLE_STEP) points.push(zero(at));
  points.push(zero(to));
  return points;
}

/** The rates the panel derived for a sample, or none when it has none it stands behind. */
function sampleRates(sample: TrafficSample | undefined): TrafficRates {
  const upload = sample?.upload_bytes_per_second;
  const download = sample?.download_bytes_per_second;
  if (!sample?.accepted || upload == null || download == null
    || !Number.isFinite(upload) || !Number.isFinite(download) || upload < 0 || download < 0) {
    return emptyRates;
  }
  return { uploadBytesPerSecond: upload, downloadBytesPerSecond: download };
}

export function createTelemetryStore(initial: Partial<TelemetryState> = {}) {
  return createStore<TelemetryState>((set) => ({
    ...noReadings, dashboardSubscribers: 0,
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

/**
 * The provider owns stable state; components subscribe only to their fields.
 * The node's streams run only while it is online; going offline drops its
 * readings, so a node that returns starts from its first new reading.
 */
export function useTelemetryStore(online: boolean): TelemetryStore {
  const client = useApiClient();
  const settings = use(PanelSettingsContext);
  const revision = settings?.view?.revision;
  const previousRevisionRef = useRef(revision);
  const visible = usePageVisible();
  const [store] = useState(createTelemetryStore);
  const needsDashboard = useStore(store, s => s.dashboardSubscribers > 0);

  useEffect(() => {
    if (!online) return;
    return () => store.setState(noReadings);
  }, [online, store]);

  useEffect(() => {
    const previous = previousRevisionRef.current;
    previousRevisionRef.current = revision;
    if (previous === undefined || previous === revision || !visible || !online) return;
    const controller = new AbortController();
    void client.getMetrics(controller.signal).then(snapshot => {
      if (!controller.signal.aborted && Date.parse(store.getState().snapshot?.collected_at ?? '1970-01-01') <= Date.parse(snapshot.collected_at)) store.setState({ snapshot });
    }).catch(() => undefined);
    return () => controller.abort();
  }, [client, store, revision, visible, online]);

  useEffect(() => {
    if (!online) return;
    // Keep live samples flowing across tab switches; only history pauses below.
    let previous: TrafficSample | null = null;
    let receivedAt = Date.now();
    let ratesExpireAt = 0;
    let interrupted = store.getState().liveTail.length > 0;
    store.setState({ rates: emptyRates });
    // The window opens with the node's stored samples of the last two minutes
    // and the spans in which its core did not run, older than anything the
    // stream has brought, so re-entering a node or reloading shows what came
    // before.
    const recent = new AbortController();
    void client.getRecentMetrics(recent.signal).then(({ idle, samples }) => {
      if (recent.signal.aborted) return;
      const tail = store.getState().liveTail;
      const first = tail[0]?.at ?? Number.POSITIVE_INFINITY;
      const stored = [...samples.map(storedPoint), ...idle.flatMap(idlePoints)]
        .filter(point => point.at < first)
        .sort((a, b) => a.at - b.at)
        .filter((point, index, points) => index === 0 || point.at !== points[index - 1]!.at);
      if (stored.length) store.setState({ liveTail: [...stored, ...tail] });
    }).catch(() => undefined);
    const stop = followStream(signal => client.streamMetrics(signal), event => {
      receivedAt = Date.now();
      const state = store.getState();
      if (state.snapshot && Date.parse(event.metrics.collected_at) < Date.parse(state.snapshot.collected_at)) return;
      // A stopped core moves nothing, so its readings are a known zero rather
      // than a gap; a sample it left behind no longer stands.
      const stopped = event.runtime.observation_state === 'stopped';
      const sample = stopped ? undefined : event.metrics.live_sample;
      const repeated = sample && sample.sampled_at === previous?.sampled_at
        && sample.pid === previous.pid && sample.process_start_token === previous.process_start_token
        && sample.activation_bundle_id === previous.activation_bundle_id;
      const at = sample ? Date.parse(sample.sampled_at) : Date.parse(event.metrics.collected_at);
      if (!repeated) ratesExpireAt = receivedAt + 3000 - Math.max(0, Date.parse(event.metrics.collected_at) - at);
      // The current rates are the newest reading's while it is at most three
      // seconds old; the chart keeps every reading's own, however old it
      // arrives, such as the one a node sampling every ten seconds sent last.
      const reading = stopped ? idleRates : sampleRates(sample);
      const rates = stopped || receivedAt < ratesExpireAt ? reading : emptyRates;
      const connections = stopped ? 0 : sample?.accepted ? sample.active_connections : null;
      const point: LivePoint = { at, ...reading, connections };
      const tail = repeated ? state.liveTail : state.liveTail.filter(p => p.at > at - 120_000 && p.at < at);
      if (!repeated) {
        const changedProcess = previous && sample && (previous.pid !== sample.pid
          || previous.process_start_token !== sample.process_start_token
          || previous.activation_bundle_id !== sample.activation_bundle_id);
        const last = tail.at(-1);
        if (interrupted || changedProcess) {
          // Midway, so the cursor at either reading shows that reading.
          if (last && last.at < at - 1) tail.push({ at: (last.at + at) / 2, ...emptyRates, connections: null });
        } else if (previous && sample && reading.uploadBytesPerSecond !== null) {
          point.span = at - Date.parse(previous.sampled_at);
        }
        tail.push(point);
        interrupted = false;
      }
      previous = sample ?? null;
      store.setState({ snapshot: event.metrics, runtimeStatus: replaceEqualDeep(state.runtimeStatus, event.runtime),
        rates, liveTail: tail, trafficError: null, runtimeError: null, liveStale: false });
    }, error => store.setState({ trafficError: error, runtimeError: error }), () => {
      interrupted = true;
    });
    const freshness = setInterval(() => {
      const rates = store.getState().rates;
      if (Date.now() >= ratesExpireAt
        && (rates.uploadBytesPerSecond !== null || rates.downloadBytesPerSecond !== null)) {
        store.setState({ rates: emptyRates });
      }
      if (Date.now() - receivedAt > 10_000 && !store.getState().liveStale) {
        store.setState({ liveStale: true, rates: emptyRates });
      }
    }, 1000);
    return () => {
      recent.abort();
      stop();
      clearInterval(freshness);
    };
  }, [client, store, online]);

  useEffect(() => {
    if (!online || !visible || !needsDashboard) return;
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
  }, [client, store, online, visible, needsDashboard]);
  return store;
}
