import {
  isLosslessNumber,
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import type {
  ApiClient,
  CanonicalSnapshot,
  CatalogAsset,
  ConfigurationFile,
  ConfigurationFileWrite,
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
} from '../api-client';

import { ApiRequestError } from '../api-client';
import { createDemoCoreLogs } from './demo-core-logs';
import { DEFAULT_APPEARANCE } from '../../theme/appearance';
import { reviewedSchemaManifest } from '../../schemas/generated';
import { demoBackupSettings, demoRestoreSettings } from './demo-panel-backup';
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

interface DemoState extends ReturnType<typeof createDemoData> {
  nextID: number;
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
  filter: { beforeID?: string; beforeTime?: string; limit?: number; offset?: number },
): { items: T[]; next?: { created_at: string; id: string } } {
  const sorted = [...items].sort(
    (left, right) =>
      right.created_at.localeCompare(left.created_at) || right.id.localeCompare(left.id),
  );
  let start = filter.offset ?? 0;
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

  return {
    ...data,
    nextID: 100,
    session: { displayName: 'Demo administrator' },
  };
}

function addRuntimeTransition(
  state: DemoState,
  runtimeState: 'running' | 'stopped',
  reason: string,
): void {
  state.runtimeHistory.items.unshift({
    id: Math.max(0, ...state.runtimeHistory.items.map((item) => item.id)) + 1,
    state: runtimeState,
    reason,
    activation_bundle_id: state.runtime.applied_bundle_id,
    generation: state.runtime.target_generation,
    pid: state.runtime.running?.pid,
    process_started_at: state.runtime.running?.started_at,
    occurred_at: updatedAt(),
  });
}

function stoppedRuntime(state: DemoState): void {
  state.runtime = {
    ...state.runtime,
    desired_running: false,
    target_generation: state.runtime.target_generation,
    observation_state: 'stopped',
    running: undefined,
    loaded_canonical_revision_id: undefined,
  };
  addRuntimeTransition(state, 'stopped', 'stop_succeeded');
}

function runningRuntime(state: DemoState, bundleID?: string): void {
  const core
    = state.cores.find((item) => item.id === state.runtime.enabled_core?.core_artifact_id)
      ?? state.cores.find((item) => item.id === state.runtime.running?.core_artifact_id)
      ?? state.cores[0];
  if (core === undefined) return;
  const processToken = `demo-process-${state.nextID++}-${Date.now()}`;
  state.runtime = {
    ...state.runtime,
    enabled_core: { core_artifact_id: core.id, exact_core_version: core.exact_version },
    desired_running: true,
    desired_bundle_id: bundleID ?? state.runtime.desired_bundle_id,
    applied_bundle_id: bundleID ?? state.runtime.applied_bundle_id,
    target_generation: state.runtime.target_generation,
    observation_state: 'running',
    loaded_canonical_revision_id: state.canonical.id,
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
  addRuntimeTransition(state, 'running', 'start_succeeded');
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
  return { revision, no_change: false } as const;
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
  let panelSecrets = { management: 'demo-management-token-for-config-backup', github: '', identity: '' };
  let panelSettings: PanelSettingsView = {
    service: { data_dir: '/var/lib/sing-box-panel', base_path: '', secure_cookie: false, catalog_refresh_interval_hours: 12, traffic_period_months: 1, sample_retention_days: 90, private_source_cidrs: [], core_log_retention_days: 7, core_log_max_files: 0, core_log_max_file_size_mib: 32 },
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
  function updateConfigurationFile(input: ConfigurationFileWrite) {
    if (input.revision !== configurationFile.revision) conflict('Configuration file');
    if (input.content === configurationFile.content) return configurationFile;
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
    return configurationFile;
  }
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
  const client: ApiClient = {
    newInboundDefaults: (type, signal) => respond({ type }, signal),
    getConfigurationFile: (signal) => respond(configurationFile, signal),
    async saveConfigurationFile(input, signal) {
      assertActive(signal);
      return respond(updateConfigurationFile(input), signal);
    },
    exportPanelBackup: signal => respond({ format: 'sing-box-panel-backup', version: 1, exported_at: updatedAt(), panel_settings: demoBackupSettings(panelSettings, panelSecrets), sing_box_configuration: configurationFile.content }, signal),
    async restorePanelBackup(input, signal) {
      assertActive(signal);
      if (input.settings_revision !== panelSettings.revision || input.configuration_revision !== configurationFile.revision) conflict('Saved configuration');
      const restored = demoRestoreSettings(input.backup, panelSettings.revision + 1);
      updateConfigurationFile({
        revision: configurationFile.revision, content: input.backup.sing_box_configuration,
      });
      const reauthenticate = restored.secrets.management !== panelSecrets.management;
      restored.view.restart_required = restored.view.preferences.listen_host !== '127.0.0.1' || restored.view.preferences.listen_port !== 3000 || restored.view.preferences.external_origin !== '' || restored.view.service.data_dir !== '/var/lib/sing-box-panel' || restored.view.service.base_path !== '' || restored.view.service.secure_cookie;
      panelSettings = restored.view;
      panelSecrets = restored.secrets;
      if (reauthenticate) state.session = null;
      return respond({ settings: panelSettings, reauthentication_required: reauthenticate }, signal);
    },
    getPanelSettings: (signal) => respond(panelSettings, signal),
    async savePanelSettings(input, signal) {
      assertActive(signal);
      if (input.revision !== panelSettings.revision) conflict('Panel settings');
      panelSecrets = { management: input.management_token || panelSecrets.management, github: input.clear_github_token ? '' : input.github_token || panelSecrets.github, identity: input.clear_identity_key ? '' : input.identity_key || panelSecrets.identity };
      panelSettings = {
        revision: panelSettings.revision + 1,
        service: input.service ?? panelSettings.service,
        preferences: structuredClone(input.preferences),
        github_token_configured: input.clear_github_token
          ? false
          : Boolean(input.github_token) || panelSettings.github_token_configured,
        identity_key_configured:
          !input.clear_identity_key && (Boolean(input.identity_key) || panelSettings.identity_key_configured),
        restart_required:
          input.preferences.listen_host !== '127.0.0.1'
          || input.preferences.listen_port !== 3000
          || input.preferences.external_origin !== ''
          || (input.service !== undefined && (input.service.data_dir !== '/var/lib/sing-box-panel' || input.service.base_path !== '' || input.service.secure_cookie)),
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
    listCatalogAssets(filter = {}, signal) {
      const assets = state.catalog.assets.filter(
        (asset) =>
          (filter.exactVersion === undefined || asset.version === filter.exactVersion)
          && (filter.architecture === undefined || asset.arch === filter.architecture)
          && (filter.variant === undefined || asset.variant === filter.variant),
      );
      return respond({ ...state.catalog, assets }, signal);
    },
    refreshCatalog(force = false, signal) {
      assertActive(signal);
      state.catalog.refreshed_at = updatedAt();
      return respond({
        refreshed_at: state.catalog.refreshed_at, not_modified: !force,
        releases: state.catalog.assets.length, assets: state.catalog.assets.length,
      }, signal);
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
      const asset = state.catalog.assets.find(item => item.asset_id === assetID) ?? notFound('Catalog asset', String(assetID));
      const core = state.cores.find(item => item.asset_id === assetID) ?? coreFromCatalog(state, asset);
      if (!state.cores.includes(core)) state.cores.unshift(core);
      return respond(core, signal);
    },
    importCoreArchive(input, signal) {
      assertActive(signal);
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

      return respond(state.cores[0], signal);
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
      const revision = state.canonical;
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
      artifact.state = 'ready';
      artifact.checked_at = updatedAt();
      return respond(
        {
          support: {
            structured: reviewedSchemaManifest[core.exact_version] !== undefined,
            exact_version: core.exact_version,
          },
          artifact,
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
      artifact.state = 'ready';
      artifact.checked_at = updatedAt();
      return respond(artifact, signal);
    },
    activateStartupArtifact(artifactID, monitoringTier, signal) {
      assertActive(signal);
      const artifact = requireItem(state.startupArtifacts, artifactID, 'Startup artifact');
      const bundleID = nextID(state, 'bundle');
      runningRuntime(state, bundleID);
      return respond(
        {
          status: state.runtime,
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
        },
        signal,
      );
    },
    getRuntimeStatus(signal) {
      return respond(state.runtime, signal);
    },
    getRuntimeHistory(filter = {}, signal) {
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
      if (!state.runtime.enabled_core) throw new Error('No core version is enabled.');
      requireParsedFile();
      if (state.runtime.observation_state !== 'running') runningRuntime(state);
      return respond(state.runtime, signal);
    },
    enableCore(artifactID, signal) {
      assertActive(signal);
      requireParsedFile();
      const artifact = requireItem(state.cores, artifactID, 'Core artifact');
      const platform = demoSystemStatus(state).platform;
      if (
        artifact.os !== platform?.os
        || artifact.arch !== platform?.arch
      ) {
        throw new Error('The core must match the demo platform.');
      }
      const wasRunning = state.runtime.observation_state === 'running';
      if (wasRunning) stoppedRuntime(state);
      state.runtime.rollback_bundle_id = state.runtime.applied_bundle_id;
      state.runtime.applied_bundle_id = nextID(state, 'bundle');
      state.runtime.desired_bundle_id = state.runtime.applied_bundle_id;
      state.runtime.enabled_core = { core_artifact_id: artifact.id, exact_core_version: artifact.exact_version };
      if (wasRunning) runningRuntime(state);
      else state.runtime.target_generation += 1;

      return respond(state.runtime, signal);
    },
    disableCore(artifactID, signal) {
      assertActive(signal);
      requireItem(state.cores, artifactID, 'Core artifact');
      if (state.runtime.enabled_core?.core_artifact_id !== artifactID) {
        throw new Error('The requested core is no longer enabled.');
      }
      stoppedRuntime(state);
      state.runtime.enabled_core = undefined;
      state.runtime.applied_bundle_id = undefined;
      state.runtime.desired_bundle_id = undefined;
      state.runtime.rollback_bundle_id = undefined;

      return respond(state.runtime, signal);
    },
    stopRuntime(signal) {
      assertActive(signal);
      stoppedRuntime(state);
      return respond(state.runtime, signal);
    },
    restartRuntime(signal) {
      assertActive(signal);
      if (!state.runtime.enabled_core) throw new Error('No core version is enabled.');
      requireParsedFile();
      runningRuntime(state);
      return respond(state.runtime, signal);
    },
    rollbackRuntime(activationBundleID, signal) {
      assertActive(signal);
      runningRuntime(state, activationBundleID);
      return respond(state.runtime, signal);
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
      source.updated_at = updatedAt();
      const version = state.sourceVersions.get(sourceID)?.[0];
      return respond({ source_id: sourceID, version_id: version?.id ?? nextID(state, 'version'), format: 'sing-box', sha256: version?.sha256 ?? '8'.repeat(64), node_count: 2, fetched_at: source.updated_at }, signal);
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
      return respond({ ...pageByCreatedAt(keys, filter), total: keys.length }, signal);
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
      let entries: PanelLog[] = [
        ...state.logs.map(log => ({ ...log, id: `log:${log.id}`, status: '' })),
        ...state.runtimeHistory.items.map((transition): PanelLog => ({
          id: `runtime:${transition.id}`, time: transition.occurred_at, source: 'runtime',
          level: transition.state === 'failed' ? 'error' : 'info', code: transition.reason,
          message: transition.reason, status: transition.state,
          metadata: Object.fromEntries(Object.entries({
            pid: transition.pid, process_started_at: transition.process_started_at,
            generation: transition.generation, activation_bundle_id: transition.activation_bundle_id,
            uncertain_since: transition.uncertain_since,
          }).filter(([, value]) => value !== undefined && value !== null && value !== '')),
        })),
      ];
      entries = entries.filter(
        (entry) =>
          (!filter.level || entry.level === filter.level)
          && ((!filter.search && !filter.searchCodes?.length)
            || (Boolean(filter.search) && `${entry.message} ${entry.code}`.toLowerCase().includes(filter.search!.toLowerCase()))
            || Boolean(filter.searchCodes?.includes(entry.code)))
          && (!filter.since || entry.time >= filter.since)
          && (!filter.until || entry.time < filter.until),
      );
      const total = entries.length;
      entries.sort((a, b) => b.time.localeCompare(a.time) || b.id.localeCompare(a.id));
      if (filter.beforeTime && filter.beforeID) {
        entries = entries.filter(
          (e) =>
            e.time < filter.beforeTime!
            || (e.time === filter.beforeTime && e.id < filter.beforeID!),
        );
      }
      const offset = filter.offset ?? 0;
      const items = entries.slice(offset, offset + (filter.limit ?? 10));
      const last = items.at(-1);
      return respond(
        {
          items,
          total,
          next:
            last && offset + items.length < entries.length ? { time: last.time, id: last.id } : undefined,
        },
        signal,
      );
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
        yield {
          metrics: demoMetrics(state, panelSettings.preferences.traffic_quota_gib),
          runtime: structuredClone(state.runtime),
        };
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
    async* streamDashboard(signal) {
      while (!signal?.aborted) {
        const collectedAt = updatedAt();
        const history1H = demoMetricsHistory(
          new Date(Date.parse(collectedAt) - 3_600_000).toISOString(),
          collectedAt,
          60,
        );
        const history24H = demoMetricsHistory(
          new Date(Date.parse(collectedAt) - 86_400_000).toISOString(),
          collectedAt,
          300,
        );
        const activity = await client.listPanelLogs({ limit: 2 }, signal);
        yield {
          collected_at: collectedAt,
          history_1h: history1H,
          history_24h: history24H,
          runtime_24h: structuredClone(state.runtimeHistory),
          activity,
        };
        await new Promise<void>((resolve) => {
          const timer = setTimeout(done, 30_000);
          function done() {
            clearTimeout(timer);
            signal?.removeEventListener('abort', done);
            resolve();
          }
          signal?.addEventListener('abort', done, { once: true });
        });
      }
    },
    getMetrics: (signal) => respond(demoMetrics(state, panelSettings.preferences.traffic_quota_gib), signal),
    getTrafficStatus(signal) {
      const period = state.trafficPeriods[0];
      if (period !== undefined && state.runtime.observation_state === 'running') {
        period.inbound_bytes += 486_210;
        period.outbound_bytes += 124_820;
        period.period_end = updatedAt();
      }
      return respond(demoMetrics(state, panelSettings.preferences.traffic_quota_gib), signal);
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
