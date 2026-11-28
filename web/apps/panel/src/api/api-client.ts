import type { FilesystemPage, FilesystemQuery, FilesystemResolveInput, FilesystemSelection } from './contracts/filesystem';
import type {
  CatalogAssetFilter,
  CatalogAssetList,
  ConfigurationCompile,
  ConfigurationSchemaContract,
  CoreArtifact,
  CoreArtifactFilter,
  CoreArtifactPage,
  CoreImportUpload,
  RuntimeStatus,
} from './contracts/core';
import type {
  CoreLogChunk,
  CoreLogFile,
  DashboardStreamSnapshot,
  FleetRuntimeHistoryFilter,
  FleetRuntimeHistoryPage,
  LogClearResult,
  LogScope,
  LogUsage,
  MetricsSnapshot,
  NodeSettings,
  PanelLogFilter,
  PanelLogPage,
  RecentMetrics,
} from './contracts/observability';
import type {
  CreatedSubscriptionToken,
  SubscriptionChannel,
  SubscriptionChannelPage,
  SubscriptionChannelWrite,
  SubscriptionDraftPreview,
  SubscriptionListFilter,
  SubscriptionNodeDetail,
  SubscriptionNodeList,
  SubscriptionNodeUpdate,
  SubscriptionPreview,
  SubscriptionRuleSet,
  SubscriptionRuleSetCreate,
  SubscriptionRuleSetList,
  SubscriptionRuleSetUpdate,
  SubscriptionSource,
  SubscriptionSourceCreate,
  SubscriptionSourcePage,
  SubscriptionSourcePreview,
  SubscriptionSourcePreviewRequest,
  SubscriptionSourceUpdate,
  SubscriptionToken,
  SubscriptionTokenPage,
  SubscriptionTokenRotation,
} from './contracts/subscription';
import type {
  CatalogRefresh,
  ConfigurationFile,
  ConfigurationFileWrite,
  FleetCommand,
  FleetCommandResult,
  FleetCommandStarted,
  FleetCommandSummary,
  Node as FleetNode,
  FleetStatus,
  FleetStreamFrame,
  LoginInputWritable as LoginInput,
  NodeEnrollment,
  NodeUpdate,
  PanelBackupCreate,
  PanelBackupInfo,
  PanelBackupList,
  PanelRestoreApply,
  PanelRestorePlan,
  PanelRestorePlanRequest,
  PanelRestoreRequested,
  PanelRestoreStage,
  PanelRestoreStaged,
  PanelSettingsSaveResult,
  PanelSettingsView,
  PanelSettingsWriteWritable as PanelSettingsWrite,
  Session,
  SystemRestart,
  SystemStatus,
  SystemUpdate,
} from './generated';

export type {
  AppearanceSettings,
  CatalogRefresh,
  ConfigurationFile,
  ConfigurationFileWrite,
  FleetCommand,
  FleetCommandResult,
  FleetCommandStarted,
  FleetCommandSummary,
  Node as FleetNode,
  FleetStatus,
  FleetStreamFrame,
  DynamicObject as JsonObject,
  LoginInputWritable as LoginInput,
  NodeEnrollment,
  NodeUpdate,
  PanelBackupCategory,
  PanelBackupCounts,
  PanelBackupCreate,
  PanelBackupInfo,
  PanelBackupKind,
  PanelBackupList,
  PanelPreferences,
  PanelRestoreApply,
  PanelRestorePlan,
  PanelRestorePlanRequest,
  PanelRestoreRequested,
  PanelRestoreStage,
  PanelRestoreStaged,
  PanelServiceSettings,
  PanelSettingsSaveResult,
  PanelSettingsView,
  PanelSettingsWriteWritable as PanelSettingsWrite,
  Session,
  SystemRestart,
  SystemStatus,
  SystemUpdate,
  UpdateAttempt,
} from './generated';

export type { FilesystemEntry, FilesystemMode, FilesystemPage, FilesystemQuery, FilesystemResolveInput, FilesystemSelection } from './contracts/filesystem';
export * from './contracts/observability';
export * from './contracts/subscription';
export * from './contracts/canonical';

export * from './contracts/core';

/** One event of a batch command's stream: the command's ID, then each node's result, then the summary that ends it. */
export type FleetCommandEvent
  = | { command: FleetCommandStarted }
    | { result: FleetCommandResult }
    | { done: FleetCommandSummary };

export interface ApiClient {
  /** The node whose resources this client addresses; null for the panel-wide client. */
  readonly nodeID: string | null;
  /** Returns a client that addresses the resources of nodeID. */
  forNode: (nodeID: string) => ApiClient;
  logout: (signal?: AbortSignal) => Promise<void>;
  /** Where the browser downloads a backup with the session's cookie. */
  panelBackupDownloadUrl: (name: string) => string;
  /** Asks this client's node to update itself to the panel's release. */
  updateNode: (signal?: AbortSignal) => Promise<NodeUpdate>;
  getSession: (signal?: AbortSignal) => Promise<Session | null>;
  stopRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  /** Every node with its metrics, for the fleet overview. */
  getFleetStatus: (signal?: AbortSignal) => Promise<FleetStatus>;
  getMetrics: (signal?: AbortSignal) => Promise<MetricsSnapshot>;
  /** Asks the panel to restart; it answers with the start of the process that stops. */
  restartPanel: (signal?: AbortSignal) => Promise<SystemRestart>;
  startRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;

  getNodeSettings: (signal?: AbortSignal) => Promise<NodeSettings>;
  /** The panel's build and platform, and when its process started. */
  getSystemStatus: (signal?: AbortSignal) => Promise<SystemStatus>;
  /** The panel's version, the newest release it knows and the state of its update. */
  getSystemUpdate: (signal?: AbortSignal) => Promise<SystemUpdate>;
  restartRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  subscribeSessionInvalidated: (listener: () => void) => () => void;
  /** Asks GitHub for the newest stable release now. */
  checkSystemUpdate: (signal?: AbortSignal) => Promise<SystemUpdate>;
  /** The node's stored samples of the last two minutes, oldest first, which the live chart opens with. */
  getRecentMetrics: (signal?: AbortSignal) => Promise<RecentMetrics>;
  getRuntimeStatus: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  /** Starts updating the panel in the background; the panel then restarts. */
  startSystemUpdate: (signal?: AbortSignal) => Promise<SystemUpdate>;
  clearCoreLog: (file: string, signal?: AbortSignal) => Promise<void>;
  /** Removes the node and its key and ends its session. */
  deleteNode: (nodeID: string, signal?: AbortSignal) => Promise<void>;
  /** The backups in the panel's data directory, newest first. */
  listPanelBackups: (signal?: AbortSignal) => Promise<PanelBackupList>;
  login: (input: LoginInput, signal?: AbortSignal) => Promise<Session>;
  getPanelSettings: (signal?: AbortSignal) => Promise<PanelSettingsView>;

  deleteCoreLogFile: (file: string, signal?: AbortSignal) => Promise<void>;
  deletePanelBackup: (name: string, signal?: AbortSignal) => Promise<void>;
  discardPanelRestore: (id: string, signal?: AbortSignal) => Promise<void>;
  /** The entries in scope and the size of the whole log's files on disk. */
  getLogUsage: (scope: LogScope, signal?: AbortSignal) => Promise<LogUsage>;
  listFleetNodes: (signal?: AbortSignal) => Promise<{ items: FleetNode[] }>;
  getConfigurationFile: (signal?: AbortSignal) => Promise<ConfigurationFile>;
  /** Issues a new node's one-time enrollment code; the node exists once a host enrolls with it. */
  createNode: (name: string, signal?: AbortSignal) => Promise<NodeEnrollment>;
  /**
   * Discards the unused code issued for a node ID, a new node's or an
   * enrolled one's; keepalive lets the request outlive the page.
   */
  cancelNodeEnrollment: (nodeID: string, keepalive?: boolean) => Promise<void>;
  /** Every node, then the nodes that changed, from one snapshot the panel shares among its viewers. */
  streamFleetStatus: (signal?: AbortSignal) => AsyncIterable<FleetStreamFrame>;
  installCore: (assetID: number, signal?: AbortSignal) => Promise<CoreArtifact>;
  listCoreLogFiles: (signal?: AbortSignal) => Promise<{ items: CoreLogFile[] }>;
  /** The subscription node pool in its saved order, with the fleet nodes that publish nothing. */
  listSubscriptionNodes: (signal?: AbortSignal) => Promise<SubscriptionNodeList>;
  removeCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<void>;
  enableCore: (artifactID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  deleteSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<void>;
  disableCore: (artifactID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  /** Deletes the log entries in scope and shrinks the log's files; runtime states stay. */
  clearPanelLogs: (scope: LogScope, signal?: AbortSignal) => Promise<LogClearResult>;
  refreshCatalog: (force?: boolean, signal?: AbortSignal) => Promise<CatalogRefresh>;

  renameNode: (nodeID: string, name: string, signal?: AbortSignal) => Promise<FleetNode>;
  saveNodeSettings: (input: NodeSettings, signal?: AbortSignal) => Promise<NodeSettings>;
  /** Issues a new one-time enrollment code; the node's current key stays valid until it is redeemed. */
  createNodeEnrollment: (nodeID: string, signal?: AbortSignal) => Promise<NodeEnrollment>;
  listPanelLogs: (filter?: PanelLogFilter, signal?: AbortSignal) => Promise<PanelLogPage>;
  getSubscriptionNode: (id: string, signal?: AbortSignal) => Promise<SubscriptionNodeDetail>;
  streamDashboard: (
    signal?: AbortSignal,
  ) => AsyncIterable<DashboardStreamSnapshot>;
  getSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<SubscriptionToken>;
  importCoreArchive: (input: CoreImportUpload, signal?: AbortSignal) => Promise<CoreArtifact>;
  rollbackRuntime: (activationBundleID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  getSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<SubscriptionSource>;
  createPanelBackup: (input: PanelBackupCreate, signal?: AbortSignal) => Promise<PanelBackupInfo>;
  getSubscriptionChannel: (channelID: string, signal?: AbortSignal) => Promise<SubscriptionChannel>;
  getSubscriptionTokenSecret: (tokenID: string, signal?: AbortSignal) => Promise<{ token: string }>;
  listFilesystemEntries: (query?: FilesystemQuery, signal?: AbortSignal) => Promise<FilesystemPage>;
  /** Fetches a URL source now; a failure keeps its content and is recorded in last_error. */
  refreshSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<SubscriptionSource>;
  /** Runs one action on several nodes: each node's result as it finishes, then the summary. */
  runFleetCommand: (command: FleetCommand, signal?: AbortSignal) => AsyncIterable<FleetCommandEvent>;
  /** Deletes a rule set and the exit rules that use it. */
  deleteSubscriptionRuleSet: (ruleSetID: string, updatedAt: string, signal?: AbortSignal) => Promise<void>;
  resolveFilesystemPath: (input: FilesystemResolveInput, signal?: AbortSignal) => Promise<FilesystemSelection>;
  /** Saves the pool order; ids must be every current pool ID once, or the pool changed and it answers 409. */
  setSubscriptionNodeOrder: (
    ids: string[],
    signal?: AbortSignal,
  ) => Promise<SubscriptionNodeList>;
  /** Verifies a stored backup and stages it to restore. */
  stagePanelBackup: (name: string, input: PanelRestoreStage, signal?: AbortSignal) => Promise<PanelRestoreStaged>;
  /** Follows a batch command again after its stream broke: its ID, its results after the first `after`, its summary. */
  followFleetCommand: (commandID: string, after: number, signal?: AbortSignal) => AsyncIterable<FleetCommandEvent>;
  listCatalogAssets: (
    filter?: CatalogAssetFilter,
    signal?: AbortSignal,
  ) => Promise<CatalogAssetList>;
  listCoreArtifacts: (
    filter?: CoreArtifactFilter,
    signal?: AbortSignal,
  ) => Promise<CoreArtifactPage>;
  readCoreLog: (file: string, offset?: number, generation?: string, signal?: AbortSignal) => Promise<CoreLogChunk>;
  applyPanelRestore: (id: string, input: PanelRestoreApply, signal?: AbortSignal) => Promise<PanelRestoreRequested>;
  /** Backs up the current state and applies the staged restore as the panel restarts. */
  /** Counts what restoring categories of a staged restore changes beyond them; changes nothing. */
  planPanelRestore: (id: string, input: PanelRestorePlanRequest, signal?: AbortSignal) => Promise<PanelRestorePlan>;
  createSubscriptionRuleSet: (input: SubscriptionRuleSetCreate, signal?: AbortSignal) => Promise<SubscriptionRuleSet>;
  savePanelSettings: (
    input: PanelSettingsWrite,
    signal?: AbortSignal,
  ) => Promise<PanelSettingsSaveResult>;

  streamMetrics: (
    signal?: AbortSignal,
  ) => AsyncIterable<{ metrics: MetricsSnapshot; runtime: RuntimeStatus }>;
  deleteSubscriptionSource: (
    sourceID: string,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<void>;
  getConfigurationSchema: (
    artifactID: string,
    signal?: AbortSignal,
  ) => Promise<ConfigurationSchemaContract>;

  saveConfigurationFile: (
    input: ConfigurationFileWrite,
    signal?: AbortSignal,
  ) => Promise<ConfigurationFile>;
  deleteSubscriptionChannel: (
    channelID: string,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<void>;
  /** Every rule set, or one client's, each client's in their saved order. */
  listSubscriptionRuleSets: (client?: SubscriptionRuleSet['client'], signal?: AbortSignal) => Promise<SubscriptionRuleSetList>;
  compileConfiguration: (
    input: { coreArtifactID: string },
    signal?: AbortSignal,
  ) => Promise<ConfigurationCompile>;

  createSubscriptionSource: (
    input: SubscriptionSourceCreate,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSource>;
  createSubscriptionChannel: (
    input: SubscriptionChannelWrite,
    signal?: AbortSignal,
  ) => Promise<SubscriptionChannel>;
  listSubscriptionSources: (
    filter?: SubscriptionListFilter,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSourcePage>;
  listSubscriptionChannels: (
    filter?: SubscriptionListFilter,
    signal?: AbortSignal,
  ) => Promise<SubscriptionChannelPage>;
  /** Every node's runtime transitions, newest first. */
  listFleetRuntimeHistory: (
    filter?: FleetRuntimeHistoryFilter,
    signal?: AbortSignal,
  ) => Promise<FleetRuntimeHistoryPage>;
  setSubscriptionTokenEnabled: (
    tokenID: string,
    enabled: boolean,
    signal?: AbortSignal,
  ) => Promise<SubscriptionToken>;
  rotateSubscriptionToken: (
    tokenID: string,
    expiresAt?: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionTokenRotation>;
  /** Saves one client's rule set order; ids must be every one of its sets once, or they changed and it answers 409. */
  setSubscriptionRuleSetOrder: (
    client: SubscriptionRuleSet['client'],
    ids: string[],
    signal?: AbortSignal,
  ) => Promise<void>;
  /** Parses content without saving: a stored source's, a URL's or the request's, filtered by the given rules. */
  previewSubscriptionSource: (
    input: SubscriptionSourcePreviewRequest,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSourcePreview>;
  streamCoreLog: (
    file: string,
    offset?: number,
    generation?: string,
    signal?: AbortSignal,
  ) => AsyncIterable<CoreLogChunk>;
  listSubscriptionTokens: (
    filter?: SubscriptionListFilter & { offset?: number },
    signal?: AbortSignal,
  ) => Promise<SubscriptionTokenPage>;
  /** Changes a pool node's display name; updatedAt is the version it was read at. */
  updateSubscriptionNode: (
    id: string,
    input: SubscriptionNodeUpdate,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionNodeDetail>;
  updateSubscriptionSource: (
    sourceID: string,
    input: SubscriptionSourceUpdate,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSource>;
  previewSubscriptionChannel: (
    channelID: string,
    userID: string,
    signal?: AbortSignal,
    draft?: SubscriptionDraftPreview,
  ) => Promise<SubscriptionPreview>;
  updateSubscriptionChannel: (
    channelID: string,
    input: SubscriptionChannelWrite,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionChannel>;
  /** Replaces a rule set's name and rules at the version it was read. */
  updateSubscriptionRuleSet: (
    ruleSetID: string,
    input: SubscriptionRuleSetUpdate,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionRuleSet>;
  /** Uploads a backup, reporting the bytes sent, then verifies it and stages it to restore. */
  uploadPanelRestore: (
    file: File,
    passphrase: string,
    onProgress: (sent: number, total: number) => void,
    signal?: AbortSignal,
  ) => Promise<PanelRestoreStaged>;
  createSubscriptionToken: (
    input: { userID?: string; label: string; expiresAt?: string; downloadLimit?: number },
    signal?: AbortSignal,
  ) => Promise<CreatedSubscriptionToken>;
}

export class ApiRequestError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields?: Record<string, string>;

  constructor(
    message: string,
    options: { status: number; code: string; fields?: Record<string, string> },
  ) {
    super(message);
    this.name = 'ApiRequestError';
    this.status = options.status;
    this.code = options.code;
    this.fields = options.fields;
  }
}
