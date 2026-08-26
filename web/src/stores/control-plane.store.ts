import { createContext, use } from 'react';

import type { CapabilityStatus, DashboardContext } from '@/api/api-client';

export type ControlPlaneState
  = | { status: 'loading'; context: null; message: null }
    | { status: 'error'; context: null; message: string }
    | { status: 'ready'; context: DashboardContext; message: null };

export type ControlPlaneValue = ControlPlaneState & {
  refresh: (signal?: AbortSignal) => Promise<void>;
  setViewVersion: (version: string) => void;
  viewCapability: CapabilityStatus | null;
  viewCapabilityError: unknown | null;
  viewVersion: string;
};

export const ControlPlaneContext = createContext<ControlPlaneValue | null>(null);

export function useControlPlane(): ControlPlaneValue {
  const value = use(ControlPlaneContext);

  if (value === null) {
    throw new Error('useControlPlane must be used within ControlPlaneProvider');
  }

  return value;
}
