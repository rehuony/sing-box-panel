import type {
  ActivationSummary,
  CoreArtifact,
  CreatedAtCursor,
  RuntimeTransition,
  StartupArtifactSummary,
} from '../generated';

export type {
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
  RuntimeResponse,
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
  exactVersion: string;
  sourceDescription: string;
  architecture: 'amd64' | 'arm64';
  variant: CoreArtifact['variant'];
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
