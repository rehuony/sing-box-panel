import type {
  ActivationSummary,
  ConfigurationDiagnostic,
  CoreArtifact,
  CreatedAtCursor,
  StartupArtifactSummary,
} from '../generated';

export type {
  ActivationQueued,
  ActivationSummary,
  CatalogAsset,
  CatalogAssetList,
  CompiledConfigurationArtifact,
  ConfigurationAdapterSupport,
  ConfigurationCompile,
  ConfigurationDiagnostic,
  ConfigurationPreview,
  ConfigurationProfile,
  CoreArtifact,
  CoreArtifactPage,
  RuntimeStatus,
  StartupArtifactPage,
  StartupArtifactSummary,
} from '../generated';

export type ConfigurationDiagnosticClass = ConfigurationDiagnostic['class'];
export type StartupArtifactState = StartupArtifactSummary['state'];
export type MonitoringTier = ActivationSummary['monitoring_tier'];
export type CoreArtifactCursor = CreatedAtCursor;

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
  verificationState?: CoreArtifact['verification_state'];
}
