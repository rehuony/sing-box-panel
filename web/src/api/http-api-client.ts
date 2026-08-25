import {
  isLosslessNumber,
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import {
  ApiRequestError,
  type ActivationQueued,
  type ApiClient,
  type CanonicalRevisionPage,
  type CanonicalSave,
  type CanonicalSnapshot,
  type CanonicalChange,
  type CapabilityStatus,
  type CatalogAssetFilter,
  type CatalogAssetList,
  type CoreArtifactFilter,
  type CoreArtifactPage,
  type CreatedSubscriptionToken,
  type DashboardContext,
  type Entity,
  type EntityCollection,
  type EntityList,
  type LogEntry,
  type LogClearFilter,
  type LogFilter,
  type LogPage,
  type ManualArtifact,
  type ManualArtifactList,
  type ManualReplacePreview,
  type ManualReattachPreview,
  type ManualReattachSave,
  type ManualSave,
  type MetricsSnapshot,
  type Session,
  type StructuredRender,
  type StartupArtifactPage,
  type SubscriptionChannel,
  type SubscriptionSource,
  type SubscriptionToken,
  type SubscriptionTokenRotation,
  type Task,
  type TaskFilter,
  type TaskPage,
  type TrafficPeriod,
  type TrafficPeriodFilter,
  type TrafficPeriodPage,
} from './api-client';

interface ProblemDetails {
  code?: string;
  detail?: string;
  fields?: Record<string, string>;
  title?: string;
}

interface SessionPayload {
  displayName: string;
  csrfToken?: string;
}

export interface HttpApiClientOptions {
  baseUrl?: string;
  fetcher?: typeof fetch;
}

async function readProblem(response: Response): Promise<ApiRequestError> {
  let problem: ProblemDetails = {};

  try {
    problem = (await response.json()) as ProblemDetails;
  } catch {
    // A proxy or an older server may return an empty/non-JSON error response.
  }

  return new ApiRequestError(
    problem.detail ?? problem.title ?? `Request failed with status ${response.status}`,
    {
      status: response.status,
      code: problem.code ?? 'request_failed',
      fields: problem.fields,
    },
  );
}

async function request<T>(
  fetcher: typeof fetch,
  url: string,
  init: RequestInit,
): Promise<T> {
  const response = await fetcher(url, {
    ...init,
    credentials: 'same-origin',
    headers: {
      Accept: 'application/json',
      ...init.headers,
    },
  });

  if (!response.ok) {
    throw await readProblem(response);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

function losslessEntities(
  result: EntityList,
  collection: EntityCollection,
): EntityList {
  let parsed: unknown;
  try {
    parsed = parseLosslessJSON(result.revision.document_json);
  } catch (error) {
    throw new Error('The canonical revision contains invalid document_json.', { cause: error });
  }
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object' || isLosslessNumber(parsed)) {
    throw new Error('The canonical revision document_json is not one JSON object.');
  }
  const entities = (parsed as Record<string, unknown>)[collection];
  if (!Array.isArray(entities)) {
    throw new Error(`The canonical revision document_json does not contain a ${collection} array.`);
  }
  for (const [index, entity] of entities.entries()) {
    if (entity === null || Array.isArray(entity) || typeof entity !== 'object' || isLosslessNumber(entity)) {
      throw new Error(`The canonical revision ${collection}[${index}] is not one JSON object.`);
    }
  }
  return { ...result, entities: entities as Entity[] };
}

function losslessRequestJSON(value: unknown): string {
  const encoded = stringifyLosslessJSON(value);
  if (encoded === undefined) {
    throw new Error('The request value cannot be encoded as JSON.');
  }
  return encoded;
}

export function createHttpApiClient(
  options: HttpApiClientOptions = {},
): ApiClient {
  const baseUrl = (options.baseUrl ?? '/api/v1').replace(/\/$/, '');
  const fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis);
	let csrfToken = '';

  function writeHeaders(headers: HeadersInit = {}): HeadersInit {
    return csrfToken === ''
      ? headers
      : { ...headers, 'X-CSRF-Token': csrfToken };
  }

  function writeJSONHeaders(headers: HeadersInit = {}): HeadersInit {
    return writeHeaders({ 'Content-Type': 'application/json', ...headers });
  }

  function quoteETag(value: string): string {
    return `"${value.replaceAll('"', '')}"`;
  }

  function buildQuery(values: Record<string, string | number | boolean | undefined>): string {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(values)) {
      if (value !== undefined && value !== '') {
        query.set(key, String(value));
      }
    }
    const encoded = query.toString();
    return encoded === '' ? '' : `?${encoded}`;
  }

	function acceptSession(payload: SessionPayload): Session {
		csrfToken = payload.csrfToken ?? '';
		return { displayName: payload.displayName };
	}

  return {
    async getSession(signal) {
      try {
		const payload = await request<SessionPayload>(fetcher, `${baseUrl}/auth/session`, {
          method: 'GET',
          signal,
        });
		return acceptSession(payload);
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 401) {
		  csrfToken = '';
          return null;
        }
        throw error;
      }
    },
	async login(token, signal) {
	  const payload = await request<SessionPayload>(fetcher, `${baseUrl}/auth/session`, {
        method: 'POST',
        body: JSON.stringify({ token }),
        headers: {
          'Content-Type': 'application/json',
        },
        signal,
      });
	  return acceptSession(payload);
    },
	async logout(signal) {
	  await request<void>(fetcher, `${baseUrl}/auth/session`, {
        method: 'DELETE',
		headers: writeHeaders(),
        signal,
      });
	  csrfToken = '';
    },
    getDashboardContext(signal) {
      return request<DashboardContext>(fetcher, `${baseUrl}/dashboard/context`, {
        method: 'GET',
        signal,
      });
    },
    async listEntities(collection, signal) {
      const result = await request<EntityList>(fetcher, `${baseUrl}/${collection}`, {
        method: 'GET',
        signal,
      });
      return losslessEntities(result, collection);
    },
    saveEntity(collection, entity, saveOptions, signal) {
      const existingPath = saveOptions.existingID
        ? `/${encodeURIComponent(saveOptions.existingID)}`
        : '';
      return request<CanonicalSave>(
        fetcher,
        `${baseUrl}/${collection}${existingPath}`,
        {
          method: saveOptions.existingID ? 'PUT' : 'POST',
          body: losslessRequestJSON(entity),
          headers: writeJSONHeaders({
            'If-Match': quoteETag(saveOptions.baseRevision),
          }),
          signal,
        },
      );
    },
    getCanonical(signal) {
      return request<CanonicalSnapshot>(fetcher, `${baseUrl}/config/canonical`, {
        method: 'GET',
        signal,
      });
    },
    replaceCanonical(documentJSON: string, baseRevision, signal) {
      return request<CanonicalSave>(fetcher, `${baseUrl}/config/canonical`, {
        method: 'PUT',
        body: documentJSON,
        headers: writeJSONHeaders({
          'If-Match': quoteETag(baseRevision),
        }),
        signal,
      });
    },
    patchCanonical(changes: CanonicalChange[], baseRevision, signal) {
      return request<CanonicalSave>(fetcher, `${baseUrl}/config/canonical`, {
        method: 'PATCH',
        body: JSON.stringify({ changes }),
        headers: writeJSONHeaders({
          'If-Match': quoteETag(baseRevision),
        }),
        signal,
      });
    },
    listRevisions(signal) {
      return request<CanonicalRevisionPage>(
        fetcher,
        `${baseUrl}/config/revisions?limit=8`,
        { method: 'GET', signal },
      );
    },
    listTasks(filter: TaskFilter = {}, signal) {
      const query = buildQuery({
        kind: filter.kind,
        lane: filter.lane,
        limit: filter.limit ?? 50,
        status: filter.status,
      });
      return request<TaskPage>(fetcher, `${baseUrl}/tasks${query}`, {
        method: 'GET',
        signal,
      });
    },
    cancelTask(taskID, signal) {
      return request<Task>(
        fetcher,
        `${baseUrl}/tasks/${encodeURIComponent(taskID)}/cancel`,
        { method: 'POST', headers: writeHeaders(), signal },
      );
    },
    listCatalogAssets(filter: CatalogAssetFilter = {}, signal) {
      const query = buildQuery({
        architecture: filter.architecture,
        exact_version: filter.exactVersion,
        installable: filter.installable,
        variant: filter.variant,
      });
      return request<CatalogAssetList>(
        fetcher,
        `${baseUrl}/core/catalog/assets${query}`,
        { method: 'GET', signal },
      );
    },
    refreshCatalog(signal) {
      return request<Task>(fetcher, `${baseUrl}/core/catalog/refresh`, {
        method: 'POST',
        headers: writeHeaders(),
        signal,
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
        method: 'GET',
        signal,
      });
    },
    installCore(assetID, signal) {
      return request<Task>(fetcher, `${baseUrl}/core/install`, {
        method: 'POST',
        body: JSON.stringify({ asset_id: assetID }),
        headers: writeJSONHeaders(),
        signal,
      });
    },
    removeCoreArtifact(artifactID, signal) {
      return request<void>(
        fetcher,
        `${baseUrl}/core/artifacts/${encodeURIComponent(artifactID)}`,
        { method: 'DELETE', headers: writeHeaders(), signal },
      );
    },
    quarantineCoreArtifact(artifactID, signal) {
      return request<CoreArtifactPage['items'][number]>(
        fetcher,
        `${baseUrl}/core/artifacts/${encodeURIComponent(artifactID)}/quarantine`,
        { method: 'POST', headers: writeHeaders(), signal },
      );
    },
    revokeCoreArtifact(artifactID, signal) {
      return request<CoreArtifactPage['items'][number]>(
        fetcher,
        `${baseUrl}/core/artifacts/${encodeURIComponent(artifactID)}/revoke`,
        { method: 'POST', headers: writeHeaders(), signal },
      );
    },
    getCoreCapability(exactVersion, signal) {
      const query = buildQuery({ core_version: exactVersion });
      return request<CapabilityStatus>(
        fetcher,
        `${baseUrl}/core/capability${query}`,
        { method: 'GET', signal },
      );
    },
    renderStructured(input, signal) {
      return request<StructuredRender>(fetcher, `${baseUrl}/config/render`, {
        method: 'POST',
        body: JSON.stringify({
          core_version: input.coreVersion,
          core_artifact_id: input.coreArtifactID,
          allow_compatible: input.allowCompatible,
        }),
        headers: writeJSONHeaders(),
        signal,
      });
    },
    listStartupArtifacts(filter, signal) {
      const query = buildQuery({
        canonical_revision_id: filter.canonicalRevisionID,
        core_version: filter.coreVersion,
        core_artifact_id: filter.coreArtifactID,
        kind: filter.kind,
        state: filter.state,
        before_time: filter.beforeTime,
        before_id: filter.beforeID,
        limit: filter.limit ?? 100,
      });
      return request<StartupArtifactPage>(fetcher, `${baseUrl}/config/artifacts${query}`, {
        method: 'GET',
        signal,
      });
    },
    listManualArtifacts(filter, signal) {
      const query = buildQuery({
        core_version: filter.coreVersion,
        core_artifact_id: filter.coreArtifactID,
        limit: filter.limit ?? 50,
      });
      return request<ManualArtifactList>(fetcher, `${baseUrl}/config/manual${query}`, {
        method: 'GET',
        signal,
      });
    },
    getManualArtifact(artifactID, signal) {
      return request<ManualArtifact>(
        fetcher,
        `${baseUrl}/config/manual/${encodeURIComponent(artifactID)}`,
        { method: 'GET', signal },
      );
    },
    replaceManualArtifact(input, signal) {
      const query = buildQuery({
        core_version: input.coreVersion,
        core_artifact_id: input.coreArtifactID,
        allow_compatible: input.allowCompatible,
      });
      return request<ManualSave>(fetcher, `${baseUrl}/config/manual${query}`, {
        method: 'PUT',
        body: input.raw,
        headers: writeHeaders({
          'Content-Type': 'application/jsonc',
          'If-Match': quoteETag(input.baseRevision),
        }),
        signal,
      });
    },
    previewManualReplacement(input, signal) {
      const query = buildQuery({
        core_version: input.coreVersion,
        core_artifact_id: input.coreArtifactID,
        allow_compatible: input.allowCompatible,
      });
      return request<ManualReplacePreview>(fetcher, `${baseUrl}/config/manual/preview${query}`, {
        method: 'POST',
        body: input.raw,
        headers: writeHeaders({
          'Content-Type': 'application/jsonc',
          'If-Match': quoteETag(input.baseRevision),
        }),
        signal,
      });
    },
    discardManualArtifact(artifactID, signal) {
      return request<ManualArtifact>(
        fetcher,
        `${baseUrl}/config/manual/${encodeURIComponent(artifactID)}`,
        { method: 'DELETE', headers: writeHeaders(), signal },
      );
    },
    activateStartupArtifact(artifactID, monitoringTier, signal) {
      return request<ActivationQueued>(fetcher, `${baseUrl}/config/apply`, {
        method: 'POST',
        body: JSON.stringify({
          startup_artifact_id: artifactID,
          monitoring_tier: monitoringTier,
        }),
        headers: writeJSONHeaders(),
        signal,
      });
    },
    previewManualReattach(artifactID, signal) {
      return request<ManualReattachPreview>(
        fetcher,
        `${baseUrl}/config/manual/${encodeURIComponent(artifactID)}/reattach/preview`,
        { method: 'GET', signal },
      );
    },
    applyManualReattach(artifactID, input, signal) {
      return request<ManualReattachSave>(
        fetcher,
        `${baseUrl}/config/manual/${encodeURIComponent(artifactID)}/reattach`,
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: writeJSONHeaders(),
          signal,
        },
      );
    },
    listSubscriptionChannels(signal) {
      return request<SubscriptionChannel[]>(
        fetcher,
        `${baseUrl}/subscription/channels`,
        { method: 'GET', signal },
      );
    },
    createSubscriptionChannel(input, signal) {
      return request<SubscriptionChannel>(
        fetcher,
        `${baseUrl}/subscription/channels`,
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: writeJSONHeaders(),
          signal,
        },
      );
    },
    updateSubscriptionChannel(channelID, input, updatedAt, signal) {
      return request<SubscriptionChannel>(
        fetcher,
        `${baseUrl}/subscription/channels/${encodeURIComponent(channelID)}`,
        {
          method: 'PUT',
          body: JSON.stringify(input),
          headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    deleteSubscriptionChannel(channelID, updatedAt, signal) {
      return request<void>(
        fetcher,
        `${baseUrl}/subscription/channels/${encodeURIComponent(channelID)}`,
        {
          method: 'DELETE',
          headers: writeHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    listSubscriptionSources(signal) {
      return request<SubscriptionSource[]>(
        fetcher,
        `${baseUrl}/subscription/sources`,
        { method: 'GET', signal },
      );
    },
    createSubscriptionSource(input, signal) {
      return request<SubscriptionSource>(fetcher, `${baseUrl}/subscription/sources`, {
        method: 'POST',
        body: JSON.stringify(input),
        headers: writeJSONHeaders(),
        signal,
      });
    },
    updateSubscriptionSource(sourceID, input, updatedAt, signal) {
      return request<SubscriptionSource>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}`,
        {
          method: 'PUT',
          body: JSON.stringify(input),
          headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    updateSubscriptionSourceSnapshot(sourceID, snapshot, updatedAt, signal) {
      return request<SubscriptionSource>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}/snapshot`,
        {
          method: 'PUT',
          body: JSON.stringify({ latest_snapshot: snapshot }),
          headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    deleteSubscriptionSource(sourceID, updatedAt, signal) {
      return request<void>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}`,
        {
          method: 'DELETE',
          headers: writeHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    listSubscriptionTokens(signal) {
      return request<SubscriptionToken[]>(fetcher, `${baseUrl}/subscription/tokens`, {
        method: 'GET',
        signal,
      });
    },
    createSubscriptionToken(input, signal) {
      return request<CreatedSubscriptionToken>(
        fetcher,
        `${baseUrl}/subscription/tokens`,
        {
          method: 'POST',
          body: JSON.stringify({
            channel_id: input.channelID,
            expires_at: input.expiresAt,
          }),
          headers: writeJSONHeaders(),
          signal,
        },
      );
    },
    rotateSubscriptionToken(tokenID, expiresAt, signal) {
      return request<SubscriptionTokenRotation>(
        fetcher,
        `${baseUrl}/subscription/tokens/${encodeURIComponent(tokenID)}/rotate`,
        {
          method: 'POST',
          body: JSON.stringify({ expires_at: expiresAt }),
          headers: writeJSONHeaders(),
          signal,
        },
      );
    },
    revokeSubscriptionToken(tokenID, signal) {
      return request<SubscriptionToken>(
        fetcher,
        `${baseUrl}/subscription/tokens/${encodeURIComponent(tokenID)}/revoke`,
        { method: 'POST', headers: writeHeaders(), signal },
      );
    },
    listLogs(filter: LogFilter = {}, signal) {
      const query = buildQuery({
        source: filter.source,
        level: filter.level,
        since: filter.since,
        limit: filter.limit ?? 50,
        after_time: filter.afterTime,
        after_id: filter.afterID,
      });
      return request<LogPage>(fetcher, `${baseUrl}/logs${query}`, {
        method: 'GET',
        signal,
      });
    },
    getLog(entryID, signal) {
      return request<LogEntry>(fetcher, `${baseUrl}/logs/${encodeURIComponent(entryID)}`, {
        method: 'GET',
        signal,
      });
    },
    getMetrics(signal) {
      return request<MetricsSnapshot>(fetcher, `${baseUrl}/metrics`, {
        method: 'GET',
        signal,
      });
    },
    getTrafficStatus(signal) {
      return request<MetricsSnapshot>(fetcher, `${baseUrl}/traffic/status`, {
        method: 'GET',
        signal,
      });
    },
    listTrafficPeriods(filter: TrafficPeriodFilter = {}, signal) {
      const query = buildQuery({
        activation_bundle_id: filter.activationBundleID,
        from: filter.from,
        to: filter.to,
        limit: filter.limit ?? 50,
      });
      return request<TrafficPeriodPage>(fetcher, `${baseUrl}/traffic/periods${query}`, {
        method: 'GET',
        signal,
      });
    },
    getTrafficPeriod(periodID, signal) {
      return request<TrafficPeriod>(
        fetcher,
        `${baseUrl}/traffic/periods/${encodeURIComponent(periodID)}`,
        { method: 'GET', signal },
      );
    },
    clearLogs(filter: LogClearFilter = {}, signal) {
      const query = buildQuery({ before: filter.before, source: filter.source });
      return request<{ deleted: number }>(fetcher, `${baseUrl}/logs${query}`, {
        method: 'DELETE',
        headers: writeHeaders(),
        signal,
      });
    },
    deleteLog(entryID, signal) {
      return request<{ id: string; deleted: true }>(
        fetcher,
        `${baseUrl}/logs/${encodeURIComponent(entryID)}`,
        { method: 'DELETE', headers: writeHeaders(), signal },
      );
    },
  };
}
