import type { FilesystemPage, FilesystemQuery, FilesystemResolveInput, FilesystemSelection } from './contracts/filesystem';
import type {
  CoreLogChunk,
  CoreLogFile,
  DashboardStreamSnapshot,
  MetricsSnapshot,
  NodeSettings,
  PanelLogFilter,
  PanelLogPage,
  TrafficStatus,
} from './contracts/observability';
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
  CatalogRefresh,
  ConfigurationFile,
  ConfigurationFileWrite,
  Node as FleetNode,
  LoginInputWritable as LoginInput,
  NodeEnrollment,
  PanelBackup,
  PanelRestoreRequest,
  PanelRestoreResult,
  PanelSettingsSaveResult,
  PanelSettingsView,
  PanelSettingsWriteWritable as PanelSettingsWrite,
  Session,
  SubscriptionSourceRefreshResult,
} from './generated';
import type {
  CreatedSubscriptionToken,
  SubscriptionChannel,
  SubscriptionChannelPage,
  SubscriptionChannelWrite,
  SubscriptionDraftPreview,
  SubscriptionListFilter,
  SubscriptionNodeCatalog,
  SubscriptionNodeDetail,
  SubscriptionNodeOrder,
  SubscriptionNodeSummary,
  SubscriptionPreview,
  SubscriptionSource,
  SubscriptionSourcePage,
  SubscriptionSourceWrite,
  SubscriptionToken,
  SubscriptionTokenPage,
  SubscriptionTokenRotation,
} from './contracts/subscription';

export type {
  AppearanceSettings,
  CatalogRefresh,
  ConfigurationFile,
  ConfigurationFileWrite,
  Node as FleetNode,
  DynamicObject as JsonObject,
  LoginInputWritable as LoginInput,
  NodeEnrollment,
  PanelBackup,
  PanelPreferences,
  PanelRestoreRequest,
  PanelRestoreResult,
  PanelServiceSettings,
  PanelSettingsSaveResult,
  PanelSettingsView,
  PanelSettingsWriteWritable as PanelSettingsWrite,
  Session,
  SubscriptionSourceRefreshResult,
  SystemStatus,
} from './generated';

export type { FilesystemEntry, FilesystemMode, FilesystemPage, FilesystemQuery, FilesystemResolveInput, FilesystemSelection } from './contracts/filesystem';
export * from './contracts/observability';
export * from './contracts/subscription';
export * from './contracts/canonical';

export * from './contracts/core';

export interface ApiClient {
  /** The node whose resources this client addresses; null for the panel-wide client. */
  readonly nodeID: string | null;
  /** Returns a client that addresses the resources of nodeID. */
  forNode: (nodeID: string) => ApiClient;
  logout: (signal?: AbortSignal) => Promise<void>;
  getSession: (signal?: AbortSignal) => Promise<Session | null>;
  stopRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  getMetrics: (signal?: AbortSignal) => Promise<MetricsSnapshot>;
  startRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  getNodeSettings: (signal?: AbortSignal) => Promise<NodeSettings>;
  restartRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;

  exportPanelBackup: (signal?: AbortSignal) => Promise<PanelBackup>;
  subscribeSessionInvalidated: (listener: () => void) => () => void;
  getRuntimeStatus: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  getTrafficStatus: (signal?: AbortSignal) => Promise<TrafficStatus>;
  clearCoreLog: (file: string, signal?: AbortSignal) => Promise<void>;
  /** Removes the node and its key and ends its session. */
  deleteNode: (nodeID: string, signal?: AbortSignal) => Promise<void>;

  login: (input: LoginInput, signal?: AbortSignal) => Promise<Session>;
  getPanelSettings: (signal?: AbortSignal) => Promise<PanelSettingsView>;
  deleteCoreLogFile: (file: string, signal?: AbortSignal) => Promise<void>;
  listFleetNodes: (signal?: AbortSignal) => Promise<{ items: FleetNode[] }>;
  getConfigurationFile: (signal?: AbortSignal) => Promise<ConfigurationFile>;
  /** Registers a node and returns its one-time enrollment code. */
  createNode: (name: string, signal?: AbortSignal) => Promise<NodeEnrollment>;
  installCore: (assetID: number, signal?: AbortSignal) => Promise<CoreArtifact>;
  listCoreLogFiles: (signal?: AbortSignal) => Promise<{ items: CoreLogFile[] }>;
  removeCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<void>;
  enableCore: (artifactID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  deleteSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<void>;
  disableCore: (artifactID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  refreshCatalog: (force?: boolean, signal?: AbortSignal) => Promise<CatalogRefresh>;
  getSubscriptionNodeCatalog: (signal?: AbortSignal) => Promise<SubscriptionNodeCatalog>;
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
  deleteSubscriptionNode: (id: string, revision: number, signal?: AbortSignal) => Promise<void>;
  rollbackRuntime: (activationBundleID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  getSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<SubscriptionSource>;
  getSubscriptionChannel: (channelID: string, signal?: AbortSignal) => Promise<SubscriptionChannel>;
  getSubscriptionTokenSecret: (tokenID: string, signal?: AbortSignal) => Promise<{ token: string }>;
  listFilesystemEntries: (query?: FilesystemQuery, signal?: AbortSignal) => Promise<FilesystemPage>;
  parseSubscriptionNode: (text: string, signal?: AbortSignal) => Promise<{ outbound_json: string }>;
  restorePanelBackup: (input: PanelRestoreRequest, signal?: AbortSignal) => Promise<PanelRestoreResult>;
  resolveFilesystemPath: (input: FilesystemResolveInput, signal?: AbortSignal) => Promise<FilesystemSelection>;

  refreshSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<SubscriptionSourceRefreshResult>;
  listCatalogAssets: (
    filter?: CatalogAssetFilter,
    signal?: AbortSignal,
  ) => Promise<CatalogAssetList>;
  listCoreArtifacts: (
    filter?: CoreArtifactFilter,
    signal?: AbortSignal,
  ) => Promise<CoreArtifactPage>;
  readCoreLog: (file: string, offset?: number, generation?: string, signal?: AbortSignal) => Promise<CoreLogChunk>;

  createSubscriptionNode: (
    outboundJSON: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionNodeDetail>;
  savePanelSettings: (
    input: PanelSettingsWrite,
    signal?: AbortSignal,
  ) => Promise<PanelSettingsSaveResult>;
  streamMetrics: (
    signal?: AbortSignal,
  ) => AsyncIterable<{ metrics: MetricsSnapshot; runtime: RuntimeStatus }>;
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
  createSubscriptionSource: (
    input: SubscriptionSourceWrite,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSource>;
  compileConfiguration: (
    input: { coreArtifactID: string },
    signal?: AbortSignal,
  ) => Promise<ConfigurationCompile>;
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
  updateSubscriptionNode: (
    id: string,
    outboundJSON: string,
    revision: number,
    signal?: AbortSignal,
  ) => Promise<SubscriptionNodeDetail>;
  setSubscriptionNodeVisibility: (
    id: string,
    hidden: boolean,
    revision: number,
    signal?: AbortSignal,
  ) => Promise<SubscriptionNodeSummary>;
  setSubscriptionNodeOrder: (
    collectionID: string,
    ids: string[],
    revision: number,
    signal?: AbortSignal,
  ) => Promise<SubscriptionNodeOrder>;
  updateSubscriptionSource: (
    sourceID: string,
    input: SubscriptionSourceWrite,
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
