import type { HttpApiContext } from './shared';
import type { ActivationQueued, ApiClient, CatalogAssetFilter, CatalogAssetList, ConfigurationAdapterSupport, ConfigurationCompile, ConfigurationPreview, CoreArtifact, CoreArtifactFilter, CoreArtifactPage, CoreImportUpload, RuntimeStatus, StartupArtifactPage, Task } from '../api-client';

async function fileSHA256(file: File): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', await file.arrayBuffer());
  return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, '0')).join('');
}

export function createCoreHttpApi(context: HttpApiContext) {
  const {
    baseUrl, buildQuery, fetcher, request, writeHeaders, writeJSONHeaders,
  } = context;

  function runtimeAction(operation: 'start' | 'stop' | 'restart' | 'rollback', signal?: AbortSignal) {
    return request<Task>(fetcher, `${baseUrl}/core/${operation}`, {
      method: 'POST', headers: writeHeaders(), signal,
    });
  }

  return {
    listCatalogAssets(filter: CatalogAssetFilter = {}, signal) {
      const query = buildQuery({
        architecture: filter.architecture,
        exact_version: filter.exactVersion,
        installable: filter.installable,
        variant: filter.variant,
      });
      return request<CatalogAssetList>(fetcher, `${baseUrl}/core/catalog/assets${query}`, {
        method: 'GET', signal,
      });
    },
    refreshCatalog(force = false, signal) {
      return request<Task>(fetcher, `${baseUrl}/core/catalog/refresh`, {
        method: 'POST', body: JSON.stringify({ force }), headers: writeJSONHeaders(), signal,
      });
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
        verification_state: filter.verificationState,
      });
      return request<CoreArtifactPage>(fetcher, `${baseUrl}/core/artifacts${query}`, {
        method: 'GET', signal,
      });
    },
    getCoreArtifact(artifactID, signal) {
      return request<CoreArtifact>(fetcher, `${baseUrl}/core/artifacts/${encodeURIComponent(artifactID)}`, {
        method: 'GET', signal,
      });
    },
    installCore(assetID, signal) {
      return request<Task>(fetcher, `${baseUrl}/core/install`, {
        method: 'POST', body: JSON.stringify({ asset_id: assetID }), headers: writeJSONHeaders(), signal,
      });
    },
    async importCoreArchive(input: CoreImportUpload, signal) {
      const form = new FormData();
      form.set('archive', input.archive);
      form.set('source_description', input.sourceDescription);
      form.set('sha256', await fileSHA256(input.archive));
      form.set('exact_version', input.exactVersion);
      form.set('architecture', input.architecture);
      form.set('variant', input.variant);
      return request<Task>(fetcher, `${baseUrl}/core/import`, {
        method: 'POST', body: form, headers: writeHeaders(), signal,
      });
    },
    removeCoreArtifact(artifactID, signal) {
      return request<void>(fetcher, `${baseUrl}/core/artifacts/${encodeURIComponent(artifactID)}`, {
        method: 'DELETE', headers: writeHeaders(), signal,
      });
    },
    quarantineCoreArtifact(artifactID, signal) {
      return request<CoreArtifact>(fetcher, `${baseUrl}/core/artifacts/${encodeURIComponent(artifactID)}/quarantine`, {
        method: 'POST', headers: writeHeaders(), signal,
      });
    },
    revokeCoreArtifact(artifactID, signal) {
      return request<CoreArtifact>(fetcher, `${baseUrl}/core/artifacts/${encodeURIComponent(artifactID)}/revoke`, {
        method: 'POST', headers: writeHeaders(), signal,
      });
    },
    getConfigurationSupport(artifactID, signal) {
      return request<ConfigurationAdapterSupport>(fetcher, `${baseUrl}/core/artifacts/${encodeURIComponent(artifactID)}/configuration-support`, {
        method: 'GET', signal,
      });
    },
    previewConfiguration(input, signal) {
      return request<ConfigurationPreview>(fetcher, `${baseUrl}/config/preview`, {
        method: 'POST',
        body: JSON.stringify({
          core_artifact_id: input.coreArtifactID,
          canonical_revision_id: input.canonicalRevisionID,
        }),
        headers: writeJSONHeaders(), signal,
      });
    },
    compileConfiguration(input, signal) {
      return request<ConfigurationCompile>(fetcher, `${baseUrl}/config/compile`, {
        method: 'POST',
        body: JSON.stringify({
          core_artifact_id: input.coreArtifactID,
          accepted_ignored_digest: input.acceptedIgnoredDigest,
        }),
        headers: writeJSONHeaders(), signal,
      });
    },
    listStartupArtifacts(filter, signal) {
      const query = buildQuery({
        canonical_revision_id: filter.canonicalRevisionID,
        core_version: filter.coreVersion,
        core_artifact_id: filter.coreArtifactID,
        state: filter.state,
        before_time: filter.beforeTime,
        before_id: filter.beforeID,
        limit: filter.limit ?? 100,
      });
      return request<StartupArtifactPage>(fetcher, `${baseUrl}/config/artifacts${query}`, {
        method: 'GET', signal,
      });
    },
    checkStartupArtifact(artifactID, signal) {
      return request<Task>(fetcher, `${baseUrl}/core/check`, {
        method: 'POST', body: JSON.stringify({ startup_artifact_id: artifactID }), headers: writeJSONHeaders(), signal,
      });
    },
    activateStartupArtifact(artifactID, monitoringTier, signal) {
      return request<ActivationQueued>(fetcher, `${baseUrl}/config/apply`, {
        method: 'POST',
        body: JSON.stringify({ startup_artifact_id: artifactID, monitoring_tier: monitoringTier }),
        headers: writeJSONHeaders(), signal,
      });
    },
    getRuntimeStatus(signal) {
      return request<RuntimeStatus>(fetcher, `${baseUrl}/core/status`, { method: 'GET', signal });
    },
    startRuntime(signal) {
      return runtimeAction('start', signal);
    },
    stopRuntime(signal) {
      return runtimeAction('stop', signal);
    },
    restartRuntime(signal) {
      return runtimeAction('restart', signal);
    },
    rollbackRuntime(signal) {
      return runtimeAction('rollback', signal);
    },
  } satisfies Partial<ApiClient>;
}
