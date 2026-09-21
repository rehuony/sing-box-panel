import { useCallback, useEffect, useState } from 'react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { useApiClient } from '@/api/api-client-context';
import { hasReviewedSchemaVersion } from '@/schemas/generated';
import { resolveBundledReviewedSchema, resolveReviewedSchema } from '@/schemas/resolve-reviewed-schema';

type SchemaState
  = | { status: 'unavailable'; resolution: null; error: null }
    | { status: 'loading'; resolution: null; error: null }
    | { status: 'error'; resolution: null; error: unknown }
    | { status: 'ready'; resolution: ReviewedSchemaResolution; error: null; bundled: boolean };

export function useConfigurationSchema(exactVersion: string) {
  const client = useApiClient();
  const [state, setState] = useState<SchemaState>(() => ({
    status: hasReviewedSchemaVersion(exactVersion) ? 'loading' : 'unavailable',
    resolution: null,
    error: null,
  }));

  const load = useCallback(async (signal?: AbortSignal) => {
    if (exactVersion === '' || !hasReviewedSchemaVersion(exactVersion)) {
      setState({
        status: 'unavailable', resolution: null, error: null,
      });
      return;
    }
    setState({ status: 'loading', resolution: null, error: null });
    try {
      const page = await client.listCoreArtifacts({
        exactVersion, limit: 50,
      }, signal);
      const artifacts = page.items.filter((artifact) => artifact.exact_version === exactVersion);
      const selected = artifacts[0];
      const resolution = selected === undefined
        ? await resolveBundledReviewedSchema(exactVersion)
        : await resolveReviewedSchema(await client.getConfigurationSchema(selected.id, signal), exactVersion);
      if (signal?.aborted) return;
      setState({
        status: 'ready', resolution, error: null, bundled: selected === undefined,
      });
    } catch (error) {
      if (signal?.aborted || (error instanceof DOMException && error.name === 'AbortError')) return;
      setState({ status: 'error', resolution: null, error });
    }
  }, [client, exactVersion]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  return state;
}
