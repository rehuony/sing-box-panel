import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';

import type { CapabilityStatus, DashboardContext } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';

type ControlPlaneState =
  | { status: 'loading'; context: null; message: null }
  | { status: 'error'; context: null; message: string }
  | { status: 'ready'; context: DashboardContext; message: null };

type ControlPlaneValue = ControlPlaneState & {
  refresh(signal?: AbortSignal): Promise<void>;
  setViewVersion(version: string): void;
  viewCapability: CapabilityStatus | null;
  viewCapabilityError: unknown | null;
  viewVersion: string;
};

const ControlPlaneContext = createContext<ControlPlaneValue | null>(null);

export interface ControlPlaneProviderProps {
  children: ReactNode;
}

export function ControlPlaneProvider({ children }: ControlPlaneProviderProps) {
  const client = useApiClient();
  const [state, setState] = useState<ControlPlaneState>({
    status: 'loading',
    context: null,
    message: null,
  });
  const [viewVersion, setViewVersionState] = useState('');
  const [viewCapability, setViewCapability] = useState<CapabilityStatus | null>(null);
  const [viewCapabilityError, setViewCapabilityError] = useState<unknown | null>(null);

  const setViewVersion = useCallback((version: string) => {
    const normalized = version.trim();
    if (normalized !== '' && !/^\d+\.\d+\.\d+$/.test(normalized)) return;
    setViewCapability(null);
    setViewCapabilityError(null);
    setViewVersionState(normalized);
  }, []);

  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      setState((current) =>
        current.status === 'ready'
          ? current
          : { status: 'loading', context: null, message: null },
      );
      try {
        const context = await client.getDashboardContext(signal);
        if (!signal?.aborted) {
          setState({ status: 'ready', context, message: null });
          setViewVersionState((current) =>
            current !== '' || !/^\d+\.\d+\.\d+$/.test(context.view.exactVersion)
              ? current
              : context.view.exactVersion,
          );
        }
      } catch (error) {
        if (
          signal?.aborted ||
          (error instanceof DOMException && error.name === 'AbortError')
        ) {
          return;
        }
        setState({
          status: 'error',
          context: null,
          message:
            'Control-plane context is unavailable. The panel may still be starting.',
        });
      }
    },
    [client],
  );

  useEffect(() => {
    const controller = new AbortController();
    void refresh(controller.signal);
    return () => controller.abort();
  }, [refresh]);

  useEffect(() => {
    if (viewVersion === '') {
      setViewCapability(null);
      setViewCapabilityError(null);
      return;
    }
    const controller = new AbortController();
    setViewCapability(null);
    setViewCapabilityError(null);
    void client.getCoreCapability(viewVersion, controller.signal)
      .then((capability) => {
        if (!controller.signal.aborted) setViewCapability(capability);
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted || (error instanceof DOMException && error.name === 'AbortError')) return;
        setViewCapabilityError(error);
      });
    return () => controller.abort();
  }, [client, viewVersion]);

  const value = useMemo<ControlPlaneValue>(
    () => ({
      ...state,
      refresh,
      setViewVersion,
      viewCapability,
      viewCapabilityError,
      viewVersion,
    }),
    [refresh, setViewVersion, state, viewCapability, viewCapabilityError, viewVersion],
  );

  return (
    <ControlPlaneContext.Provider value={value}>
      {children}
    </ControlPlaneContext.Provider>
  );
}

export function useControlPlane(): ControlPlaneValue {
  const value = useContext(ControlPlaneContext);
  if (value === null) {
    throw new Error('useControlPlane must be used within ControlPlaneProvider');
  }
  return value;
}
