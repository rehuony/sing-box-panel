import type {
  CoreArtifact,
  RuntimeTransition,
} from '../generated';

export type {
  ActivationSummary,
  CatalogAssetList,
  ConfigurationCompile,
  ConfigurationSchemaContract,
  ConfigurationSupport,
  CoreArtifact,
  CoreArtifactPage,
  RuntimeHistoryPage,
  RuntimeStatus,
  RuntimeTransition,
  StartupArtifactSummary,
} from '../generated';

export type RuntimeTransitionState = RuntimeTransition['state'];

export interface CatalogAssetFilter {
  variant?: string;
  architecture?: string;
  exactVersion?: string;
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
