import type { ReactNode } from 'react';

import { useTelemetry } from './use-telemetry';
import { TelemetryContext } from './telemetry-context';

export function TelemetryProvider({ children }: { children: ReactNode }) {
  const telemetry = useTelemetry();
  return <TelemetryContext value={telemetry}>{children}</TelemetryContext>;
}
