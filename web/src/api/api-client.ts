import type { JsonObject } from './contracts/common';
import type { Task, TaskFilter, TaskPage } from './contracts/tasks';
import type { DashboardContext, Session, SystemStatus } from './contracts/session';
import type { PanelSettingsView, PanelSettingsWrite } from './contracts/panel-settings';
import type { ConfigurationFile, ConfigurationFileWrite } from './contracts/configuration-file';
import type {
  CanonicalChange,
  CanonicalRevisionDiff,
  CanonicalRevisionListFilter,
  CanonicalRevisionPage,
  CanonicalSave,
  CanonicalSnapshot,
} from './contracts/canonical';
import type {
  CoreLogChunk,
  CoreLogFile,
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
  ActivationQueued,
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
  RuntimeStatus,
  StartupArtifactPage,
  StartupArtifactState,
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

export * from './contracts/configuration-file';
export * from './contracts/panel-settings';
export * from './contracts/observability';
export * from './contracts/subscription';
export * from './contracts/canonical';
export * from './contracts/session';
export * from './contracts/common';
export * from './contracts/tasks';
export * from './contracts/core';

export interface ApiClient {
  logout: (signal?: AbortSignal) => Promise<void>;
  stopRuntime: (signal?: AbortSignal) => Promise<Task>;
  startRuntime: (signal?: AbortSignal) => Promise<Task>;
  restartRuntime: (signal?: AbortSignal) => Promise<Task>;
  getSession: (signal?: AbortSignal) => Promise<Session | null>;

  getMetrics: (signal?: AbortSignal) => Promise<MetricsSnapshot>;
  getSystemStatus: (signal?: AbortSignal) => Promise<SystemStatus>;
  getTask: (taskID: string, signal?: AbortSignal) => Promise<Task>;
  login: (token: string, signal?: AbortSignal) => Promise<Session>;
  subscribeSessionInvalidated: (listener: () => void) => () => void;
  getCanonical: (signal?: AbortSignal) => Promise<CanonicalSnapshot>;
  getRuntimeStatus: (signal?: AbortSignal) => Promise<RuntimeStatus>;
  retryTask: (taskID: string, signal?: AbortSignal) => Promise<Task>;
  cancelTask: (taskID: string, signal?: AbortSignal) => Promise<Task>;
  getLog: (entryID: string, signal?: AbortSignal) => Promise<LogEntry>;

  getTrafficStatus: (signal?: AbortSignal) => Promise<MetricsSnapshot>;
  installCore: (assetID: number, signal?: AbortSignal) => Promise<Task>;
  getPanelSettings: (signal?: AbortSignal) => Promise<PanelSettingsView>;
  enableCore: (artifactID: string, signal?: AbortSignal) => Promise<Task>;
  getDashboardContext: (signal?: AbortSignal) => Promise<DashboardContext>;
  listLogs: (filter?: LogFilter, signal?: AbortSignal) => Promise<LogPage>;
  refreshCatalog: (force?: boolean, signal?: AbortSignal) => Promise<Task>;
  getConfigurationFile: (signal?: AbortSignal) => Promise<ConfigurationFile>;
  listTasks: (filter?: TaskFilter, signal?: AbortSignal) => Promise<TaskPage>;
  listCoreLogFiles: (signal?: AbortSignal) => Promise<{ items: CoreLogFile[] }>;
  newInboundDefaults: (type: string, signal?: AbortSignal) => Promise<JsonObject>;
  removeCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<void>;

  checkStartupArtifact: (artifactID: string, signal?: AbortSignal) => Promise<Task>;
  deleteSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<void>;
  importCoreArchive: (input: CoreImportUpload, signal?: AbortSignal) => Promise<Task>;
  getCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<CoreArtifact>;
  getRevision: (reference: string, signal?: AbortSignal) => Promise<CanonicalSnapshot>;
  getTrafficPeriod: (periodID: string, signal?: AbortSignal) => Promise<TrafficPeriod>;
  refreshSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<Task>;
  rollbackRuntime: (activationBundleID: string, signal?: AbortSignal) => Promise<Task>;
  getSubscriptionNodeCatalog: (signal?: AbortSignal) => Promise<SubscriptionNodeCatalog>;
  listPanelLogs: (filter?: PanelLogFilter, signal?: AbortSignal) => Promise<PanelLogPage>;
  revokeCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<CoreArtifact>;
  getSubscriptionUser: (userID: string, signal?: AbortSignal) => Promise<SubscriptionUser>;
  clearLogs: (filter?: LogClearFilter, signal?: AbortSignal) => Promise<{ deleted: number }>;
  getSubscriptionNode: (id: string, signal?: AbortSignal) => Promise<SubscriptionNodeDetail>;
  getSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<SubscriptionToken>;
  quarantineCoreArtifact: (artifactID: string, signal?: AbortSignal) => Promise<CoreArtifact>;
  readCoreLog: (file: string, offset?: number, signal?: AbortSignal) => Promise<CoreLogChunk>;
  deleteLog: (entryID: string, signal?: AbortSignal) => Promise<{ id: string; deleted: true }>;
  deleteSubscriptionNode: (id: string, revision: number, signal?: AbortSignal) => Promise<void>;
  streamLogs: (filter?: LogStreamFilter, signal?: AbortSignal) => AsyncIterable<LogStreamEvent>;
  getSubscriptionSource: (sourceID: string, signal?: AbortSignal) => Promise<SubscriptionSource>;
  revokeSubscriptionToken: (tokenID: string, signal?: AbortSignal) => Promise<SubscriptionToken>;
  diffRevisions: (from: string, to: string, signal?: AbortSignal) => Promise<CanonicalRevisionDiff>;
  getSubscriptionChannel: (channelID: string, signal?: AbortSignal) => Promise<SubscriptionChannel>;
  parseSubscriptionNode: (text: string, signal?: AbortSignal) => Promise<{ outbound_json: string }>;

  getMetricsHistory: (
    filter: MetricsHistoryFilter,
    signal?: AbortSignal,
  ) => Promise<MetricsHistory>;
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
  streamCoreLog: (
    file: string,
    offset?: number,
    signal?: AbortSignal,
  ) => AsyncIterable<CoreLogChunk>;
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
  listRevisions: (
    filter?: CanonicalRevisionListFilter,
    signal?: AbortSignal,
  ) => Promise<CanonicalRevisionPage>;
  restoreRevision: (
    reference: string,
    baseRevision: string,
    signal?: AbortSignal,
  ) => Promise<CanonicalSave>;
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
  listSubscriptionTokens: (
    filter?: SubscriptionListFilter,
    signal?: AbortSignal,
  ) => Promise<SubscriptionTokenPage>;
  createSubscriptionChannel: (
    input: SubscriptionChannelWrite,
    signal?: AbortSignal,
  ) => Promise<SubscriptionChannel>;
  replaceCanonical: (
    documentJSON: string,
    baseRevision: string,
    signal?: AbortSignal,
  ) => Promise<CanonicalSave>;
  listSubscriptionSources: (
    filter?: SubscriptionListFilter,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSourcePage>;
  listSubscriptionChannels: (
    filter?: SubscriptionListFilter,
    signal?: AbortSignal,
  ) => Promise<SubscriptionChannelPage>;
  patchCanonical: (
    changes: CanonicalChange[],
    baseRevision: string,
    signal?: AbortSignal,
  ) => Promise<CanonicalSave>;
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
  getSubscriptionSourceVersion: (
    sourceID: string,
    versionID: string,
    signal?: AbortSignal,
  ) => Promise<SubscriptionSourceVersion>;
  activateStartupArtifact: (
    artifactID: string,
    monitoringTier: MonitoringTier,
    signal?: AbortSignal,
  ) => Promise<ActivationQueued>;
  updateSubscriptionNode: (
    id: string,
    outboundJSON: string,
    revision: number,
    signal?: AbortSignal,
  ) => Promise<SubscriptionNodeDetail>;
  previewConfiguration: (
    input: { coreArtifactID: string; canonicalRevisionID?: string },
    signal?: AbortSignal,
  ) => Promise<ConfigurationPreview>;
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
