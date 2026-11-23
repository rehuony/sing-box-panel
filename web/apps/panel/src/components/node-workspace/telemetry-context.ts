import { useStore } from 'zustand';
import { createContext, use, useEffect } from 'react';

import type { TelemetryState, TelemetryStore } from './use-telemetry';

import { createTelemetryStore } from './use-telemetry';

export const TelemetryContext = createContext<TelemetryStore | null>(null);
const emptyStore = createTelemetryStore();
const identity = (state: TelemetryState) => state;

export function useSharedTelemetry<T = TelemetryState>(
  selector: (state: TelemetryState) => T = identity as (state: TelemetryState) => T,
): T {
  const store = use(TelemetryContext);
  if (store === null) throw new Error('useSharedTelemetry requires TelemetryProvider');
  return useStore(store, selector);
}
export function useOptionalSharedTelemetry<T = TelemetryState>(
  selector: (state: TelemetryState) => T = identity as (state: TelemetryState) => T,
): T | null {
  const store = use(TelemetryContext);
  const value = useStore(store ?? emptyStore, selector);
  return store ? value : null;
}
export function useDashboardSubscription() {
  const store = use(TelemetryContext);
  useEffect(() => store?.getState().retainDashboard(), [store]);
}
