import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';

import type { CoreArtifact } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { installedCoreVersions } from '@/utils/installed-core-artifacts';

export function useInstalledConfigurationVersions() {
  const client = useApiClient();
  const system = useQuery(queries.system(client));
  const installed = useQuery(queries.installed(client, system.data?.platform));
  const runtime = useQuery(queries.runtime(client));
  const state = {
    status: system.isPending || (system.data?.platform && installed.isPending) || runtime.isPending
      ? 'loading' as const
      : system.error || installed.error || runtime.error ? 'error' as const : 'ready' as const,
    artifacts: installed.data ?? [],
    runtime: runtime.data ?? null,
    error: system.error ?? installed.error ?? runtime.error,
  };
  const versions = useMemo(() => installedCoreVersions(state.artifacts), [state.artifacts]);
  const artifactsByVersion = useMemo(() => {
    const result = new Map<string, CoreArtifact>();
    state.artifacts.forEach((artifact) => {
      if (!result.has(artifact.exact_version)) result.set(artifact.exact_version, artifact);
    });
    return result;
  }, [state.artifacts]);

  return { ...state, artifactsByVersion, versions };
}
