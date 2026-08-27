export type CapabilityLevel
  = | 'native_structured'
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
  canonical: {
    revision: number;
    savedAt: string;
    hasUnappliedChanges: boolean;
  };
  running: {
    exactVersion: string;
    artifactName: string;
    digest: string;
  } | null;
}

export type JsonObject = Record<string, unknown>;

export interface CanonicalDocument {
  nodes: Entity[];
  rules: Entity[];
  schema_version: 1;
  global: JsonObject;
  subscription: JsonObject;
}

export interface CanonicalSnapshot {
  id: string;
  sha256: string;
  sequence: number;
  created_at: string;
  parent_id?: string;
  document_json: string;
  schema_version: number;
  document: CanonicalDocument;
}

export type CanonicalChange
  = | { op: 'set'; path: string; value_json: string }
    | { op: 'unset'; path: string };

export interface CanonicalSave {
  task_id?: string;
  no_change: boolean;
  revision: CanonicalSnapshot;
}

export interface CanonicalRevisionPage {
  items: CanonicalSnapshot[];
  next_before_sequence?: number;
}

export type EntityCollection = 'nodes' | 'rules';

export interface Entity extends JsonObject {
  id: string;
  kind?: string;
  enabled: boolean;
}

export interface EntityList {
  entities: Entity[];
  revision: CanonicalSnapshot;
}

export type TaskStatus
  = | 'queued'
    | 'running'
    | 'succeeded'
    | 'failed'
    | 'canceled'
    | 'superseded';

export interface Task {
  id: string;
  kind: string;
  attempt: number;
  payload: unknown;
  result?: unknown;
  failure?: unknown;
  created_at: string;
  generation: number;
  status: TaskStatus;
  updated_at: string;
  not_before?: string;
  idempotency_key?: string;
  cancel_requested: boolean;
  lease_expires_at?: string;
  startup_artifact_id?: string;
  activation_bundle_id?: string;
  canonical_revision_id?: string;
  lane: 'runtime' | 'maintenance';
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
  limit?: number;
  lane?: Task['lane'];
  status?: TaskStatus;
}

export interface CatalogAsset {
  os: string;
  arch: string;
  name: string;
  size: number;
  variant: string;
  version: string;
  asset_id: number;
  release_id: number;
  api_digest?: string;
  download_url: string;
  repository_id: number;
  catalog_digest?: string;
  has_api_digest: boolean;
  has_catalog_digest: boolean;
}

export interface CatalogAssetList {
  validator: string;
  refreshed_at: string;
  assets: CatalogAsset[];
}

export interface CatalogAssetFilter {
  variant?: string;
  architecture?: string;
  exactVersion?: string;
  installable?: boolean;
}

export interface CoreArtifact {
  id: string;
  os: string;
  arch: string;
  variant: string;
  asset_id?: number;
  created_at: string;
  binary_path: string;
  release_id?: number;
  user_source?: string;
  binary_sha256: string;
  exact_version: string;
  archive_sha256: string;
  repository_id?: number;
  reported_version: string;
  feature_fingerprint: unknown;
  source_kind: 'official' | 'user_verified';
  verification_state: 'verified' | 'revoked' | 'quarantined';
}

export interface CoreArtifactPage {
  items: CoreArtifact[];
  next?: CoreArtifactCursor;
}

export interface CoreArtifactCursor {
  id: string;
  created_at: string;
}

export interface CoreArtifactFilter {
  limit?: number;
  variant?: string;
  beforeID?: string;
  beforeTime?: string;
  architecture?: string;
  exactVersion?: string;
  sourceKind?: CoreArtifact['source_kind'];
  verificationState?: CoreArtifact['verification_state'];
}

export interface CapabilityStatus {
  pinned: boolean;
  pin?: CapabilityPin;
  quarantined: boolean;
  reason_code?: string;
  support_level: CapabilityLevel;
  presentation?: CapabilityPresentation;
  resolution: {
    exact_version: string;
    source: 'explicit' | 'running' | string;
  };
}

export type CapabilityClassification
  = | 'supported'
    | 'intentionally_unsupported'
    | 'behavior_changed';

export interface CapabilitySemanticFact {
  id: string;
  canonical_path: string;
  owned_paths?: string[];
  classification: CapabilityClassification;
}

export interface CapabilityUIOption {
  label: string;
  value: string;
}

export interface CapabilityUIDescriptor {
  id: string;
  help?: string;
  label: string;
  order?: number;
  fact_id: string;
  options?: CapabilityUIOption[];
  kind: 'group' | 'text' | 'number' | 'boolean' | 'select' | 'json';
  visible_when?: {
    canonical_path: string;
    equals_json: string;
  };
}

export interface CapabilityPresentation {
  ui: CapabilityUIDescriptor[];
  semantic_facts: CapabilitySemanticFact[];
}

export interface CoreVersionResolution {
  exact_version: string;
  source: 'explicit' | 'running' | string;
}

export type StartupArtifactState = 'pending' | 'ready' | 'failed' | 'stale';
export type MonitoringTier = 'full' | 'limited' | 'process_only';

export interface CapabilityPin {
  pinned_at: string;
  commit_sha: string;
  repository: string;
  manifest_sha256: string;
  exact_core_version: string;
  support_level: CapabilityLevel;
}

export interface CapabilityDiagnostic {
  code: string;
  fact_id: string;
  message: string;
  severity: 'warning' | string;
}

export interface StructuredArtifact {
  id: string;
  config: JsonObject;
  config_sha256: string;
  core_artifact_id: string;
  renderer_version: string;
  capability_commit: string;
  capability_digest: string;
  exact_core_version: string;
  state: StartupArtifactState;
  canonical_revision_id: string;
  diagnostics: CapabilityDiagnostic[];
}

export interface StructuredRender {
  task: Task;
  pin: CapabilityPin;
  artifact: StructuredArtifact;
  resolution: CoreVersionResolution;
}

export interface StartupArtifactSummary {
  id: string;
  created_at: string;
  checked_at?: string;
  config_sha256: string;
  diagnostics: unknown[];
  core_artifact_id: string;
  renderer_version: string;
  capability_commit?: string;
  capability_digest?: string;
  exact_core_version: string;
  state: StartupArtifactState;
  canonical_revision_id: string;
  kind: 'structured' | 'manual';
}

export interface StartupArtifactPage {
  items: StartupArtifactSummary[];
  next?: { created_at: string; id: string };
}

export interface ManualArtifact {
  id: string;
  raw: string;
  created_at: string;
  checked_at?: string;
  config_sha256: string;
  diagnostics: unknown[];
  core_artifact_id: string;
  exact_core_version: string;
  state: StartupArtifactState;
  canonical_revision_id: string;
}

export type ManualArtifactSummary = Omit<ManualArtifact, 'raw'>;

export interface ManualArtifactList {
  items: ManualArtifactSummary[];
  resolution: CoreVersionResolution;
  next?: { created_at: string; id: string };
}

export interface ManualSave {
  task: Task;
  no_change: boolean;
  artifact: ManualArtifact;
  revision: CanonicalSnapshot;
  preview: ManualReplacePreview;
  resolution: CoreVersionResolution;
}

export interface ManualReverseMapping {
  available: boolean;
  reason_code?: string;
  residual_paths: string[];
  owned_partial: JsonObject;
  canonical_changed: boolean;
  capability?: ReattachPinEvidence;
  proposed_canonical: CanonicalDocument;
}

export interface ManualReplacePreview {
  config_sha256: string;
  base: CanonicalSnapshot;
  core_artifact_id: string;
  reverse: ManualReverseMapping;
  resolution: CoreVersionResolution;
}

export interface ActivationSummary {
  config_sha256: string;
  core_artifact_id: string;
  activation_sha256: string;
  exact_core_version: string;
  startup_artifact_id: string;
  subscription_sha256: string;
  activation_bundle_id: string;
  canonical_revision_id: string;
  monitoring_tier: MonitoringTier;
  subscription_snapshot_id: string;
}

export interface ActivationQueued {
  task: Task;
  activation: ActivationSummary;
}

export interface ReattachPinEvidence {
  commit_sha: string;
  repository: string;
  manifest_sha256: string;
  support_level: CapabilityLevel;
}

export interface ReattachEvidence {
  config_sha256: string;
  current_head_id: string;
  base_revision_id: string;
  core_artifact_id: string;
  exact_core_version: string;
  current_head_sha256: string;
  startup_artifact_id: string;
  base_revision_sha256: string;
  capability: ReattachPinEvidence;
}

export interface ReattachValue {
  value?: unknown;
  present: boolean;
}

export interface ReattachConflict {
  path: string;
  base: ReattachValue;
  manual: ReattachValue;
  current: ReattachValue;
}

export interface ManualReattachPreview {
  base: CanonicalSnapshot;
  residual_paths: string[];
  manual: CanonicalDocument;
  merged: CanonicalDocument;
  owned_partial: JsonObject;
  current: CanonicalSnapshot;
  evidence: ReattachEvidence;
  conflicts: ReattachConflict[];
}

export interface ManualReattachSave {
  task: Task;
  artifact: ManualArtifact;
  revision: CanonicalSnapshot;
  preview: ManualReattachPreview;
}

export type SubscriptionFormat = 'sing-box' | 'mihomo' | 'loon';

export interface SubscriptionChannelConfig {
  exclude_tags?: string[];
  exclude_types?: string[];
}

export interface SubscriptionChannel {
  id: string;
  name: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  format: SubscriptionFormat;
  config: SubscriptionChannelConfig;
}

export type SubscriptionChannelSummary = Omit<SubscriptionChannel, 'config'>;

export interface SubscriptionChannelWrite {
  name: string;
  enabled: boolean;
  format: SubscriptionFormat;
  config: SubscriptionChannelConfig;
}

export type SubscriptionSourceKind = 'remote' | 'local';

export interface SubscriptionSource {
  id: string;
  name: string;
  enabled: boolean;
  config: JsonObject;
  created_at: string;
  updated_at: string;
  latest_snapshot?: unknown;
  source_kind: SubscriptionSourceKind;
}

export type SubscriptionSourceSummary = Omit<SubscriptionSource, 'config' | 'latest_snapshot'> & {
  has_snapshot: boolean;
};

export interface SubscriptionSourceWrite {
  name: string;
  enabled: boolean;
  config: JsonObject;
  latest_snapshot?: unknown;
  source_kind: SubscriptionSourceKind;
}

export interface SubscriptionToken {
  id: string;
  active: boolean;
  created_at: string;
  expires_at?: string;
  revoked_at?: string;
}

export interface SubscriptionCursor {
  id: string;
  created_at: string;
}

export interface SubscriptionListFilter {
  limit?: number;
  beforeID?: string;
  beforeTime?: string;
}

export interface SubscriptionChannelPage {
  next?: SubscriptionCursor;
  items: SubscriptionChannelSummary[];
}

export interface SubscriptionSourcePage {
  next?: SubscriptionCursor;
  items: SubscriptionSourceSummary[];
}

export interface SubscriptionTokenPage {
  next?: SubscriptionCursor;
  items: SubscriptionToken[];
}

export interface CreatedSubscriptionToken {
  token: string;
  metadata: SubscriptionToken;
}

export interface SubscriptionTokenRotation {
  token: string;
  created: SubscriptionToken;
  revoked: SubscriptionToken;
}

export type LogSource = 'panel' | 'core' | 'task' | 'security';
export type LogLevel = 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';

export interface LogEntry {
  id: string;
  code: string;
  time: string;
  level: LogLevel;
  message: string;
  source: LogSource;
  metadata: JsonObject;
}

export interface LogPage {
  items: LogEntry[];
  next?: { time: string; id: string };
}

export interface LogFilter {
  limit?: number;
  since?: string;
  afterID?: string;
  level?: LogLevel;
  afterTime?: string;
  source?: LogSource;
}

export interface LogClearFilter {
  before?: string;
  source?: LogSource;
}

export interface TrafficPeriod {
  id: string;
  created_at: string;
  period_end: string;
  counters: JsonObject;
  period_start: string;
  inbound_bytes: number;
  outbound_bytes: number;
  activation_bundle_id?: string;
}

export interface MetricsSnapshot {
  available: boolean;
  collected_at: string;
  applied_bundle_id?: string;
  monitoring_tier?: MonitoringTier;
  current_traffic_period?: TrafficPeriod;
  reason_code?:
    | 'not_applied'
    | 'process_only'
    | 'no_collector_sample'
    | 'stale_collector_sample'
    | string;
}

export interface TrafficPeriodPage {
  items: TrafficPeriod[];
}

export interface TrafficPeriodFilter {
  to?: string;
  from?: string;
  limit?: number;
  activationBundleID?: string;
}

export interface ApiClient {
  logout: (signal?: AbortSignal) => Promise<void>;
  refreshCatalog: (signal?: AbortSignal) => Promise<Task>;
  getSession: (signal?: AbortSignal) => Promise<Session | null>;
  getMetrics: (signal?: AbortSignal) => Promise<MetricsSnapshot>;
  login: (token: string, signal?: AbortSignal) => Promise<Session>;
  getCanonical: (signal?: AbortSignal) => Promise<CanonicalSnapshot>;
  cancelTask: (taskID: string, signal?: AbortSignal) => Promise<Task>;
  getLog: (entryID: string, signal?: AbortSignal) => Promise<LogEntry>;
  getTrafficStatus: (signal?: AbortSignal) => Promise<MetricsSnapshot>;
  installCore: (assetID: number, signal?: AbortSignal) => Promise<Task>;
  listRevisions: (signal?: AbortSignal) => Promise<CanonicalRevisionPage>;
  getDashboardContext: (signal?: AbortSignal) => Promise<DashboardContext>;
  listLogs: (filter?: LogFilter, signal?: AbortSignal) => Promise<LogPage>;
  listTasks: (filter?: TaskFilter, signal?: AbortSignal) => Promise<TaskPage>;
  removeCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<void>;
  getTrafficPeriod: (periodID: string, signal?: AbortSignal) => Promise<TrafficPeriod>;
  revokeCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<CoreArtifact>;
  getManualArtifact: (artifactID: string, signal?: AbortSignal) => Promise<ManualArtifact>;
  clearLogs: (filter?: LogClearFilter, signal?: AbortSignal) => Promise<{ deleted: number }>;
  quarantineCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<CoreArtifact>;
  deleteLog: (entryID: string, signal?: AbortSignal) => Promise<{ id: string; deleted: true }>;
  getSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<SubscriptionSource>;
  getSubscriptionChannel: (channelID: string, signal?: AbortSignal) => Promise<SubscriptionChannel>;
  listEntities: (
    collection: EntityCollection,
    signal?: AbortSignal,
  ) => Promise<EntityList>;
  discardManualArtifact: (
    artifactID: string,
    signal?: AbortSignal,
  ) => Promise<ManualArtifact>;
  getCoreCapability: (
    exactVersion: string,
    signal?: AbortSignal,
  ) => Promise<CapabilityStatus>;
  revokeSubscriptionToken: (
    tokenID: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionToken>;
  listCatalogAssets: (
    filter?: CatalogAssetFilter,
    signal?: AbortSignal,
  ) => Promise<CatalogAssetList>;
  listCoreArtifacts: (
    filter?: CoreArtifactFilter,
    signal?: AbortSignal,
  ) => Promise<CoreArtifactPage>;
  previewManualReattach: (
    artifactID: string,
    signal?: AbortSignal,
  ) => Promise<ManualReattachPreview>;
  listSubscriptionTokens: (filter?: SubscriptionListFilter, signal?: AbortSignal) => Promise<SubscriptionTokenPage>;
  listSubscriptionSources: (filter?: SubscriptionListFilter, signal?: AbortSignal) => Promise<SubscriptionSourcePage>;
  listTrafficPeriods: (
    filter?: TrafficPeriodFilter,
    signal?: AbortSignal,
  ) => Promise<TrafficPeriodPage>;
  listSubscriptionChannels: (filter?: SubscriptionListFilter, signal?: AbortSignal) => Promise<SubscriptionChannelPage>;
  deleteSubscriptionSource: (
    sourceID: string,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<void>;
  deleteSubscriptionChannel: (
    channelID: string,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<void>;
  createSubscriptionSource: (
    input: SubscriptionSourceWrite,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSource>;
  createSubscriptionChannel: (
    input: SubscriptionChannelWrite,
    signal?: AbortSignal,
  ) => Promise<SubscriptionChannel>;
  replaceCanonical: (
    documentJSON: string,
    baseRevision: string,
    signal?: AbortSignal,
  ) => Promise<CanonicalSave>;
  createSubscriptionToken: (
    input: { expiresAt?: string },
    signal?: AbortSignal,
  ) => Promise<CreatedSubscriptionToken>;
  patchCanonical: (
    changes: CanonicalChange[],
    baseRevision: string,
    signal?: AbortSignal,
  ) => Promise<CanonicalSave>;
  rotateSubscriptionToken: (
    tokenID: string,
    expiresAt?: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionTokenRotation>;
  activateStartupArtifact: (
    artifactID: string,
    monitoringTier: MonitoringTier,
    signal?: AbortSignal,
  ) => Promise<ActivationQueued>;
  updateSubscriptionSourceSnapshot: (
    sourceID: string,
    snapshot: unknown,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSource>;
  updateSubscriptionChannel: (
    channelID: string,
    input: SubscriptionChannelWrite,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionChannel>;
  saveEntity: (
    collection: EntityCollection,
    entity: Entity,
    options: { baseRevision: string; existingID?: string },
    signal?: AbortSignal,
  ) => Promise<CanonicalSave>;
  renderStructured: (
    input: {
      coreVersion: string;
      coreArtifactID?: string;
      allowCompatible: boolean;
    },
    signal?: AbortSignal,
  ) => Promise<StructuredRender>;
  updateSubscriptionSource: (
    sourceID: string,
    input: Omit<SubscriptionSourceWrite, 'latest_snapshot'>,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSource>;
  listManualArtifacts: (
    filter: { coreVersion?: string; coreArtifactID?: string; beforeTime?: string; beforeID?: string; limit?: number },
    signal?: AbortSignal,
  ) => Promise<ManualArtifactList>;
  applyManualReattach: (
    artifactID: string,
    input: {
      evidence: ReattachEvidence;
      decisions: Record<string, 'current' | 'manual'>;
    },
    signal?: AbortSignal,
  ) => Promise<ManualReattachSave>;
  replaceManualArtifact: (
    input: {
      baseRevision: string;
      coreVersion?: string;
      coreArtifactID: string;
      raw: string;
      allowCompatible?: boolean;
    },
    signal?: AbortSignal,
  ) => Promise<ManualSave>;
  previewManualReplacement: (
    input: {
      baseRevision: string;
      coreVersion?: string;
      coreArtifactID: string;
      raw: string;
      allowCompatible?: boolean;
    },
    signal?: AbortSignal,
  ) => Promise<ManualReplacePreview>;
  listStartupArtifacts: (
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
  ) => Promise<StartupArtifactPage>;
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
