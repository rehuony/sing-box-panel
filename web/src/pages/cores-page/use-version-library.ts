import { useShallow } from 'zustand/react/shallow';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useRef, useState } from 'react';

import type { RuntimeStatus } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';

export function useVersionLibrary() {
  const client = useApiClient();
  const cache = useQueryClient();
  const telemetry = useOptionalSharedTelemetry(useShallow(s => ({
    runtimeStatus: s.runtimeStatus, acceptRuntimeStatus: s.acceptRuntimeStatus,
  })));
  const acceptRuntimeStatus = telemetry?.acceptRuntimeStatus;
  const latestRuntimeRef = useRef(telemetry?.runtimeStatus);
  latestRuntimeRef.current = telemetry?.runtimeStatus;
  const catalogReadRef = useRef<AbortController | null>(null);
  useEffect(() => () => catalogReadRef.current?.abort(), []);
  const runtimeReadRef = useRef<AbortController | null>(null);
  const [refreshError, setRefreshError] = useState<unknown>(null);
  const [runtime, setRuntime] = useState<RuntimeStatus | null>(null);
  const [runtimeError, setRuntimeError] = useState<unknown>(null);
  const system = useQuery(queries.system(client));
  const platform = system.data?.platform;
  const installed = useQuery(queries.installed(client, platform));
  const catalog = useQuery(queries.catalog(client, platform));
  const readRuntime = useCallback(async (signal: AbortSignal) => {
    const previous = latestRuntimeRef.current;
    try {
      const next = await client.getRuntimeStatus(signal);
      if (signal.aborted || latestRuntimeRef.current !== previous) return;
      setRuntime(next);
      setRuntimeError(null);
      acceptRuntimeStatus?.(next, previous);
    } catch (error) {
      if (!signal.aborted) setRuntimeError(error);
    }
  }, [client, acceptRuntimeStatus]);
  useEffect(() => {
    const controller = new AbortController();
    runtimeReadRef.current = controller;
    void readRuntime(controller.signal);
    return () => runtimeReadRef.current?.abort();
  }, [readRuntime]);
  const refreshInstalled = async () => {
    await cache.invalidateQueries({ queryKey: ['installed'], refetchType: 'none' });
    runtimeReadRef.current?.abort();
    const controller = new AbortController();
    runtimeReadRef.current = controller;
    await Promise.all([installed.refetch(), readRuntime(controller.signal)]);
  };
  const refreshCatalog = async (force = true) => {
    setRefreshError(null);
    catalogReadRef.current?.abort();
    const controller = new AbortController();
    catalogReadRef.current = controller;
    try {
      if (force) await client.refreshCatalog(true, controller.signal);
      await catalog.refetch();
    } catch (error) {
      setRefreshError(error);
    }
  };
  return {
    platform,
    artifacts: installed.data ?? [],
    catalog: catalog.data ?? null,
    runtime: telemetry?.runtimeStatus ?? runtime,
    platformLoading: system.isPending,
    installedLoading: system.isPending || installed.isFetching,
    catalogLoading: system.isPending || catalog.isFetching,
    error: system.error ?? installed.error ?? runtimeError,
    catalogError: refreshError ?? catalog.error,
    refreshInstalled,
    refreshCatalog,
  };
}
