export type CapabilityLevel =
  | 'native_structured'
  | 'compatible_structured'
  | 'manual_json'
  | 'unavailable';

export interface Session {
  displayName: string;
}

export interface DashboardContext {
  view: {
    exactVersion: string;
  };
  running: {
    exactVersion: string;
    artifactName: string;
    digest: string;
  } | null;
  canonical: {
    revision: number;
    savedAt: string;
    hasUnappliedChanges: boolean;
  };
  applied: {
    bundle: string;
    revision: number;
    appliedAt: string;
  } | null;
  capability: {
    level: CapabilityLevel;
    label: string;
    warning: string | null;
  };
}

export type JsonObject = Record<string, unknown>;

export interface CanonicalDocument {
  schema_version: 1;
  global: JsonObject;
  nodes: Entity[];
  rules: Entity[];
  subscription: JsonObject;
}

export interface CanonicalSnapshot {
  id: string;
  sequence: number;
  parent_id?: string;
  schema_version: number;
  document: CanonicalDocument;
  document_json: string;
  sha256: string;
  created_at: string;
}

export type CanonicalChange =
  | { op: 'set'; path: string; value_json: string }
  | { op: 'unset'; path: string };

export interface CanonicalSave {
  revision: CanonicalSnapshot;
  task_id?: string;
  no_change: boolean;
}

export interface CanonicalRevisionPage {
  items: CanonicalSnapshot[];
  next_before_sequence?: number;
}

export type EntityCollection = 'nodes' | 'rules';

export interface Entity extends JsonObject {
  id: string;
  enabled: boolean;
  kind?: string;
}

export interface EntityList {
  revision: CanonicalSnapshot;
  entities: Entity[];
}

export type TaskStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'canceled'
  | 'superseded';

export interface Task {
  id: string;
  idempotency_key?: string;
  lane: 'runtime' | 'maintenance';
  kind: string;
  status: TaskStatus;
  generation: number;
  canonical_revision_id?: string;
  startup_artifact_id?: string;
  activation_bundle_id?: string;
  payload: unknown;
  result?: unknown;
  failure?: unknown;
  cancel_requested: boolean;
  attempt: number;
  lease_expires_at?: string;
  not_before?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskPage {
  items: Task[];
  next?: {
    created_at: string;
    id: string;
  };
}

export interface TaskFilter {
  kind?: string;
  lane?: Task['lane'];
  limit?: number;
  status?: TaskStatus;
}

export interface CatalogAsset {
  repository_id: number;
  release_id: number;
  asset_id: number;
  name: string;
  download_url: string;
  size: number;
  version: string;
  os: string;
  arch: string;
  variant: string;
  api_digest?: string;
  has_api_digest: boolean;
  catalog_digest?: string;
  has_catalog_digest: boolean;
}

export interface CatalogAssetList {
  validator: string;
  refreshed_at: string;
  assets: CatalogAsset[];
}

export interface CatalogAssetFilter {
  architecture?: string;
  exactVersion?: string;
  installable?: boolean;
  variant?: string;
}

export interface CoreArtifact {
  id: string;
  exact_version: string;
  os: string;
  arch: string;
  variant: string;
  source_kind: 'official' | 'user_verified';
  user_source?: string;
  repository_id?: number;
  release_id?: number;
  asset_id?: number;
  archive_sha256: string;
  binary_sha256: string;
  binary_path: string;
  reported_version: string;
  feature_fingerprint: unknown;
  verification_state: 'verified' | 'revoked' | 'quarantined';
  created_at: string;
}

export interface CoreArtifactPage {
  items: CoreArtifact[];
  next?: CoreArtifactCursor;
}

export interface CoreArtifactCursor {
  created_at: string;
  id: string;
}

export interface CoreArtifactFilter {
  architecture?: string;
  beforeID?: string;
  beforeTime?: string;
  exactVersion?: string;
  limit?: number;
  sourceKind?: CoreArtifact['source_kind'];
  variant?: string;
  verificationState?: CoreArtifact['verification_state'];
}

export interface CapabilityStatus {
  resolution: {
    exact_version: string;
    source: 'explicit' | 'running' | string;
  };
  support_level: CapabilityLevel;
  pinned: boolean;
  pin?: CapabilityPin;
  quarantined: boolean;
  reason_code?: string;
  presentation?: CapabilityPresentation;
}

export type CapabilityClassification =
  | 'supported'
  | 'intentionally_unsupported'
  | 'behavior_changed';

export interface CapabilitySemanticFact {
  id: string;
  canonical_path: string;
  classification: CapabilityClassification;
  owned_paths?: string[];
}

export interface CapabilityUIOption {
  value: string;
  label: string;
}

export interface CapabilityUIDescriptor {
  id: string;
  fact_id: string;
  kind: 'group' | 'text' | 'number' | 'boolean' | 'select' | 'json';
  label: string;
  help?: string;
  order?: number;
  options?: CapabilityUIOption[];
  visible_when?: {
    canonical_path: string;
    equals_json: string;
  };
}

export interface CapabilityPresentation {
  semantic_facts: CapabilitySemanticFact[];
  ui: CapabilityUIDescriptor[];
}

export interface CoreVersionResolution {
  exact_version: string;
  source: 'explicit' | 'running' | string;
}

export type StartupArtifactState = 'pending' | 'ready' | 'failed' | 'stale';
export type MonitoringTier = 'full' | 'limited' | 'process_only';

export interface CapabilityPin {
  repository: string;
  commit_sha: string;
  exact_core_version: string;
  manifest_sha256: string;
  support_level: CapabilityLevel;
  pinned_at: string;
}

export interface CapabilityDiagnostic {
  severity: 'warning' | string;
  code: string;
  fact_id: string;
  message: string;
}

export interface StructuredArtifact {
  id: string;
  canonical_revision_id: string;
  exact_core_version: string;
  capability_commit: string;
  capability_digest: string;
  renderer_version: string;
  core_artifact_id: string;
  config_sha256: string;
  config: JsonObject;
  diagnostics: CapabilityDiagnostic[];
  state: StartupArtifactState;
}

export interface StructuredRender {
  resolution: CoreVersionResolution;
  pin: CapabilityPin;
  artifact: StructuredArtifact;
  task: Task;
}

export interface StartupArtifactSummary {
  id: string;
  kind: 'structured' | 'manual';
  canonical_revision_id: string;
  exact_core_version: string;
  capability_commit?: string;
  capability_digest?: string;
  renderer_version: string;
  core_artifact_id: string;
  config_sha256: string;
  diagnostics: unknown[];
  state: StartupArtifactState;
  checked_at?: string;
  created_at: string;
}

export interface StartupArtifactPage {
  items: StartupArtifactSummary[];
  next?: { created_at: string; id: string };
}

export interface ManualArtifact {
  id: string;
  canonical_revision_id: string;
  exact_core_version: string;
  core_artifact_id: string;
  config_sha256: string;
  raw: string;
  diagnostics: unknown[];
  state: StartupArtifactState;
  checked_at?: string;
  created_at: string;
}

export interface ManualArtifactList {
  resolution: CoreVersionResolution;
  items: ManualArtifact[];
}

export interface ManualSave {
  resolution: CoreVersionResolution;
  preview: ManualReplacePreview;
  revision: CanonicalSnapshot;
  no_change: boolean;
  artifact: ManualArtifact;
  task: Task;
}

export interface ManualReverseMapping {
  available: boolean;
  reason_code?: string;
  capability?: ReattachPinEvidence;
  owned_partial: JsonObject;
  proposed_canonical: CanonicalDocument;
  residual_paths: string[];
  canonical_changed: boolean;
}

export interface ManualReplacePreview {
  resolution: CoreVersionResolution;
  base: CanonicalSnapshot;
  core_artifact_id: string;
  config_sha256: string;
  reverse: ManualReverseMapping;
}

export interface ActivationSummary {
  startup_artifact_id: string;
  canonical_revision_id: string;
  exact_core_version: string;
  core_artifact_id: string;
  config_sha256: string;
  subscription_snapshot_id: string;
  subscription_sha256: string;
  activation_bundle_id: string;
  activation_sha256: string;
  monitoring_tier: MonitoringTier;
}

export interface ActivationQueued {
  activation: ActivationSummary;
  task: Task;
}

export interface ReattachPinEvidence {
  repository: string;
  commit_sha: string;
  manifest_sha256: string;
  support_level: CapabilityLevel;
}

export interface ReattachEvidence {
  startup_artifact_id: string;
  config_sha256: string;
  base_revision_id: string;
  base_revision_sha256: string;
  current_head_id: string;
  current_head_sha256: string;
  exact_core_version: string;
  core_artifact_id: string;
  capability: ReattachPinEvidence;
}

export interface ReattachValue {
  present: boolean;
  value?: unknown;
}

export interface ReattachConflict {
  path: string;
  base: ReattachValue;
  current: ReattachValue;
  manual: ReattachValue;
}

export interface ManualReattachPreview {
  evidence: ReattachEvidence;
  base: CanonicalSnapshot;
  current: CanonicalSnapshot;
  manual: CanonicalDocument;
  owned_partial: JsonObject;
  merged: CanonicalDocument;
  residual_paths: string[];
  conflicts: ReattachConflict[];
}

export interface ManualReattachSave {
  preview: ManualReattachPreview;
  revision: CanonicalSnapshot;
  artifact: ManualArtifact;
  task: Task;
}

export type SubscriptionFormat = 'sing-box' | 'mihomo' | 'loon';

export interface SubscriptionChannelConfig {
  exclude_tags?: string[];
  exclude_types?: string[];
}

export interface SubscriptionChannel {
  id: string;
  name: string;
  format: SubscriptionFormat;
  config: SubscriptionChannelConfig;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SubscriptionChannelWrite {
  name: string;
  format: SubscriptionFormat;
  config: SubscriptionChannelConfig;
  enabled: boolean;
}

export type SubscriptionSourceKind = 'remote' | 'local';

export interface SubscriptionSource {
  id: string;
  name: string;
  source_kind: SubscriptionSourceKind;
  config: JsonObject;
  latest_snapshot?: unknown;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SubscriptionSourceWrite {
  name: string;
  source_kind: SubscriptionSourceKind;
  config: JsonObject;
  latest_snapshot?: unknown;
  enabled: boolean;
}

export interface SubscriptionToken {
  id: string;
  channel_id?: string;
  expires_at?: string;
  revoked_at?: string;
  created_at: string;
  active: boolean;
}

export interface CreatedSubscriptionToken {
  metadata: SubscriptionToken;
  token: string;
}

export interface SubscriptionTokenRotation {
  revoked: SubscriptionToken;
  created: SubscriptionToken;
  token: string;
}

export type LogSource = 'panel' | 'core' | 'task' | 'security';
export type LogLevel = 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';

export interface LogEntry {
  id: string;
  time: string;
  source: LogSource;
  level: LogLevel;
  code: string;
  message: string;
  metadata: JsonObject;
}

export interface LogPage {
  items: LogEntry[];
  next?: { time: string; id: string };
}

export interface LogFilter {
  source?: LogSource;
  level?: LogLevel;
  since?: string;
  limit?: number;
  afterTime?: string;
  afterID?: string;
}

export interface LogClearFilter {
  before?: string;
  source?: LogSource;
}

export interface TrafficPeriod {
  id: string;
  activation_bundle_id?: string;
  period_start: string;
  period_end: string;
  inbound_bytes: number;
  outbound_bytes: number;
  counters: JsonObject;
  created_at: string;
}

export interface MetricsSnapshot {
  available: boolean;
  reason_code?:
    | 'not_applied'
    | 'process_only'
    | 'no_collector_sample'
    | 'stale_collector_sample'
    | string;
  applied_bundle_id?: string;
  monitoring_tier?: MonitoringTier;
  collected_at: string;
  current_traffic_period?: TrafficPeriod;
}

export interface TrafficPeriodPage {
  items: TrafficPeriod[];
}

export interface TrafficPeriodFilter {
  activationBundleID?: string;
  from?: string;
  to?: string;
  limit?: number;
}

export interface ApiClient {
  getSession(signal?: AbortSignal): Promise<Session | null>;
  login(token: string, signal?: AbortSignal): Promise<Session>;
  logout(signal?: AbortSignal): Promise<void>;
  getDashboardContext(signal?: AbortSignal): Promise<DashboardContext>;
  listEntities(
    collection: EntityCollection,
    signal?: AbortSignal,
  ): Promise<EntityList>;
  saveEntity(
    collection: EntityCollection,
    entity: Entity,
    options: { baseRevision: string; existingID?: string },
    signal?: AbortSignal,
  ): Promise<CanonicalSave>;
  getCanonical(signal?: AbortSignal): Promise<CanonicalSnapshot>;
  replaceCanonical(
    documentJSON: string,
    baseRevision: string,
    signal?: AbortSignal,
  ): Promise<CanonicalSave>;
  patchCanonical(
    changes: CanonicalChange[],
    baseRevision: string,
    signal?: AbortSignal,
  ): Promise<CanonicalSave>;
  listRevisions(signal?: AbortSignal): Promise<CanonicalRevisionPage>;
  listTasks(filter?: TaskFilter, signal?: AbortSignal): Promise<TaskPage>;
  cancelTask(taskID: string, signal?: AbortSignal): Promise<Task>;
  listCatalogAssets(
    filter?: CatalogAssetFilter,
    signal?: AbortSignal,
  ): Promise<CatalogAssetList>;
  refreshCatalog(signal?: AbortSignal): Promise<Task>;
  listCoreArtifacts(
    filter?: CoreArtifactFilter,
    signal?: AbortSignal,
  ): Promise<CoreArtifactPage>;
  installCore(assetID: number, signal?: AbortSignal): Promise<Task>;
  removeCoreArtifact(artifactID: string, signal?: AbortSignal): Promise<void>;
  quarantineCoreArtifact(artifactID: string, signal?: AbortSignal): Promise<CoreArtifact>;
  revokeCoreArtifact(artifactID: string, signal?: AbortSignal): Promise<CoreArtifact>;
  getCoreCapability(
    exactVersion: string,
    signal?: AbortSignal,
  ): Promise<CapabilityStatus>;
  renderStructured(
    input: {
      coreVersion: string;
      coreArtifactID?: string;
      allowCompatible: boolean;
    },
    signal?: AbortSignal,
  ): Promise<StructuredRender>;
  listStartupArtifacts(
    filter: {
      canonicalRevisionID?: string;
      coreVersion?: string;
      coreArtifactID?: string;
      kind?: StartupArtifactSummary['kind'];
      state?: StartupArtifactState;
      beforeTime?: string;
      beforeID?: string;
      limit?: number;
    },
    signal?: AbortSignal,
  ): Promise<StartupArtifactPage>;
  listManualArtifacts(
    filter: { coreVersion?: string; coreArtifactID?: string; limit?: number },
    signal?: AbortSignal,
  ): Promise<ManualArtifactList>;
  getManualArtifact(artifactID: string, signal?: AbortSignal): Promise<ManualArtifact>;
  replaceManualArtifact(
    input: {
      baseRevision: string;
      coreVersion?: string;
      coreArtifactID: string;
      raw: string;
      allowCompatible?: boolean;
    },
    signal?: AbortSignal,
  ): Promise<ManualSave>;
  previewManualReplacement(
    input: {
      baseRevision: string;
      coreVersion?: string;
      coreArtifactID: string;
      raw: string;
      allowCompatible?: boolean;
    },
    signal?: AbortSignal,
  ): Promise<ManualReplacePreview>;
  discardManualArtifact(
    artifactID: string,
    signal?: AbortSignal,
  ): Promise<ManualArtifact>;
  activateStartupArtifact(
    artifactID: string,
    monitoringTier: MonitoringTier,
    signal?: AbortSignal,
  ): Promise<ActivationQueued>;
  previewManualReattach(
    artifactID: string,
    signal?: AbortSignal,
  ): Promise<ManualReattachPreview>;
  applyManualReattach(
    artifactID: string,
    input: {
      evidence: ReattachEvidence;
      decisions: Record<string, 'current' | 'manual'>;
    },
    signal?: AbortSignal,
  ): Promise<ManualReattachSave>;
  listSubscriptionChannels(signal?: AbortSignal): Promise<SubscriptionChannel[]>;
  createSubscriptionChannel(
    input: SubscriptionChannelWrite,
    signal?: AbortSignal,
  ): Promise<SubscriptionChannel>;
  updateSubscriptionChannel(
    channelID: string,
    input: SubscriptionChannelWrite,
    updatedAt: string,
    signal?: AbortSignal,
  ): Promise<SubscriptionChannel>;
  deleteSubscriptionChannel(
    channelID: string,
    updatedAt: string,
    signal?: AbortSignal,
  ): Promise<void>;
  listSubscriptionSources(signal?: AbortSignal): Promise<SubscriptionSource[]>;
  createSubscriptionSource(
    input: SubscriptionSourceWrite,
    signal?: AbortSignal,
  ): Promise<SubscriptionSource>;
  updateSubscriptionSource(
    sourceID: string,
    input: Omit<SubscriptionSourceWrite, 'latest_snapshot'>,
    updatedAt: string,
    signal?: AbortSignal,
  ): Promise<SubscriptionSource>;
  updateSubscriptionSourceSnapshot(
    sourceID: string,
    snapshot: unknown,
    updatedAt: string,
    signal?: AbortSignal,
  ): Promise<SubscriptionSource>;
  deleteSubscriptionSource(
    sourceID: string,
    updatedAt: string,
    signal?: AbortSignal,
  ): Promise<void>;
  listSubscriptionTokens(signal?: AbortSignal): Promise<SubscriptionToken[]>;
  createSubscriptionToken(
    input: { channelID?: string; expiresAt?: string },
    signal?: AbortSignal,
  ): Promise<CreatedSubscriptionToken>;
  rotateSubscriptionToken(
    tokenID: string,
    expiresAt?: string,
    signal?: AbortSignal,
  ): Promise<SubscriptionTokenRotation>;
  revokeSubscriptionToken(
    tokenID: string,
    signal?: AbortSignal,
  ): Promise<SubscriptionToken>;
  listLogs(filter?: LogFilter, signal?: AbortSignal): Promise<LogPage>;
  getLog(entryID: string, signal?: AbortSignal): Promise<LogEntry>;
  clearLogs(filter?: LogClearFilter, signal?: AbortSignal): Promise<{ deleted: number }>;
  deleteLog(entryID: string, signal?: AbortSignal): Promise<{ id: string; deleted: true }>;
  getMetrics(signal?: AbortSignal): Promise<MetricsSnapshot>;
  getTrafficStatus(signal?: AbortSignal): Promise<MetricsSnapshot>;
  listTrafficPeriods(
    filter?: TrafficPeriodFilter,
    signal?: AbortSignal,
  ): Promise<TrafficPeriodPage>;
  getTrafficPeriod(periodID: string, signal?: AbortSignal): Promise<TrafficPeriod>;
}

export class ApiRequestError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields?: Record<string, string>;

  constructor(
    message: string,
    options: {
      status: number;
      code: string;
      fields?: Record<string, string>;
    },
  ) {
    super(message);
    this.name = 'ApiRequestError';
    this.status = options.status;
    this.code = options.code;
    this.fields = options.fields;
  }
}
