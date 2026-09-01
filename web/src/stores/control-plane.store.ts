import { createContext, use } from 'react';

import type { DashboardContext } from '@/api/api-client';

import i18n from '@/i18n';

export type ControlPlaneState
  = | { status: 'loading'; context: null; message: null }
    | { status: 'error'; context: null; message: string }
    | { status: 'ready'; context: DashboardContext; message: null };

export type ControlPlaneValue = ControlPlaneState & {
  refresh: (signal?: AbortSignal) => Promise<void>;
  setViewVersion: (version: string) => void;
  viewVersion: string;
};

export const ControlPlaneContext = createContext<ControlPlaneValue | null>(null);

export function useControlPlane(): ControlPlaneValue {
  const value = use(ControlPlaneContext);

  if (value === null) {
    throw new Error(i18n.t('common.providerRequired', {
      hook: 'useControlPlane',
      provider: 'ControlPlaneProvider',
    }));
  }

  return value;
}
