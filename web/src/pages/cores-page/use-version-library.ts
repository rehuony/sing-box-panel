import { useCallback, useEffect, useRef, useState } from 'react';

import type { CatalogAssetList, CoreArtifact, RuntimeStatus, SystemStatus } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { listInstalledCoreArtifacts } from '@/utils/installed-core-artifacts';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';

type Platform = NonNullable<SystemStatus['platform']>;

export function useVersionLibrary() {
  const client = useApiClient();
  const telemetry = useOptionalSharedTelemetry();
  const acceptRuntimeStatus = telemetry?.acceptRuntimeStatus;
  const latestRuntimeRef = useRef(telemetry?.runtimeStatus);
  useEffect(() => {
    latestRuntimeRef.current = telemetry?.runtimeStatus;
  }, [telemetry?.runtimeStatus]);
  const [platform, setPlatform] = useState<Platform>();
  const platformRef = useRef<Platform | undefined>(undefined);
  const [artifacts, setArtifacts] = useState<CoreArtifact[]>([]);
  const [catalog, setCatalog] = useState<CatalogAssetList | null>(null);
  const [runtime, setRuntime] = useState<RuntimeStatus | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [catalogError, setCatalogError] = useState<unknown>(null);
  const [platformLoading, setPlatformLoading] = useState(true);
  const [installedLoading, setInstalledLoading] = useState(true);
  const [catalogLoading, setCatalogLoading] = useState(true);
  const installedRef = useRef<AbortController | null>(null);
  const catalogRef = useRef<AbortController | null>(null);

  const readCatalog = useCallback(async (
    target: Platform,
    signal: AbortSignal,
    force: boolean,
  ) => {
    if (force) await client.refreshCatalog(true, signal);
    try {
      const cached = await client.listCatalogAssets({ architecture: target.arch }, signal);
      return {
        ...cached,
        assets: cached.assets.filter(asset => asset.os === target.os && asset.arch === target.arch),
      };
    } catch (reason) {
      if (force || signal.aborted) throw reason;
      await client.refreshCatalog(false, signal);
      const initialized = await client.listCatalogAssets({ architecture: target.arch }, signal);
      return {
        ...initialized,
        assets: initialized.assets.filter(asset => asset.os === target.os && asset.arch === target.arch),
      };
    }
  }, [client]);

  const readRuntime = useCallback(async (signal: AbortSignal) => {
    const previous = latestRuntimeRef.current;
    const current = await client.getRuntimeStatus(signal);
    // A stream event or completed control action is newer than this pending read.
    if (signal.aborted || latestRuntimeRef.current !== previous) return;
    setRuntime(current);
    acceptRuntimeStatus?.(current, previous);
  }, [acceptRuntimeStatus, client]);

  const refreshInstalled = useCallback(async (refreshRuntime = true) => {
    const target = platformRef.current;
    if (!target) return;
    if (refreshRuntime) client.invalidateReadCache();
    installedRef.current?.abort();
    const controller = new AbortController();
    installedRef.current = controller;
    setInstalledLoading(true);
    setError(null);
    try {
      const [installed] = await Promise.all([
        listInstalledCoreArtifacts(client, target, controller.signal),
        refreshRuntime ? readRuntime(controller.signal) : undefined,
      ]);
      if (controller.signal.aborted) return;
      setArtifacts(installed);
    } catch (reason) {
      if (!controller.signal.aborted) setError(reason);
    } finally {
      if (!controller.signal.aborted) setInstalledLoading(false);
    }
  }, [client, readRuntime]);

  const refreshCatalog = useCallback(async (force = true) => {
    const target = platformRef.current;
    if (!target) return;
    catalogRef.current?.abort();
    const controller = new AbortController();
    catalogRef.current = controller;
    setCatalogLoading(true);
    setCatalogError(null);
    try {
      const next = await readCatalog(target, controller.signal, force);
      if (!controller.signal.aborted) setCatalog(next);
    } catch (reason) {
      if (!controller.signal.aborted) setCatalogError(reason);
    } finally {
      if (!controller.signal.aborted) setCatalogLoading(false);
    }
  }, [readCatalog]);

  useEffect(() => {
    const controller = new AbortController();
    void (async () => {
      try {
        const [system] = await Promise.all([
          client.getSystemStatus(controller.signal),
          readRuntime(controller.signal),
        ]);
        if (controller.signal.aborted) return;
        setPlatform(system.platform);
        platformRef.current = system.platform;
        setPlatformLoading(false);
        if (!system.platform) {
          setInstalledLoading(false);
          setCatalogLoading(false);
          return;
        }
        void refreshInstalled(false);
        void refreshCatalog(false);
      } catch (reason) {
        if (!controller.signal.aborted) {
          setError(reason);
          setPlatformLoading(false);
          setInstalledLoading(false);
          setCatalogLoading(false);
        }
      }
    })();
    return () => {
      controller.abort();
      installedRef.current?.abort();
      catalogRef.current?.abort();
    };
  }, [client, readRuntime, refreshCatalog, refreshInstalled]);

  return {
    platform,
    artifacts,
    catalog,
    runtime: telemetry?.runtimeStatus ?? runtime,
    platformLoading,
    installedLoading,
    catalogLoading,
    error,
    catalogError,
    refreshInstalled,
    refreshCatalog,
  };
}
