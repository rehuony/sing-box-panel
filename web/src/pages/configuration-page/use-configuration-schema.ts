import { useQuery } from '@tanstack/react-query';

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
  const version = artifact?.exact_version ?? '';
  const enabled = artifact !== null && hasReviewedSchemaVersion(version);
  const query = useQuery({
    queryKey: ['schema', artifact?.id, version], enabled, staleTime: 300_000,
    queryFn: ({ signal }) => resolveReviewedSchema(client.getConfigurationSchema(artifact!.id, signal), version),
  });
  if (!artifact || !enabled) return { status: 'unavailable', artifactID: artifact?.id ?? null, resolution: null, error: null };
  if (query.data) return { status: 'ready', artifactID: artifact.id, resolution: query.data, error: null };
  if (query.error) return { status: 'error', artifactID: artifact.id, resolution: null, error: query.error };
  return { status: 'loading', artifactID: artifact.id, resolution: null, error: null };
}
