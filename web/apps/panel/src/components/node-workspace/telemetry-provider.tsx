import type { ReactNode } from 'react';

import { useTelemetryStore } from './use-telemetry';
import { TelemetryContext } from './telemetry-context';

/**
 * Holds the node's telemetry for its whole workspace, so nothing below it
 * remounts when the node goes online or offline. It shares the store only
 * while the node is online; offline, its views read no telemetry.
 */
export function TelemetryProvider({ children, online }: { children: ReactNode; online: boolean }) {
  const telemetry = useTelemetryStore(online);
  return <TelemetryContext value={online ? telemetry : null}>{children}</TelemetryContext>;
}
