import type { RJSFSchema } from '@rjsf/utils';

import { useQuery } from '@tanstack/react-query';

import type { CoreArtifact } from '@/api/api-client';

import i18n from '@/i18n';
import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';

/** The configuration schema the panel serves for the selected core. */
export interface ConfigurationSchemaResolution {
  schema: RJSFSchema;
}

type SchemaState
  = | { status: 'unavailable'; artifactID: string | null; resolution: null; error: null }
    | { status: 'loading'; artifactID: string; resolution: null; error: null }
    | { status: 'error'; artifactID: string; resolution: null; error: unknown }
    | { status: 'ready'; artifactID: string; resolution: ConfigurationSchemaResolution; error: null };

function schemaUnavailable(error: unknown): boolean {
  return error instanceof ApiRequestError && error.status === 409 && error.code === 'configuration_schema_unavailable';
}

export function useConfigurationSchema(artifact: CoreArtifact | null): SchemaState & { retry: () => void } {
  const client = useApiClient();
  const version = artifact?.exact_version ?? '';
  const query = useQuery({
    queryKey: ['schema', artifact?.id, version], enabled: artifact !== null, staleTime: 300_000,
    queryFn: async ({ signal }): Promise<ConfigurationSchemaResolution> => {
      const contract = await client.getConfigurationSchema(artifact!.id, signal);
      const schema: unknown = contract.schema;
      if (contract.exact_version !== version || schema === null || typeof schema !== 'object' || Array.isArray(schema)) {
        throw new Error(i18n.t('configuration.schema.error.contract'));
      }
      return { schema: schema as RJSFSchema };
    },
  });
  const retry = () => void query.refetch();
  if (!artifact || schemaUnavailable(query.error)) return { status: 'unavailable', artifactID: artifact?.id ?? null, resolution: null, error: null, retry };
  if (query.data) return { status: 'ready', artifactID: artifact.id, resolution: query.data, error: null, retry };
  if (query.error) return { status: 'error', artifactID: artifact.id, resolution: null, error: query.error, retry };
  return { status: 'loading', artifactID: artifact.id, resolution: null, error: null, retry };
}
