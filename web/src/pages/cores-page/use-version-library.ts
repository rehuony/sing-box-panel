import { useCallback, useEffect, useRef, useState } from 'react';

import type { CatalogAssetList, CoreArtifact, RuntimeStatus, SystemStatus } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';

type Platform = NonNullable<SystemStatus['platform']>;

export function useVersionLibrary() {
  const client = useApiClient();
  const telemetry = useOptionalSharedTelemetry();
  const acceptRuntimeStatus = telemetry?.acceptRuntimeStatus;
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

  const readInstalled = useCallback(async (target: Platform, signal: AbortSignal) => {
    const items: CoreArtifact[] = [];
    let next: { created_at: string; id: string } | undefined;
    do {
      const page = await client.listCoreArtifacts(
        {
          architecture: target.arch,
          limit: 200,
          beforeID: next?.id,
          beforeTime: next?.created_at,
        },
        signal,
      );
      items.push(...page.items);
      next = page.next;
    } while (next && !signal.aborted);
    return items.filter(value => value.os === target.os && value.arch === target.arch);
  }, [client]);

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

  const refreshInstalled = useCallback(async () => {
    const target = platformRef.current;
    if (!target) return;
    installedRef.current?.abort();
    const controller = new AbortController();
    installedRef.current = controller;
    setInstalledLoading(true);
    setError(null);
    try {
      const [installed, current] = await Promise.all([
        readInstalled(target, controller.signal),
        client.getRuntimeStatus(controller.signal),
      ]);
      if (controller.signal.aborted) return;
      setArtifacts(installed);
      setRuntime(current);
      acceptRuntimeStatus?.(current);
    } catch (reason) {
      if (!controller.signal.aborted) setError(reason);
    } finally {
      if (!controller.signal.aborted) setInstalledLoading(false);
    }
  }, [acceptRuntimeStatus, client, readInstalled]);

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
        const [system, current] = await Promise.all([
          client.getSystemStatus(controller.signal),
          client.getRuntimeStatus(controller.signal),
        ]);
        if (controller.signal.aborted) return;
        setPlatform(system.platform);
        platformRef.current = system.platform;
        setRuntime(current);
        acceptRuntimeStatus?.(current);
        setPlatformLoading(false);
        if (!system.platform) {
          setInstalledLoading(false);
          setCatalogLoading(false);
          return;
        }
        void refreshInstalled();
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
  }, [acceptRuntimeStatus, client, refreshCatalog, refreshInstalled]);

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
