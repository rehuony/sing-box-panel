import type { HttpApiContext } from './shared';
import type {
  ApiClient,
  CatalogAssetFilter,
  CatalogAssetList,
  CatalogRefresh,
  ConfigurationCompile,
  ConfigurationSchemaContract,
  CoreArtifact,
  CoreArtifactFilter,
  CoreArtifactPage,
  CoreImportUpload,
  RuntimeStatus,
} from '../api-client';

import { ApiRequestError } from '../api-client';

/** How long a catalog refresh may run before the view says it timed out; GitHub normally answers within seconds. */
const CATALOG_REFRESH_TIMEOUT = 60_000;

export function createCoreHttpApi(context: HttpApiContext) {
  const {
    baseUrl, buildQuery, fetcher, nodeUrl, request, writeHeaders, writeJSONHeaders,
  } = context;

  function runtimeAction(operation: 'start' | 'stop' | 'restart', signal?: AbortSignal) {
    return request<RuntimeStatus>(fetcher, `${nodeUrl()}/core/${operation}`, {
      method: 'POST', headers: writeHeaders(), signal,
    });
  }

  return {
    listCatalogAssets(filter: CatalogAssetFilter = {}, signal) {
      const query = buildQuery({
        architecture: filter.architecture,
        exact_version: filter.exactVersion,
        variant: filter.variant,
      });
      return request<CatalogAssetList>(fetcher, `${baseUrl}/core/catalog/assets${query}`, {
        method: 'GET', signal,
      });
    },
    async refreshCatalog(force = false, signal) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), CATALOG_REFRESH_TIMEOUT);
      const forward = () => controller.abort(signal?.reason);
      if (signal?.aborted) forward();
      signal?.addEventListener('abort', forward, { once: true });
      try {
        return await request<CatalogRefresh>(fetcher, `${baseUrl}/core/catalog/refresh`, {
          method: 'POST', body: JSON.stringify({ force }), headers: writeJSONHeaders(), signal: controller.signal,
        });
      } catch (error) {
        if (controller.signal.aborted && !signal?.aborted) {
          throw new ApiRequestError('The catalog refresh timed out.', { status: 504, code: 'deadline' });
        }
        throw error;
      } finally {
        clearTimeout(timer);
        signal?.removeEventListener('abort', forward);
      }
    },
    listCoreArtifacts(filter: CoreArtifactFilter = {}, signal) {
      const query = buildQuery({
        architecture: filter.architecture,
        before_id: filter.beforeID,
        before_time: filter.beforeTime,
        exact_version: filter.exactVersion,
        limit: filter.limit ?? 50,
        source_kind: filter.sourceKind,
        variant: filter.variant,
      });
      return request<CoreArtifactPage>(fetcher, `${nodeUrl()}/core/artifacts${query}`, {
        method: 'GET', signal,
      });
    },
    installCore(assetID, signal) {
      return request<CoreArtifact>(fetcher, `${nodeUrl()}/core/install`, {
        method: 'POST', body: JSON.stringify({ asset_id: assetID }), headers: writeJSONHeaders(), signal,
      });
    },
    importCoreArchive(input: CoreImportUpload, signal) {
      const form = new FormData();
      form.set('archive', input.archive);
      form.set('source_description', input.sourceDescription);
      form.set('exact_version', input.exactVersion);
      form.set('architecture', input.architecture);
      form.set('variant', input.variant);
      return request<CoreArtifact>(fetcher, `${nodeUrl()}/core/import`, {
        method: 'POST', body: form, headers: writeHeaders(), signal,
      });
    },
    removeCoreArtifact(artifactID, signal) {
      return request<void>(fetcher, `${nodeUrl()}/core/artifacts/${encodeURIComponent(artifactID)}`, {
        method: 'DELETE', headers: writeHeaders(), signal,
      });
    },
    getConfigurationSchema(artifactID, signal) {
      return request<ConfigurationSchemaContract>(fetcher, `${nodeUrl()}/core/artifacts/${encodeURIComponent(artifactID)}/configuration-schema`, {
        method: 'GET', signal,
      });
    },
    compileConfiguration(input, signal) {
      return request<ConfigurationCompile>(fetcher, `${nodeUrl()}/config/compile`, {
        method: 'POST',
        body: JSON.stringify({
          core_artifact_id: input.coreArtifactID,
        }),
        headers: writeJSONHeaders(), signal,
      });
    },
    getRuntimeStatus(signal) {
      return request<RuntimeStatus>(fetcher, `${nodeUrl()}/core/status`, { method: 'GET', signal });
    },
    startRuntime(signal) {
      return runtimeAction('start', signal);
    },
    enableCore(artifactID, signal) {
      return request<RuntimeStatus>(fetcher, `${nodeUrl()}/core/artifacts/${encodeURIComponent(artifactID)}/enable`, {
        method: 'POST', headers: writeHeaders(), signal,
      });
    },
    disableCore(artifactID, signal) {
      return request<RuntimeStatus>(fetcher, `${nodeUrl()}/core/artifacts/${encodeURIComponent(artifactID)}/disable`, {
        method: 'POST', headers: writeHeaders(), signal,
      });
    },
    stopRuntime(signal) {
      return runtimeAction('stop', signal);
    },
    restartRuntime(signal) {
      return runtimeAction('restart', signal);
    },
    rollbackRuntime(activationBundleID, signal) {
      return request<RuntimeStatus>(fetcher, `${nodeUrl()}/core/rollback`, {
        method: 'POST',
        body: JSON.stringify({ activation_bundle_id: activationBundleID }),
        headers: writeJSONHeaders(),
        signal,
      });
    },
  } satisfies Partial<ApiClient>;
}
