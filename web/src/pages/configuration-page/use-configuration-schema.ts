import { useCallback, useEffect, useState } from 'react';

import type { CoreArtifact } from '@/api/api-client';
import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { useApiClient } from '@/api/api-client-context';
import { hasReviewedSchemaVersion } from '@/schemas/generated';
import { resolveReviewedSchema } from '@/schemas/resolve-reviewed-schema';

type SchemaState
  = | { status: 'unavailable'; artifactID: string | null; resolution: null; error: null }
    | { status: 'loading'; artifactID: string; resolution: null; error: null }
    | { status: 'error'; artifactID: string; resolution: null; error: unknown }
    | { status: 'ready'; artifactID: string; resolution: ReviewedSchemaResolution; error: null };

export function useConfigurationSchema(artifact: CoreArtifact | null): SchemaState {
  const client = useApiClient();
  const exactVersion = artifact?.exact_version ?? '';
  const [state, setState] = useState<SchemaState>(() =>
    artifact !== null && hasReviewedSchemaVersion(exactVersion)
      ? { status: 'loading', artifactID: artifact.id, resolution: null, error: null }
      : { status: 'unavailable', artifactID: artifact?.id ?? null, resolution: null, error: null });

  const load = useCallback(async (signal?: AbortSignal) => {
    if (artifact === null || !hasReviewedSchemaVersion(exactVersion)) {
      setState({
        status: 'unavailable', artifactID: artifact?.id ?? null, resolution: null, error: null,
      });
      return;
    }
    setState({ status: 'loading', artifactID: artifact.id, resolution: null, error: null });
    try {
      const resolution = await resolveReviewedSchema(
        client.getConfigurationSchema(artifact.id, signal),
        exactVersion,
      );
      if (signal?.aborted) return;
      setState({
        status: 'ready', artifactID: artifact.id, resolution, error: null,
      });
    } catch (error) {
      if (signal?.aborted || (error instanceof DOMException && error.name === 'AbortError')) return;
      setState({ status: 'error', artifactID: artifact.id, resolution: null, error });
    }
  }, [artifact, client, exactVersion]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  if (state.artifactID === artifact?.id) return state;
  const pending: SchemaState = artifact !== null && hasReviewedSchemaVersion(exactVersion)
    ? { status: 'loading', artifactID: artifact.id, resolution: null, error: null }
    : { status: 'unavailable', artifactID: artifact?.id ?? null, resolution: null, error: null };
  return pending;
}
