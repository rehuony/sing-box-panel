import {
  isLosslessNumber,
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import type {
  ApiClient,
  CanonicalChange,
  CanonicalRevisionDiff,
  CanonicalSnapshot,
  CatalogAsset,
  ConfigurationFile,
  CoreArtifact,
  LogEntry,
  PanelLog,
  PanelSettingsView,
  StartupArtifactSummary,
  SubscriptionChannel,
  SubscriptionSource,
  SubscriptionSourceVersion,
  SubscriptionToken,
  SubscriptionUser,
  Task,
} from '../api-client';

import { ApiRequestError } from '../api-client';
import { createDemoCoreLogs } from './demo-core-logs';
import { DEFAULT_APPEARANCE } from '../../theme/appearance';
import { reviewedSchemaManifest } from '../../schemas/generated';
import {
  createDemoNodeApi,
  demoManualNodes,
  demoSourceNodeDetails,
} from './demo-subscription-nodes';
import {
  createDemoData,
  demoDashboardContext,
  demoMetrics,
  demoMetricsHistory,
  demoSystemStatus,
} from './demo-data';

const taskDurationMilliseconds = 700;

interface PendingTask {
  readyAt: number;
  complete: () => void;
}

interface DemoState extends ReturnType<typeof createDemoData> {
  nextID: number;
  revisions: CanonicalSnapshot[];
  pendingTasks: Map<string, PendingTask>;
  session: { displayName: string } | null;
}

function abortError(): DOMException {
  return new DOMException('The operation was aborted.', 'AbortError');
}

function assertActive(signal?: AbortSignal): void {
  if (signal?.aborted === true) throw signal.reason ?? abortError();
}

async function respond<T>(value: T, signal?: AbortSignal): Promise<T> {
  assertActive(signal);
  await Promise.resolve();
  assertActive(signal);
  return structuredClone(value);
}

function notFound(resource: string, id: string): never {
  throw new ApiRequestError(`${resource} ${id} does not exist in this demo.`, {
    status: 404,
    code: 'not_found',
  });
}

function conflict(resource: string): never {
  throw new ApiRequestError(`${resource} changed after it was loaded.`, {
    status: 412,
    code: 'precondition_failed',
  });
}

function requireItem<T extends { id: string }>(items: T[], id: string, resource: string): T {
  return items.find((item) => item.id === id) ?? notFound(resource, id);
}

function assertCurrent(item: { updated_at: string }, updatedAt: string, resource: string): void {
  if (item.updated_at !== updatedAt) conflict(resource);
}

function subscriptionKeyActive(key: SubscriptionToken): boolean {
  return (
    key.enabled
    && !key.revoked_at
    && (!key.expires_at || Date.parse(key.expires_at) > Date.now())
    && (key.download_limit === undefined || key.body_response_count < key.download_limit)
  );
}

function updatedAt(): string {
  return new Date().toISOString();
}

function nextID(state: DemoState, prefix: string): string {
  state.nextID += 1;
  return `${prefix}_demo_${state.nextID}`;
}

function pageByCreatedAt<T extends { id: string; created_at: string }>(
  items: T[],
  filter: { beforeID?: string; beforeTime?: string; limit?: number },
): { items: T[]; next?: { created_at: string; id: string } } {
  const sorted = [...items].sort(
    (left, right) =>
      right.created_at.localeCompare(left.created_at) || right.id.localeCompare(left.id),
  );
  let start = 0;
  if (filter.beforeTime !== undefined && filter.beforeID !== undefined) {
    const beforeTime = filter.beforeTime;
    const cursor = sorted.findIndex(
      (item) => item.created_at === beforeTime && item.id === filter.beforeID,
    );
    start = cursor >= 0 ? cursor + 1 : sorted.findIndex((item) => item.created_at < beforeTime);
    if (start < 0) start = sorted.length;
  }
  const limit = Math.max(1, filter.limit ?? 100);
  const page = sorted.slice(start, start + limit);
  const last = page.at(-1);
  return {
    items: page,
    next:
      start + page.length < sorted.length && last !== undefined
        ? { created_at: last.created_at, id: last.id }
        : undefined,
  };
}

function createState(): DemoState {
  const data = createDemoData();
  const revisions = [data.canonical];
  for (let sequence = data.canonical.sequence - 1; sequence >= 1; sequence -= 1) {
    const createdAt = new Date(
      new Date(data.canonical.created_at).getTime()
        - (data.canonical.sequence - sequence) * 3_600_000,
    ).toISOString();
    const document = structuredClone(data.canonical.document);
    if (sequence < 5 && 'log' in document) {
      document.log = { level: sequence < 3 ? 'warn' : 'info' };
    }
    revisions.push({
      id: `revision_demo_${sequence}`,
      sequence,
      parent_id: sequence > 1 ? `revision_demo_${sequence - 1}` : undefined,
      schema_version: 1,
      document,
      document_json: JSON.stringify(document),
      sha256: sequence.toString(16).repeat(64).slice(0, 64),
      created_at: createdAt,
    });
  }
  return {
    ...data,
    nextID: 100,
    pendingTasks: new Map(),
    revisions,
    session: { displayName: 'Demo administrator' },
  };
}

function settleTasks(state: DemoState): void {
  const now = Date.now();
  for (const [taskID, pending] of state.pendingTasks) {
    const task = requireItem(state.tasks, taskID, 'Task');
    if (now >= pending.readyAt) {
      task.status = task.cancel_requested ? 'canceled' : 'succeeded';
      task.updated_at = updatedAt();
      task.result = task.cancel_requested ? undefined : { accepted: true };
      if (!task.cancel_requested) pending.complete();
      state.pendingTasks.delete(taskID);
    } else if (now >= pending.readyAt - taskDurationMilliseconds / 2) {
      task.status = 'running';
      task.attempt = 1;
      task.updated_at = updatedAt();
    }
  }
}

function queueTask(
  state: DemoState,
  kind: Task['kind'],
  options: {
    complete?: () => void;
    lane?: Task['lane'];
    payload?: unknown;
    startupArtifactID?: string;
    activationBundleID?: string;
  } = {},
): Task {
  const now = updatedAt();
  const task: Task = {
    id: nextID(state, 'task'),
    lane: options.lane ?? (kind.startsWith('runtime-') ? 'runtime' : 'maintenance'),
    kind,
    status: 'queued',
    generation: kind.startsWith('runtime-') ? state.runtime.target_generation + 1 : 0,
    canonical_revision_id: state.canonical.id,
    startup_artifact_id: options.startupArtifactID,
    activation_bundle_id: options.activationBundleID,
    payload: options.payload ?? {},
    cancel_requested: false,
    attempt: 0,
    created_at: now,
    updated_at: now,
  };
  state.tasks.unshift(task);
  state.pendingTasks.set(task.id, {
    readyAt: Date.now() + taskDurationMilliseconds,
    complete: options.complete ?? (() => undefined),
  });
  return task;
}

function addRuntimeTransition(
  state: DemoState,
  task: Task,
  runtimeState: 'running' | 'stopped',
  reason: string,
): void {
  state.runtimeHistory.items.unshift({
    id: Math.max(0, ...state.runtimeHistory.items.map((item) => item.id)) + 1,
    state: runtimeState,
    reason,
    activation_bundle_id: state.runtime.applied_bundle_id,
    generation: task.generation,
    task_id: task.id,
    pid: state.runtime.running?.pid,
    process_started_at: state.runtime.running?.started_at,
    occurred_at: updatedAt(),
  });
}

function stoppedRuntime(state: DemoState, task: Task): void {
  state.runtime = {
    ...state.runtime,
    desired_running: false,
    target_generation: task.generation,
    observation_state: 'stopped',
    running: undefined,
    loaded_canonical_revision_id: undefined,
  };
  addRuntimeTransition(state, task, 'stopped', 'stop_succeeded');
}

function runningRuntime(state: DemoState, task: Task, bundleID?: string): void {
  const core
    = state.cores.find((item) => item.id === state.runtime.enabled_core?.core_artifact_id)
      ?? state.cores.find((item) => item.id === state.runtime.running?.core_artifact_id)
      ?? state.cores[0];
  if (core === undefined) return;
  const processToken = `demo-process-${state.nextID}-${Date.now()}`;
  state.runtime = {
    ...state.runtime,
    enabled_core: { core_artifact_id: core.id, exact_core_version: core.exact_version },
    desired_running: true,
    desired_bundle_id: bundleID ?? state.runtime.desired_bundle_id,
    applied_bundle_id: bundleID ?? state.runtime.applied_bundle_id,
    target_generation: task.generation,
    observation_state: 'running',
    loaded_canonical_revision_id: task.canonical_revision_id,
    running: {
      pid: 4_000 + state.nextID,
      process_start_token: processToken,
      exact_core_version: core.exact_version,
      core_artifact_id: core.id,
      archive_sha256: core.archive_sha256,
      binary_sha256: core.binary_sha256,
      activation_bundle_id: bundleID ?? state.runtime.applied_bundle_id ?? 'bundle_demo_current',
      started_at: updatedAt(),
    },
  };
  addRuntimeTransition(state, task, 'running', 'start_succeeded');
}

function canonicalReference(state: DemoState, reference: string): CanonicalSnapshot {
  const numeric = Number(reference);
  return (
    state.revisions.find(
      (item) => item.id === reference || (Number.isInteger(numeric) && item.sequence === numeric),
    ) ?? notFound('Revision', reference)
  );
}

function saveCanonical(state: DemoState, documentJSON: string, baseRevision: string) {
  if (state.canonical.id !== baseRevision) conflict('Canonical configuration');
  let document: Record<string, unknown>;
  let canonicalJSON: string;
  try {
    const decoded = parseLosslessJSON(documentJSON);
    if (
      decoded === null
      || Array.isArray(decoded)
      || typeof decoded !== 'object'
      || isLosslessNumber(decoded)
    ) {
      throw new Error('Expected a JSON object.');
    }
    const encoded = stringifyLosslessJSON(decoded);
    if (encoded === undefined) throw new Error('Expected an encodable JSON object.');
    canonicalJSON = encoded;
    // `document_json` is the lossless source of truth. The ordinary object
    // mirrors the browser-decoded HTTP response used by non-editor views.
    document = JSON.parse(canonicalJSON) as Record<string, unknown>;
  } catch {
    throw new ApiRequestError('Canonical configuration must be a JSON object.', {
      status: 422,
      code: 'invalid_configuration',
    });
  }
  if (canonicalJSON === state.canonical.document_json) {
    return { revision: state.canonical, no_change: true } as const;
  }
  const sequence = state.canonical.sequence + 1;
  const revision: CanonicalSnapshot = {
    id: `revision_demo_${sequence}`,
    sequence,
    parent_id: state.canonical.id,
    schema_version: 1,
    document,
    document_json: canonicalJSON,
    sha256: sequence.toString(16).repeat(64).slice(0, 64),
    created_at: updatedAt(),
  };
  state.canonical = revision;
  state.revisions.unshift(revision);
  const task = queueTask(state, 'canonical-saved', { payload: { revision_id: revision.id } });
  return { revision, no_change: false, task_id: task.id } as const;
}

function applyChanges(
  document: Record<string, unknown>,
  changes: CanonicalChange[],
): Record<string, unknown> {
  const next = structuredClone(document);
  for (const change of changes) {
    const parts = change.path
      .split('/')
      .slice(1)
      .map((part) => part.replaceAll('~1', '/').replaceAll('~0', '~'));
    if (parts.length === 0) continue;
    let parent: Record<string, unknown> = next;
    for (const part of parts.slice(0, -1)) {
      const child = parent[part];
      if (child === null || Array.isArray(child) || typeof child !== 'object') parent[part] = {};
      parent = parent[part] as Record<string, unknown>;
    }
    const key = parts.at(-1);
    if (key === undefined) continue;
    if (change.op === 'unset') {
      delete parent[key];
    } else {
      parent[key]
        = change.value_json === undefined ? null : (JSON.parse(change.value_json) as unknown);
    }
  }
  return next;
}

function matchesLog(entry: LogEntry, filter: Parameters<ApiClient['listLogs']>[0] = {}): boolean {
  return (
    (filter.source === undefined || entry.source === filter.source)
    && (filter.level === undefined || entry.level === filter.level)
    && (filter.code === undefined || entry.code.includes(filter.code))
    && (filter.since === undefined || entry.time >= filter.since)
    && (filter.until === undefined || entry.time <= filter.until)
  );
}

/**
 * Creates an isolated, in-memory implementation of the complete browser API.
 * Each invocation starts from the same representative UI state.
 */
export function createDemoApiClient(): ApiClient {
  const state = createState();
  const tokenSecrets = new Map(state.tokens.map((token) => [token.id, `sbp_demo_${token.id}_secret`]));
  const nodeApi = createDemoNodeApi([...demoManualNodes(), ...demoSourceNodeDetails(state)]);
  let panelSettings: PanelSettingsView = {
    revision: 0,
    github_token_configured: false,
    identity_key_configured: false,
    restart_required: false,
    preferences: {
      listen_host: '127.0.0.1',
      listen_port: 3000,
      external_origin: '',
      public_node_host: '',
      identity_name: '',
      traffic_quota_gib: 500,
      language: 'zh-CN',
      appearance: { ...DEFAULT_APPEARANCE },
    },
  };
  let configurationFile: ConfigurationFile = {
    revision: 1,
    content: state.canonical.document_json,
    syntax_valid: true,
    canonical_revision_id: state.canonical.id,
  };
  const sessionListeners = new Set<() => void>();
  state.runtime.loaded_canonical_revision_id = state.startupArtifacts[0]?.canonical_revision_id;
  function requireParsedFile() {
    if (!configurationFile.syntax_valid) {
      throw new ApiRequestError('Correct the saved JSON before validating or starting.', {
        status: 422,
        code: 'configuration_file_unparsed',
      });
    }
  }
  function saveLegacyConfiguration(content: string, base: string) {
    requireParsedFile();
    const result = saveCanonical(state, content, base);
    if (!result.no_change) {
      configurationFile = {
        revision: configurationFile.revision + 1,
        content: result.revision.document_json,
        canonical_revision_id: result.revision.id,
        syntax_valid: true,
        updated_at: updatedAt(),
      };
    }
    return result;
  }

  const client: ApiClient = {
    newInboundDefaults: (type, signal) => respond({ type }, signal),
    getConfigurationFile: (signal) => respond(configurationFile, signal),
    async saveConfigurationFile(input, signal) {
      assertActive(signal);
      if (input.revision !== configurationFile.revision) conflict('Configuration file');
      if (input.content === configurationFile.content) return respond(configurationFile, signal);
      let canonicalID: string | undefined;
      try {
        canonicalID = saveCanonical(state, input.content, state.canonical.id).revision.id;
      } catch (error) {
        if (!(error instanceof ApiRequestError) || error.status !== 422) throw error;
      }
      configurationFile = {
        revision: configurationFile.revision + 1,
        content: input.content,
        canonical_revision_id: canonicalID,
        syntax_valid: canonicalID !== undefined,
        updated_at: updatedAt(),
      };
      return respond(configurationFile, signal);
    },
    getPanelSettings: (signal) => respond(panelSettings, signal),
    async savePanelSettings(input, signal) {
      assertActive(signal);
      if (input.revision !== panelSettings.revision) conflict('Panel settings');
      panelSettings = {
        revision: panelSettings.revision + 1,
        preferences: structuredClone(input.preferences),
        github_token_configured: input.clear_github_token
          ? false
          : Boolean(input.github_token) || panelSettings.github_token_configured,
        identity_key_configured:
          Boolean(input.identity_key) || panelSettings.identity_key_configured,
        restart_required:
          input.preferences.listen_host !== '127.0.0.1'
          || input.preferences.listen_port !== 3000
          || input.preferences.external_origin !== '',
      };
      return respond(panelSettings, signal);
    },
    subscribeSessionInvalidated(listener) {
      sessionListeners.add(listener);
      return () => sessionListeners.delete(listener);
    },
    getSession: (signal) => respond(state.session, signal),
    async login(_token, signal) {
      assertActive(signal);
      state.session = { displayName: 'Demo administrator' };
      return respond(state.session, signal);
    },
    async logout(signal) {
      assertActive(signal);
      state.session = null;
      for (const listener of sessionListeners) listener();
      await respond(undefined, signal);
    },
    getSystemStatus: (signal) => respond(demoSystemStatus(state), signal),
    getDashboardContext: (signal) => respond(demoDashboardContext(state), signal),
    getCanonical: (signal) => respond(state.canonical, signal),
    async replaceCanonical(documentJSON, baseRevision, signal) {
      assertActive(signal);
      return respond(saveLegacyConfiguration(documentJSON, baseRevision), signal);
    },
    async patchCanonical(changes, baseRevision, signal) {
      assertActive(signal);
      const next = applyChanges(state.canonical.document, changes);
      return respond(saveLegacyConfiguration(JSON.stringify(next), baseRevision), signal);
    },
    listRevisions(filter = {}, signal) {
      const eligible
        = filter.beforeSequence === undefined
          ? state.revisions
          : state.revisions.filter((revision) => revision.sequence < filter.beforeSequence!);
      const limit = Math.max(1, filter.limit ?? 8);
      const items = eligible.slice(0, limit);
      const last = items.at(-1);
      return respond(
        {
          items,
          next_before_sequence: items.length < eligible.length ? last?.sequence : undefined,
        },
        signal,
      );
    },
    getRevision: (reference, signal) => respond(canonicalReference(state, reference), signal),
    diffRevisions(from, to, signal) {
      const fromRevision = canonicalReference(state, from);
      const toRevision = canonicalReference(state, to);
      const fromDocument = fromRevision.document;
      const toDocument = toRevision.document;
      const keys = new Set([...Object.keys(fromDocument), ...Object.keys(toDocument)]);
      const changes: CanonicalRevisionDiff['changes'] = [...keys]
        .filter((key) => JSON.stringify(fromDocument[key]) !== JSON.stringify(toDocument[key]))
        .map((key) => ({
          path: `/${key}`,
          from: { present: key in fromDocument, value: fromDocument[key] },
          to: { present: key in toDocument, value: toDocument[key] },
        }));
      return respond({ from: fromRevision, to: toRevision, changes }, signal);
    },
    async restoreRevision(reference, baseRevision, signal) {
      assertActive(signal);
      const revision = canonicalReference(state, reference);
      return respond(saveLegacyConfiguration(revision.document_json, baseRevision), signal);
    },
    listCatalogAssets(filter = {}, signal) {
      const assets = state.catalog.assets.filter(
        (asset) =>
          (filter.exactVersion === undefined || asset.version === filter.exactVersion)
          && (filter.architecture === undefined || asset.arch === filter.architecture)
          && (filter.variant === undefined || asset.variant === filter.variant)
          && (filter.installable === undefined
            || filter.installable === (asset.has_api_digest || asset.has_catalog_digest)),
      );
      return respond({ ...state.catalog, assets }, signal);
    },
    refreshCatalog(force = false, signal) {
      assertActive(signal);
      const task = queueTask(state, 'catalog-refresh', {
        payload: { force },
        complete: () => {
          state.catalog.refreshed_at = updatedAt();
        },
      });
      return respond(task, signal);
    },
    listCoreArtifacts(filter = {}, signal) {
      const filtered = state.cores.filter(
        (artifact) =>
          (filter.exactVersion === undefined || artifact.exact_version === filter.exactVersion)
          && (filter.architecture === undefined || artifact.arch === filter.architecture)
          && (filter.variant === undefined || artifact.variant === filter.variant)
          && (filter.sourceKind === undefined || artifact.source_kind === filter.sourceKind),
      );
      return respond(pageByCreatedAt(filtered, filter), signal);
    },
    getCoreArtifact: (artifactID, signal) =>
      respond(requireItem(state.cores, artifactID, 'Core artifact'), signal),
    installCore(assetID, signal) {
      assertActive(signal);
      const asset
        = state.catalog.assets.find((item) => item.asset_id === assetID)
          ?? notFound('Catalog asset', String(assetID));
      const task = queueTask(state, 'core-install', {
        payload: { asset_id: assetID },
        complete: () => {
          if (state.cores.some((item) => item.asset_id === assetID)) return;
          state.cores.unshift(coreFromCatalog(state, asset));
        },
      });
      return respond(task, signal);
    },
    importCoreArchive(input, signal) {
      assertActive(signal);
      const task = queueTask(state, 'core-import', {
        payload: { name: input.archive.name, exact_version: input.exactVersion },
        complete: () => {
          const id = nextID(state, 'core');
          state.cores.unshift({
            id,
            exact_version: input.exactVersion,
            os: 'linux',
            arch: input.architecture,
            variant: input.variant,
            source_kind: 'user_verified',
            user_source: input.sourceDescription,
            archive_sha256: '8'.repeat(64),
            binary_sha256: '9'.repeat(64),
            binary_path: `/var/lib/sing-box-panel/artifacts/${id}/sing-box`,
            reported_version: input.exactVersion,
            feature_fingerprint: { source: 'demo-import' },
            created_at: updatedAt(),
          });
        },
      });
      return respond(task, signal);
    },
    async removeCoreArtifact(artifactID, signal) {
      assertActive(signal);
      requireItem(state.cores, artifactID, 'Core artifact');
      state.cores = state.cores.filter((item) => item.id !== artifactID);
      await respond(undefined, signal);
    },
    getConfigurationSupport(artifactID, signal) {
      const artifact = requireItem(state.cores, artifactID, 'Core artifact');
      const structured = reviewedSchemaManifest[artifact.exact_version] !== undefined;
      return respond(
        {
          structured,
          exact_version: artifact.exact_version,
          reason: structured
            ? undefined
            : 'Native configuration Schema is available for sing-box 1.14 and newer.',
        },
        signal,
      );
    },
    async getConfigurationSchema(artifactID, signal) {
      assertActive(signal);
      const artifact = requireItem(state.cores, artifactID, 'Core artifact');
      const reviewed = await reviewedSchemaManifest[artifact.exact_version]?.load();
      assertActive(signal);
      if (reviewed === undefined) notFound('Configuration Schema', artifact.exact_version);
      return respond(
        {
          exact_version: artifact.exact_version,
          schema_sha256: reviewed.schemaSHA256,
          schema: reviewed.schema,
        },
        signal,
      );
    },
    previewConfiguration(input, signal) {
      const core = requireItem(state.cores, input.coreArtifactID, 'Core artifact');
      const revision
        = input.canonicalRevisionID === undefined
          ? state.canonical
          : canonicalReference(state, input.canonicalRevisionID);
      const structured = reviewedSchemaManifest[core.exact_version] !== undefined;
      return respond(
        {
          canonical_revision: revision,
          core_artifact: core,
          support: {
            structured,
            exact_version: core.exact_version,
            reason: structured ? undefined : 'This exact version uses the lossless JSON editor.',
          },
          config: revision.document,
        },
        signal,
      );
    },
    compileConfiguration(input, signal) {
      assertActive(signal);
      requireParsedFile();
      const core = requireItem(state.cores, input.coreArtifactID, 'Core artifact');
      const artifact: StartupArtifactSummary = {
        id: nextID(state, 'startup'),
        canonical_revision_id: state.canonical.id,
        exact_core_version: core.exact_version,
        core_artifact_id: core.id,
        config_sha256: state.canonical.sha256,
        state: 'pending' as const,
        created_at: updatedAt(),
      };
      state.startupArtifacts.unshift(artifact);
      const task = queueTask(state, 'startup-check', {
        startupArtifactID: artifact.id,
        complete: () => {
          artifact.state = 'ready';
          (artifact as typeof artifact & { checked_at?: string }).checked_at = updatedAt();
        },
      });
      return respond(
        {
          support: {
            structured: reviewedSchemaManifest[core.exact_version] !== undefined,
            exact_version: core.exact_version,
          },
          artifact,
          task,
        },
        signal,
      );
    },
    listStartupArtifacts(filter, signal) {
      const filtered = state.startupArtifacts.filter(
        (artifact) =>
          (filter.canonicalRevisionID === undefined
            || artifact.canonical_revision_id === filter.canonicalRevisionID)
          && (filter.coreVersion === undefined
            || artifact.exact_core_version === filter.coreVersion)
          && (filter.coreArtifactID === undefined
            || artifact.core_artifact_id === filter.coreArtifactID)
          && (filter.state === undefined || artifact.state === filter.state),
      );
      return respond(pageByCreatedAt(filtered, filter), signal);
    },
    checkStartupArtifact(artifactID, signal) {
      assertActive(signal);
      const artifact = requireItem(state.startupArtifacts, artifactID, 'Startup artifact');
      artifact.state = 'pending';
      const task = queueTask(state, 'startup-check', {
        startupArtifactID: artifactID,
        complete: () => {
          artifact.state = 'ready';
          artifact.checked_at = updatedAt();
        },
      });
      return respond(task, signal);
    },
    activateStartupArtifact(artifactID, monitoringTier, signal) {
      assertActive(signal);
      const artifact = requireItem(state.startupArtifacts, artifactID, 'Startup artifact');
      const bundleID = nextID(state, 'bundle');
      const task = queueTask(state, 'runtime-apply', {
        startupArtifactID: artifactID,
        activationBundleID: bundleID,
        payload: { monitoring_tier: monitoringTier },
        complete: () => runningRuntime(state, task, bundleID),
      });
      return respond(
        {
          activation: {
            startup_artifact_id: artifact.id,
            canonical_revision_id: artifact.canonical_revision_id,
            exact_core_version: artifact.exact_core_version,
            core_artifact_id: artifact.core_artifact_id,
            config_sha256: artifact.config_sha256,
            activation_bundle_id: bundleID,
            activation_sha256: '6'.repeat(64),
            monitoring_tier: monitoringTier,
          },
          task,
        },
        signal,
      );
    },
    getRuntimeStatus(signal) {
      settleTasks(state);
      return respond(state.runtime, signal);
    },
    getRuntimeHistory(filter = {}, signal) {
      settleTasks(state);
      let items = state.runtimeHistory.items.filter(
        (item) =>
          (filter.state === undefined || item.state === filter.state)
          && (filter.reason === undefined || item.reason === filter.reason)
          && (filter.activationBundleID === undefined
            || item.activation_bundle_id === filter.activationBundleID)
          && (filter.from === undefined || item.occurred_at >= filter.from)
          && (filter.to === undefined || item.occurred_at <= filter.to),
      );
      if (filter.beforeTime !== undefined && filter.beforeID !== undefined) {
        items = items.filter(
          (item) =>
            item.occurred_at < filter.beforeTime!
            || (item.occurred_at === filter.beforeTime && item.id < filter.beforeID!),
        );
      }
      items = items.slice(0, Math.max(1, filter.limit ?? 100));
      return respond({ ...state.runtimeHistory, items, next: undefined }, signal);
    },
    startRuntime(signal) {
      assertActive(signal);
      settleTasks(state);
      if (!state.runtime.enabled_core) throw new Error('No core version is enabled.');
      requireParsedFile();
      const task = queueTask(state, 'runtime-start', {
        complete: () => {
          if (state.runtime.observation_state !== 'running') runningRuntime(state, task);
        },
      });
      return respond(task, signal);
    },
    enableCore(artifactID, signal) {
      assertActive(signal);
      settleTasks(state);
      requireParsedFile();
      const artifact = requireItem(state.cores, artifactID, 'Core artifact');
      const platform = demoSystemStatus(state).platform;
      if (
        artifact.os !== platform?.os
        || artifact.arch !== platform?.arch
      ) {
        throw new Error('The core must match the demo platform.');
      }
      const task = queueTask(state, 'runtime-restart', {
        complete: () => {
          const wasRunning = state.runtime.observation_state === 'running';
          if (wasRunning) stoppedRuntime(state, task);
          state.runtime.rollback_bundle_id = state.runtime.applied_bundle_id;
          state.runtime.applied_bundle_id = `bundle_${task.id}`;
          state.runtime.desired_bundle_id = state.runtime.applied_bundle_id;
          state.runtime.enabled_core = { core_artifact_id: artifact.id, exact_core_version: artifact.exact_version };
          if (wasRunning) runningRuntime(state, task);
          else state.runtime.target_generation = task.generation;
        },
      });
      return respond(task, signal);
    },
    disableCore(artifactID, signal) {
      assertActive(signal);
      settleTasks(state);
      requireItem(state.cores, artifactID, 'Core artifact');
      if (state.runtime.enabled_core?.core_artifact_id !== artifactID) {
        throw new Error('The requested core is no longer enabled.');
      }
      const task = queueTask(state, 'runtime-stop', {
        complete: () => {
          stoppedRuntime(state, task);
          state.runtime.enabled_core = undefined;
          state.runtime.applied_bundle_id = undefined;
          state.runtime.desired_bundle_id = undefined;
          state.runtime.rollback_bundle_id = undefined;
        },
      });
      return respond(task, signal);
    },
    stopRuntime(signal) {
      assertActive(signal);
      const task = queueTask(state, 'runtime-stop', {
        complete: () => stoppedRuntime(state, task),
      });
      return respond(task, signal);
    },
    restartRuntime(signal) {
      assertActive(signal);
      settleTasks(state);
      if (!state.runtime.enabled_core) throw new Error('No core version is enabled.');
      requireParsedFile();
      const task = queueTask(state, 'runtime-restart', {
        complete: () => runningRuntime(state, task),
      });
      return respond(task, signal);
    },
    rollbackRuntime(activationBundleID, signal) {
      assertActive(signal);
      const task = queueTask(state, 'runtime-rollback', {
        activationBundleID,
        complete: () => runningRuntime(state, task, activationBundleID),
      });
      return respond(task, signal);
    },
    listTasks(filter = {}, signal) {
      settleTasks(state);
      const filtered = state.tasks.filter(
        (task) =>
          (filter.kind === undefined || task.kind === filter.kind)
          && (filter.lane === undefined || task.lane === filter.lane)
          && (filter.status === undefined || task.status === filter.status),
      );
      return respond(pageByCreatedAt(filtered, filter), signal);
    },
    getTask(taskID, signal) {
      settleTasks(state);
      return respond(requireItem(state.tasks, taskID, 'Task'), signal);
    },
    cancelTask(taskID, signal) {
      const task = requireItem(state.tasks, taskID, 'Task');
      if (task.status === 'queued' || task.status === 'running') task.cancel_requested = true;
      settleTasks(state);
      return respond(task, signal);
    },
    listSubscriptionChannels(filter = {}, signal) {
      return respond(pageByCreatedAt(state.channels, filter), signal);
    },
    getSubscriptionChannel: (channelID, signal) =>
      respond(requireItem(state.channels, channelID, 'Subscription channel'), signal),
    createSubscriptionChannel(input, signal) {
      const now = updatedAt();
      const channel: SubscriptionChannel = {
        id: nextID(state, 'channel'),
        ...input,
        created_at: now,
        updated_at: now,
      };
      state.channels.unshift(channel);
      return respond(channel, signal);
    },
    updateSubscriptionChannel(channelID, input, currentUpdatedAt, signal) {
      const channel = requireItem(state.channels, channelID, 'Subscription channel');
      assertCurrent(channel, currentUpdatedAt, 'Subscription channel');
      Object.assign(channel, input, { updated_at: updatedAt() });
      return respond(channel, signal);
    },
    async deleteSubscriptionChannel(channelID, currentUpdatedAt, signal) {
      assertActive(signal);
      const channel = requireItem(state.channels, channelID, 'Subscription channel');
      assertCurrent(channel, currentUpdatedAt, 'Subscription channel');
      state.channels = state.channels.filter((item) => item.id !== channelID);
      await respond(undefined, signal);
    },
    async previewSubscriptionChannel(channelID, userID, signal, draft) {
      const channel = requireItem(state.channels, channelID, 'Subscription channel');
      if (draft?.config.policy || channel.config.policy) {
        throw new Error(
          'Native channel validation and rendering require a connected panel server. Demo data cannot validate templates.',
        );
      }
      if (userID) requireItem(state.users, userID, 'Subscription user');
      const nodes = (await nodeApi.getSubscriptionNodeCatalog()).nodes.filter(
        (node) => !node.hidden && node.available,
      );
      const content
        = channel.format === 'sing-box'
          ? JSON.stringify(
              { outbounds: nodes.map((node) => ({ type: node.type, tag: node.tag })) },
              null,
              2,
            )
          : nodes.map((node) => `- name: ${node.tag}\n  type: ${node.type}`).join('\n');
      return respond(
        {
          user_id: userID,
          applied_bundle_id: state.runtime.applied_bundle_id ?? '',
          channel,
          startup_artifact_id: state.startupArtifacts[0]?.id ?? '',
          canonical_revision_id: state.canonical.id,
          exact_core_version: state.runtime.running?.exact_core_version ?? '1.14.0',
          artifact_state: state.startupArtifacts[0]?.state ?? 'ready',
          result: {
            format: channel.format,
            media_type: channel.format === 'sing-box' ? 'application/json' : 'text/yaml',
            content,
            node_count: nodes.length,
            diagnostics: [],
          },
        },
        signal,
      );
    },
    listSubscriptionUsers(filter = {}, signal) {
      return respond(pageByCreatedAt(state.users, filter), signal);
    },
    getSubscriptionUser: (userID, signal) =>
      respond(requireItem(state.users, userID, 'Subscription user'), signal),
    createSubscriptionUser(input, signal) {
      const now = updatedAt();
      const user: SubscriptionUser = {
        id: nextID(state, 'user'),
        ...input,
        created_at: now,
        updated_at: now,
      };
      state.users.unshift(user);
      state.grants.set(user.id, []);
      return respond(user, signal);
    },
    updateSubscriptionUser(userID, input, currentUpdatedAt, signal) {
      const user = requireItem(state.users, userID, 'Subscription user');
      assertCurrent(user, currentUpdatedAt, 'Subscription user');
      Object.assign(user, input, { updated_at: updatedAt() });
      return respond(user, signal);
    },
    async deleteSubscriptionUser(userID, currentUpdatedAt, signal) {
      assertActive(signal);
      const user = requireItem(state.users, userID, 'Subscription user');
      assertCurrent(user, currentUpdatedAt, 'Subscription user');
      state.users = state.users.filter((item) => item.id !== userID);
      state.tokens = state.tokens.filter((item) => item.user_id !== userID);
      state.grants.delete(userID);
      await respond(undefined, signal);
    },
    getSubscriptionUserGrants(userID, signal) {
      const user = requireItem(state.users, userID, 'Subscription user');
      return respond({ user, grants: state.grants.get(userID) ?? [] }, signal);
    },
    async replaceSubscriptionUserGrants(userID, grants, currentUpdatedAt, signal) {
      const user = requireItem(state.users, userID, 'Subscription user');
      assertCurrent(user, currentUpdatedAt, 'Subscription user');
      const nodeKeys = new Set(
        (await nodeApi.getSubscriptionNodeCatalog()).nodes.map((node) => node.key),
      );
      if (grants.some((key) => !nodeKeys.has(key))) {
        throw new ApiRequestError('One or more selected subscription nodes no longer exist.', {
          status: 422,
          code: 'invalid_subscription_grant',
        });
      }
      user.updated_at = updatedAt();
      state.grants.set(userID, [...new Set(grants)]);
      return respond({ user, grants: state.grants.get(userID) ?? [] }, signal);
    },
    ...nodeApi,
    listSubscriptionSources(filter = {}, signal) {
      const summaries = state.sources.map(({ config: _config, ...source }) => ({
        ...source,
        has_version: source.current_version_id !== undefined,
      }));
      return respond(pageByCreatedAt(summaries, filter), signal);
    },
    getSubscriptionSource: (sourceID, signal) =>
      respond(requireItem(state.sources, sourceID, 'Subscription source'), signal),
    createSubscriptionSource(input, signal) {
      const now = updatedAt();
      const source: SubscriptionSource = {
        id: nextID(state, 'source'),
        ...input,
        created_at: now,
        updated_at: now,
      };
      state.sources.unshift(source);
      state.sourceVersions.set(source.id, []);
      return respond(source, signal);
    },
    updateSubscriptionSource(sourceID, input, currentUpdatedAt, signal) {
      const source = requireItem(state.sources, sourceID, 'Subscription source');
      assertCurrent(source, currentUpdatedAt, 'Subscription source');
      Object.assign(source, input, { updated_at: updatedAt() });
      return respond(source, signal);
    },
    async deleteSubscriptionSource(sourceID, currentUpdatedAt, signal) {
      assertActive(signal);
      const source = requireItem(state.sources, sourceID, 'Subscription source');
      assertCurrent(source, currentUpdatedAt, 'Subscription source');
      state.sources = state.sources.filter((item) => item.id !== sourceID);
      state.sourceVersions.delete(sourceID);
      await respond(undefined, signal);
    },
    refreshSubscriptionSource(sourceID, signal) {
      assertActive(signal);
      const source = requireItem(state.sources, sourceID, 'Subscription source');
      const task = queueTask(state, 'subscription-source-refresh', {
        payload: { source_id: sourceID },
        complete: () => {
          source.updated_at = updatedAt();
        },
      });
      return respond(task, signal);
    },
    listSubscriptionSourceVersions(sourceID, filter = {}, signal) {
      requireItem(state.sources, sourceID, 'Subscription source');
      return respond(pageByCreatedAt(state.sourceVersions.get(sourceID) ?? [], filter), signal);
    },
    getSubscriptionSourceVersion(sourceID, versionID, signal) {
      requireItem(state.sources, sourceID, 'Subscription source');
      return respond(
        requireItem(state.sourceVersions.get(sourceID) ?? [], versionID, 'Source version'),
        signal,
      );
    },
    createSubscriptionSourceVersion(sourceID, format, rawBody, currentUpdatedAt, signal) {
      const source = requireItem(state.sources, sourceID, 'Subscription source');
      assertCurrent(source, currentUpdatedAt, 'Subscription source');
      const now = updatedAt();
      const version: SubscriptionSourceVersion = {
        id: nextID(state, 'source_version'),
        source_id: sourceID,
        format: format === 'auto' ? 'uri-list' : format,
        raw_body: rawBody,
        normalized_nodes:
          rawBody.trim() === '' ? [] : [{ type: 'direct', tag: 'Imported demo node' }],
        diagnostics: [],
        sha256: '5'.repeat(64),
        fetched_at: now,
        created_at: now,
      };
      const versions = state.sourceVersions.get(sourceID) ?? [];
      versions.unshift(version);
      state.sourceVersions.set(sourceID, versions);
      source.current_version_id = version.id;
      source.updated_at = now;
      return respond({ source, version }, signal);
    },
    restoreSubscriptionSourceVersion(sourceID, versionID, currentUpdatedAt, signal) {
      const source = requireItem(state.sources, sourceID, 'Subscription source');
      assertCurrent(source, currentUpdatedAt, 'Subscription source');
      requireItem(state.sourceVersions.get(sourceID) ?? [], versionID, 'Source version');
      source.current_version_id = versionID;
      source.updated_at = updatedAt();
      return respond(source, signal);
    },
    listSubscriptionTokens(filter = {}, signal) {
      const keys = state.tokens.map((key) => ({ ...key, active: subscriptionKeyActive(key) }));
      return respond(pageByCreatedAt(keys, filter), signal);
    },
    getSubscriptionTokenSecret(tokenID, signal) {
      requireItem(state.tokens, tokenID, 'Subscription token');
      return respond({ token: tokenSecrets.get(tokenID)! }, signal);
    },
    getSubscriptionToken(tokenID, signal) {
      const key = requireItem(state.tokens, tokenID, 'Subscription token');
      return respond({ ...key, active: subscriptionKeyActive(key) }, signal);
    },
    createSubscriptionToken(input, signal) {
      const invalidLimit
        = input.downloadLimit !== undefined
          && (!Number.isSafeInteger(input.downloadLimit)
            || input.downloadLimit < 1
            || input.downloadLimit > 1_000_000_000);
      const invalidExpiry
        = input.expiresAt
          && (!Number.isFinite(Date.parse(input.expiresAt))
            || Date.parse(input.expiresAt) <= Date.now());
      if (!input.label.trim() || invalidLimit || invalidExpiry) {
        throw new ApiRequestError('Invalid subscription key.', {
          status: 422,
          code: 'subscription_invalid',
        });
      }
      if (input.userID) requireItem(state.users, input.userID, 'Subscription user');
      const token: SubscriptionToken = {
        id: nextID(state, 'token'),
        user_id: input.userID,
        label: input.label,
        enabled: true,
        expires_at: input.expiresAt,
        download_limit: input.downloadLimit,
        successful_request_count: 0,
        body_response_count: 0,
        bytes_served: 0,
        created_at: updatedAt(),
        active: true,
      };
      state.tokens.unshift(token);
      tokenSecrets.set(token.id, `sbp_demo_${state.nextID}_one_time_secret`);
      return respond(
        { metadata: token, token: `sbp_demo_${state.nextID}_one_time_secret` },
        signal,
      );
    },
    rotateSubscriptionToken(tokenID, expiresAt, signal) {
      const revoked = requireItem(state.tokens, tokenID, 'Subscription token');
      if (revoked.revoked_at) {
        throw new ApiRequestError('Subscription key is revoked.', {
          status: 409,
          code: 'subscription_token_inactive',
        });
      }
      revoked.revoked_at = updatedAt();
      revoked.active = false;
      const created: SubscriptionToken = {
        ...revoked,
        id: nextID(state, 'token'),
        expires_at: expiresAt ?? revoked.expires_at,
        revoked_at: undefined,
        created_at: updatedAt(),
        active: false,
      };
      created.active = subscriptionKeyActive(created);
      state.tokens.unshift(created);
      tokenSecrets.set(created.id, `sbp_demo_${state.nextID}_rotated_secret`);
      return respond(
        { revoked, created, token: `sbp_demo_${state.nextID}_rotated_secret` },
        signal,
      );
    },
    revokeSubscriptionToken(tokenID, signal) {
      const token = requireItem(state.tokens, tokenID, 'Subscription token');
      token.revoked_at = updatedAt();
      token.active = false;
      return respond(token, signal);
    },
    setSubscriptionTokenEnabled(tokenID, enabled, signal) {
      const token = requireItem(state.tokens, tokenID, 'Subscription token');
      token.enabled = enabled;
      token.active = subscriptionKeyActive(token);
      return respond(token, signal);
    },
    async deleteSubscriptionToken(tokenID, signal) {
      assertActive(signal);
      requireItem(state.tokens, tokenID, 'Subscription token');
      state.tokens = state.tokens.filter((item) => item.id !== tokenID);
      tokenSecrets.delete(tokenID);
      await respond(undefined, signal);
    },
    ...createDemoCoreLogs(),
    listPanelLogs(filter = {}, signal) {
      settleTasks(state);
      const taskIDs = new Set(state.tasks.map((task) => task.id));
      let entries: PanelLog[] = [
        ...state.tasks.map((task) => ({
          id: `task:${task.id}`,
          time: task.updated_at,
          source: 'task' as const,
          level: task.status === 'failed' ? ('error' as const) : ('info' as const),
          code: task.kind,
          message: task.kind,
          status: task.status,
          task_id: task.id,
          metadata: {},
        })),
        ...state.logs
          .filter((log) => !taskIDs.has(String(log.metadata.task_id)))
          .map((log) => ({ ...log, id: `log:${log.id}`, status: '' })),
      ];
      entries = entries.filter(
        (entry) =>
          (!filter.level || entry.level === filter.level)
          && (!filter.search
            || `${entry.message} ${entry.code}`.toLowerCase().includes(filter.search.toLowerCase()))
          && (!filter.since || entry.time >= filter.since)
          && (!filter.until || entry.time < filter.until),
      );
      entries.sort((a, b) => b.time.localeCompare(a.time) || b.id.localeCompare(a.id));
      if (filter.beforeTime && filter.beforeID) {
        entries = entries.filter(
          (e) =>
            e.time < filter.beforeTime!
            || (e.time === filter.beforeTime && e.id < filter.beforeID!),
        );
      }
      const items = entries.slice(0, filter.limit ?? 10);
      const last = items.at(-1);
      return respond(
        {
          items,
          next:
            last && items.length < entries.length ? { time: last.time, id: last.id } : undefined,
        },
        signal,
      );
    },
    retryTask(taskID, signal) {
      const previous = requireItem(state.tasks, taskID, 'Task');
      if (
        !['failed', 'canceled'].includes(previous.status)
        || !['catalog-refresh', 'core-install', 'subscription-source-refresh'].includes(previous.kind)
      ) {
        throw new ApiRequestError('Retry unavailable', {
          status: 409,
          code: 'task_retry_unavailable',
        });
      }
      const existing = state.tasks.find((task) => task.idempotency_key === `panel-retry:${taskID}`);
      if (existing) return respond(existing, signal);
      const task = queueTask(state, previous.kind);
      task.idempotency_key = `panel-retry:${taskID}`;
      return respond(task, signal);
    },
    listLogs(filter = {}, signal) {
      let logs = state.logs
        .filter((entry) => matchesLog(entry, filter))
        .sort(
          (left, right) => right.time.localeCompare(left.time) || right.id.localeCompare(left.id),
        );
      if (filter.afterTime !== undefined && filter.afterID !== undefined) {
        logs = logs.filter(
          (entry) =>
            entry.time < filter.afterTime!
            || (entry.time === filter.afterTime && entry.id < filter.afterID!),
        );
      }
      const limit = Math.max(1, filter.limit ?? 100);
      const items = logs.slice(0, limit);
      const last = items.at(-1);
      return respond(
        {
          items,
          next:
            items.length < logs.length && last !== undefined
              ? { time: last.time, id: last.id }
              : undefined,
        },
        signal,
      );
    },
    getLog: (entryID, signal) => respond(requireItem(state.logs, entryID, 'Log entry'), signal),
    async* streamLogs(filter = {}, signal) {
      assertActive(signal);
      let sequence = 0;
      while (signal?.aborted !== true) {
        await wait(1_800, signal);
        const time = updatedAt();
        const entry: LogEntry = {
          id: `log_demo_live_${Date.now()}_${sequence}`,
          time,
          source: 'core',
          level: sequence % 4 === 3 ? 'debug' : 'info',
          code: 'runtime.sample',
          message: `Live demo sample ${sequence + 1} received.`,
          metadata: { active_connections: 18 + (sequence % 5) },
        };
        sequence += 1;
        state.logs.unshift(entry);
        if (matchesLog(entry, filter)) yield { id: `${entry.time}|${entry.id}`, entry: structuredClone(entry) };
      }
    },
    clearLogs(filter = {}, signal) {
      assertActive(signal);
      const before = state.logs.length;
      state.logs = state.logs.filter(
        (entry) =>
          !(
            (filter.source === undefined || entry.source === filter.source)
            && (filter.before === undefined || entry.time < filter.before)
          ),
      );
      return respond({ deleted: before - state.logs.length }, signal);
    },
    deleteLog(entryID, signal) {
      assertActive(signal);
      requireItem(state.logs, entryID, 'Log entry');
      state.logs = state.logs.filter((entry) => entry.id !== entryID);
      return respond({ id: entryID, deleted: true as const }, signal);
    },
    async* streamMetrics(signal) {
      while (!signal?.aborted) {
        yield { metrics: demoMetrics(state), runtime: structuredClone(state.runtime) };
        await new Promise<void>((resolve) => {
          const timer = setTimeout(done, 2000);
          function done() {
            clearTimeout(timer);
            signal?.removeEventListener('abort', done);
            resolve();
          }
          signal?.addEventListener('abort', done, { once: true });
        });
      }
    },
    getMetrics: (signal) => respond(demoMetrics(state), signal),
    getTrafficStatus(signal) {
      const period = state.trafficPeriods[0];
      if (period !== undefined && state.runtime.observation_state === 'running') {
        period.inbound_bytes += 486_210;
        period.outbound_bytes += 124_820;
        period.period_end = updatedAt();
      }
      return respond(demoMetrics(state), signal);
    },
    getMetricsHistory(filter, signal) {
      const result = demoMetricsHistory(filter.from, filter.to, filter.bucketSeconds);
      result.activation_bundle_id = filter.activationBundleID ?? result.activation_bundle_id;
      return respond(result, signal);
    },
    listTrafficPeriods(filter = {}, signal) {
      const filtered = state.trafficPeriods.filter(
        (period) =>
          (filter.activationBundleID === undefined
            || period.activation_bundle_id === filter.activationBundleID)
          && (filter.from === undefined || period.period_end >= filter.from)
          && (filter.to === undefined || period.period_start <= filter.to),
      );
      const limit = Math.max(1, filter.limit ?? 100);
      return respond({ items: filtered.slice(0, limit) }, signal);
    },
    getTrafficPeriod: (periodID, signal) =>
      respond(requireItem(state.trafficPeriods, periodID, 'Traffic period'), signal),
  };

  return client;
}

function coreFromCatalog(state: DemoState, asset: CatalogAsset): CoreArtifact {
  const id = nextID(state, 'core');
  return {
    id,
    exact_version: asset.version,
    os: asset.os,
    arch: asset.arch,
    variant: asset.variant,
    source_kind: 'official',
    repository_id: asset.repository_id,
    release_id: asset.release_id,
    asset_id: asset.asset_id,
    archive_sha256: asset.api_digest ?? asset.catalog_digest ?? '0'.repeat(64),
    binary_sha256: '7'.repeat(64),
    binary_path: `/var/lib/sing-box-panel/artifacts/${id}/sing-box`,
    reported_version: asset.version,
    feature_fingerprint: { source: 'demo-install' },
    created_at: updatedAt(),
  };
}

function wait(milliseconds: number, signal?: AbortSignal): Promise<void> {
  assertActive(signal);
  return new Promise((resolve, reject) => {
    let timer: ReturnType<typeof setTimeout>;
    const onAbort = () => {
      clearTimeout(timer);
      reject(signal?.reason ?? abortError());
    };
    timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort);
      resolve();
    }, milliseconds);
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}
