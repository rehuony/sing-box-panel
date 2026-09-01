import { createContext, use } from 'react';

import type { TelemetryState } from './use-telemetry';

export const TelemetryContext = createContext<TelemetryState | null>(null);

export function useSharedTelemetry(): TelemetryState {
  const value = use(TelemetryContext);
  if (value === null) throw new Error('useSharedTelemetry requires TelemetryProvider');
  return value;
}

export function useOptionalSharedTelemetry(): TelemetryState | null {
  return use(TelemetryContext);
}
