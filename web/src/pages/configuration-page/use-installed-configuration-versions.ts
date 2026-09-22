import { useEffect, useMemo, useState } from 'react';

import type { CoreArtifact, RuntimeStatus } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { installedCoreVersions, listInstalledCoreArtifacts } from '@/utils/installed-core-artifacts';

type InstalledVersionState
  = | { status: 'loading'; artifacts: CoreArtifact[]; runtime: null; error: null }
    | { status: 'error'; artifacts: CoreArtifact[]; runtime: null; error: unknown }
    | { status: 'ready'; artifacts: CoreArtifact[]; runtime: RuntimeStatus; error: null };

export function useInstalledConfigurationVersions() {
  const client = useApiClient();
  const [state, setState] = useState<InstalledVersionState>({
    status: 'loading', artifacts: [], runtime: null, error: null,
  });

  useEffect(() => {
    const controller = new AbortController();
    void (async () => {
      try {
        const [system, runtime] = await Promise.all([
          client.getSystemStatus(controller.signal),
          client.getRuntimeStatus(controller.signal),
        ]);
        const artifacts = system.platform === undefined
          ? []
          : await listInstalledCoreArtifacts(client, system.platform, controller.signal);
        if (!controller.signal.aborted) setState({ status: 'ready', artifacts, runtime, error: null });
      } catch (error) {
        if (!controller.signal.aborted) setState({ status: 'error', artifacts: [], runtime: null, error });
      }
    })();
    return () => controller.abort();
  }, [client]);

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
