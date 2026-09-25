import type { FilesystemPage, FilesystemQuery, FilesystemResolveInput, FilesystemSelection } from './contracts/filesystem';
import type {
  CatalogRefresh,
  ConfigurationFile,
  ConfigurationFileWrite,
  DashboardContext,
  PanelBackup,
  PanelRestoreRequest,
  PanelRestoreResult,
  PanelSettingsView,
  PanelSettingsWrite,
  Session,
  SubscriptionSourceRefreshResult,
  SystemStatus,
} from './generated';
import type {
  CoreLogChunk,
  CoreLogFile,
  DashboardStreamSnapshot,
  LogClearFilter,
  LogEntry,
  LogFilter,
  LogPage,
  LogStreamEvent,
  LogStreamFilter,
  MetricsHistory,
  MetricsHistoryFilter,
  MetricsSnapshot,
  PanelLogFilter,
  PanelLogPage,
  TrafficPeriod,
  TrafficPeriodFilter,
  TrafficPeriodPage,
} from './contracts/observability';
import type {
  CatalogAssetFilter,
  CatalogAssetList,
  ConfigurationCompile,
  ConfigurationPreview,
  ConfigurationSchemaContract,
  ConfigurationSupport,
  CoreArtifact,
  CoreArtifactFilter,
  CoreArtifactPage,
  CoreImportUpload,
  MonitoringTier,
  RuntimeHistoryFilter,
  RuntimeHistoryPage,
  RuntimeResponse,
  RuntimeStatus,
  StartupArtifactPage,
  StartupArtifactState,
  StartupArtifactSummary,
} from './contracts/core';
import type {
  CreatedSubscriptionToken,
  SubscriptionChannel,
  SubscriptionChannelPage,
  SubscriptionChannelWrite,
  SubscriptionDraftPreview,
  SubscriptionListFilter,
  SubscriptionNodeCatalog,
  SubscriptionNodeDetail,
  SubscriptionNodeSummary,
  SubscriptionPreview,
  SubscriptionSource,
  SubscriptionSourceFormat,
  SubscriptionSourcePage,
  SubscriptionSourceVersion,
  SubscriptionSourceVersionPage,
  SubscriptionSourceVersionSave,
  SubscriptionSourceWrite,
  SubscriptionToken,
  SubscriptionTokenPage,
  SubscriptionTokenRotation,
  SubscriptionUser,
  SubscriptionUserGrants,
  SubscriptionUserPage,
  SubscriptionUserWrite,
} from './contracts/subscription';

export type {
  AppearanceSettings,
  CatalogRefresh,
  ConfigurationFile,
  ConfigurationFileWrite,
  DashboardContext,
  DynamicObject as JsonObject,
  PanelBackup,
  PanelPreferences,
  PanelRestoreRequest,
  PanelRestoreResult,
  PanelServiceSettings,
  PanelSettingsView,
  PanelSettingsWrite,
  Session,
  SubscriptionSourceRefreshResult,
  SystemStatus,
} from './generated';

export type { FilesystemEntry, FilesystemMode, FilesystemPage, FilesystemQuery, FilesystemResolveInput, FilesystemSelection } from './contracts/filesystem';
export * from './contracts/observability';
export * from './contracts/subscription';
export * from './contracts/canonical';

export * from './contracts/core';

export type DashboardConfiguration = DashboardContext['configuration'];

export interface ApiClient {
  invalidateReadCache: () => void;
  logout: (signal?: AbortSignal) => Promise<void>;
  /** Whether channel drafts can be validated by the native renderer before saving. */
  readonly supportsNativeChannelValidation: boolean;
  getSession: (signal?: AbortSignal) => Promise<Session | null>;
  stopRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  getMetrics: (signal?: AbortSignal) => Promise<MetricsSnapshot>;
  startRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  getSystemStatus: (signal?: AbortSignal) => Promise<SystemStatus>;
  login: (token: string, signal?: AbortSignal) => Promise<Session>;

  restartRuntime: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  exportPanelBackup: (signal?: AbortSignal) => Promise<PanelBackup>;
  subscribeSessionInvalidated: (listener: () => void) => () => void;
  getRuntimeStatus: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  clearCoreLog: (file: string, signal?: AbortSignal) => Promise<void>;
  getLog: (entryID: string, signal?: AbortSignal) => Promise<LogEntry>;

  getTrafficStatus: (signal?: AbortSignal) => Promise<MetricsSnapshot>;
  getPanelSettings: (signal?: AbortSignal) => Promise<PanelSettingsView>;
  deleteCoreLogFile: (file: string, signal?: AbortSignal) => Promise<void>;
  getDashboardContext: (signal?: AbortSignal) => Promise<DashboardContext>;
  listLogs: (filter?: LogFilter, signal?: AbortSignal) => Promise<LogPage>;
  getConfigurationFile: (signal?: AbortSignal) => Promise<ConfigurationFile>;
  installCore: (assetID: number, signal?: AbortSignal) => Promise<CoreArtifact>;
  listCoreLogFiles: (signal?: AbortSignal) => Promise<{ items: CoreLogFile[] }>;
  removeCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<void>;
  enableCore: (artifactID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  deleteSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<void>;
  disableCore: (artifactID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  refreshCatalog: (force?: boolean, signal?: AbortSignal) => Promise<CatalogRefresh>;
  getCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<CoreArtifact>;
  getTrafficPeriod: (periodID: string, signal?: AbortSignal) => Promise<TrafficPeriod>;

  getSubscriptionNodeCatalog: (signal?: AbortSignal) => Promise<SubscriptionNodeCatalog>;
  listPanelLogs: (filter?: PanelLogFilter, signal?: AbortSignal) => Promise<PanelLogPage>;
  getSubscriptionUser: (userID: string, signal?: AbortSignal) => Promise<SubscriptionUser>;
  clearLogs: (filter?: LogClearFilter, signal?: AbortSignal) => Promise<{ deleted: number }>;
  getSubscriptionNode: (id: string, signal?: AbortSignal) => Promise<SubscriptionNodeDetail>;
  streamDashboard: (
    signal?: AbortSignal,
  ) => AsyncIterable<DashboardStreamSnapshot>;
  getSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<SubscriptionToken>;
  importCoreArchive: (input: CoreImportUpload, signal?: AbortSignal) => Promise<CoreArtifact>;
  deleteLog: (entryID: string, signal?: AbortSignal) => Promise<{ id: string; deleted: true }>;
  deleteSubscriptionNode: (id: string, revision: number, signal?: AbortSignal) => Promise<void>;
  rollbackRuntime: (activationBundleID: string, signal?: AbortSignal) => Promise<RuntimeStatus>;
  streamLogs: (filter?: LogStreamFilter, signal?: AbortSignal) => AsyncIterable<LogStreamEvent>;
  getSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<SubscriptionSource>;
  revokeSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<SubscriptionToken>;
  getSubscriptionChannel: (channelID: string, signal?: AbortSignal) => Promise<SubscriptionChannel>;
  getSubscriptionTokenSecret: (tokenID: string, signal?: AbortSignal) => Promise<{ token: string }>;
  listFilesystemEntries: (query?: FilesystemQuery, signal?: AbortSignal) => Promise<FilesystemPage>;
  parseSubscriptionNode: (text: string, signal?: AbortSignal) => Promise<{ outbound_json: string }>;
  checkStartupArtifact: (artifactID: string, signal?: AbortSignal) => Promise<StartupArtifactSummary>;
  restorePanelBackup: (input: PanelRestoreRequest, signal?: AbortSignal) => Promise<PanelRestoreResult>;
  resolveFilesystemPath: (input: FilesystemResolveInput, signal?: AbortSignal) => Promise<FilesystemSelection>;
  getMetricsHistory: (
    filter: MetricsHistoryFilter,
    signal?: AbortSignal,
  ) => Promise<MetricsHistory>;

  refreshSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<SubscriptionSourceRefreshResult>;
  savePanelSettings: (
    input: PanelSettingsWrite,
    signal?: AbortSignal,
  ) => Promise<PanelSettingsView>;
  listCatalogAssets: (
    filter?: CatalogAssetFilter,
    signal?: AbortSignal,
  ) => Promise<CatalogAssetList>;
  listCoreArtifacts: (
    filter?: CoreArtifactFilter,
    signal?: AbortSignal,
  ) => Promise<CoreArtifactPage>;
  readCoreLog: (file: string, offset?: number, generation?: string, signal?: AbortSignal) => Promise<CoreLogChunk>;

  getConfigurationSupport: (
    artifactID: string,
    signal?: AbortSignal,
  ) => Promise<ConfigurationSupport>;
  getSubscriptionUserGrants: (
    userID: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionUserGrants>;
  deleteSubscriptionUser: (
    userID: string,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<void>;
  listTrafficPeriods: (
    filter?: TrafficPeriodFilter,
    signal?: AbortSignal,
  ) => Promise<TrafficPeriodPage>;
  createSubscriptionNode: (
    outboundJSON: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionNodeDetail>;
  getRuntimeHistory: (
    filter?: RuntimeHistoryFilter,
    signal?: AbortSignal,
  ) => Promise<RuntimeHistoryPage>;
  streamMetrics: (
    signal?: AbortSignal,
  ) => AsyncIterable<{ metrics: MetricsSnapshot; runtime: RuntimeStatus }>;
  createSubscriptionUser: (
    input: SubscriptionUserWrite,
    signal?: AbortSignal,
  ) => Promise<SubscriptionUser>;
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
  createSubscriptionSource: (
    input: SubscriptionSourceWrite,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSource>;
  listSubscriptionUsers: (
    filter?: SubscriptionListFilter,
    signal?: AbortSignal,
  ) => Promise<SubscriptionUserPage>;
  compileConfiguration: (
    input: { coreArtifactID: string },
    signal?: AbortSignal,
  ) => Promise<ConfigurationCompile>;
  previewConfiguration: (
    input: { coreArtifactID: string },
    signal?: AbortSignal,
  ) => Promise<ConfigurationPreview>;
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
  activateStartupArtifact: (
    artifactID: string,
    monitoringTier: MonitoringTier,
    signal?: AbortSignal,
  ) => Promise<RuntimeResponse>;
  getSubscriptionSourceVersion: (
    sourceID: string,
    versionID: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSourceVersion>;
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
  updateSubscriptionUser: (
    userID: string,
    input: SubscriptionUserWrite,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionUser>;
  replaceSubscriptionUserGrants: (
    userID: string,
    grants: string[],
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionUserGrants>;
  listSubscriptionSourceVersions: (
    sourceID: string,
    filter?: SubscriptionListFilter,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSourceVersionPage>;
  restoreSubscriptionSourceVersion: (
    sourceID: string,
    versionID: string,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSource>;
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
  createSubscriptionSourceVersion: (
    sourceID: string,
    format: SubscriptionSourceFormat,
    rawBody: string,
    updatedAt: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSourceVersionSave>;
  listStartupArtifacts: (
    filter: {
      canonicalRevisionID?: string;
      coreVersion?: string;
      coreArtifactID?: string;
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
    options: { status: number; code: string; fields?: Record<string, string> },
  ) {
    super(message);
    this.name = 'ApiRequestError';
    this.status = options.status;
    this.code = options.code;
    this.fields = options.fields;
  }
}
