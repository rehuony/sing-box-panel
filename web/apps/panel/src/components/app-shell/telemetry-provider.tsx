import type { ReactNode } from 'react';

import { useTelemetryStore } from './use-telemetry';
import { TelemetryContext } from './telemetry-context';

export function TelemetryProvider({ children }: { children: ReactNode }) {
  const telemetry = useTelemetryStore();
  return <TelemetryContext value={telemetry}>{children}</TelemetryContext>;
}
