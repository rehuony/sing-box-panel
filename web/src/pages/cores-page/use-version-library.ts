import { useCallback, useEffect, useRef, useState } from 'react';

import type { CatalogAssetList, CoreArtifact, RuntimeStatus, SystemStatus } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

export function useVersionLibrary() {
  const client = useApiClient();
  const [platform, setPlatform] = useState<SystemStatus['platform']>();
  const [artifacts, setArtifacts] = useState<CoreArtifact[]>([]);
  const [catalog, setCatalog] = useState<CatalogAssetList | null>(null);
  const [runtime, setRuntime] = useState<RuntimeStatus | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [catalogError, setCatalogError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const requestRef = useRef<AbortController | null>(null);
  const load = useCallback(async () => {
    requestRef.current?.abort();
    const controller = new AbortController();
    requestRef.current = controller;
    const { signal } = controller;
    setLoading(true);
    setError(null);
    setCatalogError(null);
    try {
      const [system, current] = await Promise.all([
        client.getSystemStatus(signal),
        client.getRuntimeStatus(signal),
      ]);
      if (signal.aborted) return;
      setPlatform(system.platform);
      setRuntime(current);
      if (!system.platform) {
        setArtifacts([]);
        setCatalog(null);
        return;
      }
      const arch = system.platform.arch;
      const [installed, available] = await Promise.allSettled([
        (async () => {
          const items: CoreArtifact[] = [];
          let next: { created_at: string; id: string } | undefined;
          do {
            const page = await client.listCoreArtifacts(
              { architecture: arch, limit: 200, beforeID: next?.id, beforeTime: next?.created_at },
              signal,
            );
            items.push(...page.items);
            next = page.next;
          } while (next && !signal.aborted);
          return items.filter((value) => value.os === system.platform!.os && value.arch === arch);
        })(),
        client.listCatalogAssets({ architecture: arch }, signal),
      ]);
      if (signal.aborted) return;
      if (installed.status === 'fulfilled') setArtifacts(installed.value);
      else setError(installed.reason);
      if (available.status === 'fulfilled') {
        setCatalog({
          ...available.value,
          assets: available.value.assets.filter(
            (asset) => asset.os === system.platform!.os && asset.arch === arch,
          ),
        });
      } else {
        setCatalog(null);
        setCatalogError(available.reason);
      }
    } catch (reason) {
      if (!signal.aborted) {
        setError(reason);
        setPlatform(undefined);
      }
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  }, [client]);
  useEffect(() => {
    void load();
    return () => requestRef.current?.abort();
  }, [load]);
  return { platform, artifacts, catalog, runtime, loading, error, catalogError, load };
}
