import type { ReactNode } from 'react';

import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';

import type { ControlPlaneState, ControlPlaneValue } from './control-plane.store';

import { ControlPlaneContext } from './control-plane.store';
import { ConfigurationSessionProvider } from './configuration-session-provider';

export interface ControlPlaneProviderProps {
  children: ReactNode;
}

export function ControlPlaneProvider({ children }: ControlPlaneProviderProps) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
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
        const context = await cache.fetchQuery({ ...queries.context(client), staleTime: 0 });
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
        setState(current => current.status === 'ready' ? { ...current, message: t('shell.error.contextUnavailable') } : { status: 'error', context: null, message: t('shell.error.contextUnavailable') });
      }
    },
    [client, t, cache],
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

  return (
    <ControlPlaneContext value={value}>
      <ConfigurationSessionProvider>{children}</ConfigurationSessionProvider>
    </ControlPlaneContext>
  );
}
