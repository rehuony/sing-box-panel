import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';

import type { CoreArtifact } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { installedCoreVersions } from '@/utils/installed-core-artifacts';
import { nodePlatform, useNodeSelection } from '@/stores/node-selection.store';

export function useInstalledConfigurationVersions() {
  const client = useApiClient();
  const platform = nodePlatform(useNodeSelection().selected);
  const installed = useQuery(queries.installed(client, platform));
  const runtime = useQuery(queries.runtime(client));
  const state = {
    status: (platform && installed.isPending) || runtime.isPending
      ? 'loading' as const
      : installed.error || runtime.error ? 'error' as const : 'ready' as const,
    artifacts: installed.data ?? [],
    runtime: runtime.data ?? null,
    error: installed.error ?? runtime.error,
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
