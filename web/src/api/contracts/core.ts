import type { Task } from './tasks';
import type { JsonObject } from './common';
import type { CanonicalSnapshot } from './canonical';

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

export interface CoreImportUpload {
  archive: File;
  exactVersion: string;
  sourceDescription: string;
  variant: 'plain' | 'legacy';
  architecture: 'amd64' | 'arm64';
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

export interface ConfigurationProfile {
  os: string;
  arch: string;
  variant: string;
  exact_version: string;
  feature_fingerprint: unknown;
}

export interface ConfigurationProvenance {
  source: string;
  upstream_tag: string;
  upstream_commit: string;
}

export interface ConfigurationAdapterSupport {
  reason?: string;
  supported: boolean;
  adapter_id?: string;
  adapter_revision?: string;
  profile: ConfigurationProfile;
  provenance?: ConfigurationProvenance;
}

export type ConfigurationDiagnosticClass = 'direct' | 'mapped' | 'ignored' | 'blocking';

export interface ConfigurationDiagnostic {
  code: string;
  path: string;
  message: string;
  class: ConfigurationDiagnosticClass;
}

export interface ConfigurationPreview {
  config: JsonObject;
  ignored_digest?: string;
  core_artifact: CoreArtifact;
  support: ConfigurationAdapterSupport;
  canonical_revision: CanonicalSnapshot;
  diagnostics: ConfigurationDiagnostic[];
}

export type StartupArtifactState = 'pending' | 'ready' | 'failed';
export type MonitoringTier = 'full' | 'limited' | 'process_only';

export interface CompiledConfigurationArtifact {
  id: string;
  adapter_id: string;
  config_sha256: string;
  ignored_digest?: string;
  adapter_revision: string;
  core_artifact_id: string;
  exact_core_version: string;
  state: StartupArtifactState;
  canonical_revision_id: string;
  diagnostics: ConfigurationDiagnostic[];
}

export interface ConfigurationCompile {
  task: Task;
  support: ConfigurationAdapterSupport;
  artifact: CompiledConfigurationArtifact;
}

export interface StartupArtifactSummary extends Omit<CompiledConfigurationArtifact, 'diagnostics'> {
  created_at: string;
  checked_at?: string;
  diagnostics: ConfigurationDiagnostic[];
}

export interface StartupArtifactPage {
  items: StartupArtifactSummary[];
  next?: { created_at: string; id: string };
}

export interface ActivationSummary {
  config_sha256: string;
  core_artifact_id: string;
  activation_sha256: string;
  exact_core_version: string;
  startup_artifact_id: string;
  activation_bundle_id: string;
  canonical_revision_id: string;
  monitoring_tier: MonitoringTier;
}

export interface ActivationQueued {
  task: Task;
  activation: ActivationSummary;
}

export interface RuntimeStatus {
  desired_running: boolean;
  observation_state: string;
  target_generation: number;
  applied_bundle_id?: string;
  desired_bundle_id?: string;
  rollback_bundle_id?: string;
  running?: {
    exact_version: string;
    artifact_id: string;
    artifact_digest: string;
  };
}
