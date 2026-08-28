import type { ReactNode } from 'react';

import { useCallback, useEffect, useMemo, useState } from 'react';

import { useApiClient } from '@/api/api-client-context';

import type { ControlPlaneState, ControlPlaneValue } from './control-plane.store';

import { ControlPlaneContext } from './control-plane.store';

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
  const [selectedViewVersion, setSelectedViewVersion] = useState('');
  const setViewVersion = useCallback((version: string) => {
    const normalized = version.trim();
    if (normalized !== '' && !/^\d+\.\d+\.\d+$/.test(normalized)) return;
    setSelectedViewVersion(normalized);
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
          setSelectedViewVersion((current) =>
            current !== '' || !/^\d+\.\d+\.\d+$/.test(context.view.exactVersion)
              ? current
              : context.view.exactVersion,
          );
        }
      } catch (error) {
        if (
          signal?.aborted
          || (error instanceof DOMException && error.name === 'AbortError')
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

  const value = useMemo<ControlPlaneValue>(
    () => ({
      ...state,
      refresh,
      setViewVersion,
      viewVersion: selectedViewVersion,
    }),
    [refresh, selectedViewVersion, setViewVersion, state],
  );

  return <ControlPlaneContext value={value}>{children}</ControlPlaneContext>;
}
