import type {
  ActivationSummary,
  CoreArtifact,
  CreatedAtCursor,
  RuntimeTransition,
  StartupArtifactSummary,
} from '../generated';

export type {
  ActivationQueued,
  ActivationSummary,
  CatalogAsset,
  CatalogAssetList,
  CompiledConfigurationArtifact,
  ConfigurationCompile,
  ConfigurationPreview,
  ConfigurationSchemaContract,
  ConfigurationSupport,
  CoreArtifact,
  CoreArtifactPage,
  RuntimeHistoryPage,
  RuntimeStatus,
  RuntimeTransition,
  RuntimeTransitionCursor,
  StartupArtifactPage,
  StartupArtifactSummary,
} from '../generated';

export type StartupArtifactState = StartupArtifactSummary['state'];
export type MonitoringTier = ActivationSummary['monitoring_tier'];
export type CoreArtifactCursor = CreatedAtCursor;
export type RuntimeTransitionState = RuntimeTransition['state'];

export interface RuntimeHistoryFilter {
  to?: string;
  from?: string;
  limit?: number;
  reason?: string;
  beforeID?: number;
  beforeTime?: string;
  activationBundleID?: string;
  state?: RuntimeTransitionState;
}

export interface CatalogAssetFilter {
  variant?: string;
  architecture?: string;
  exactVersion?: string;
  installable?: boolean;
}

export interface CoreImportUpload {
  archive: File;
  variant: string;
  exactVersion: string;
  sourceDescription: string;
  architecture: 'amd64' | 'arm64';
}

export interface CoreArtifactFilter {
  limit?: number;
  variant?: string;
  beforeID?: string;
  beforeTime?: string;
  architecture?: string;
  exactVersion?: string;
  sourceKind?: CoreArtifact['source_kind'];
}
