import type { JsonObject } from './common';
import type { StartupArtifactState } from './core';

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
  public_host: string;
  format: SubscriptionFormat;
  config: SubscriptionChannelConfig;
}

export type SubscriptionChannelSummary = Omit<SubscriptionChannel, 'config'>;

export interface SubscriptionChannelWrite {
  name: string;
  enabled: boolean;
  public_host: string;
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
  current_version_id?: string;
  source_kind: SubscriptionSourceKind;
}

export type SubscriptionSourceSummary = Omit<SubscriptionSource, 'config'> & {
  has_version: boolean;
};

export type SubscriptionSourceFormat = 'auto' | 'sing-box-json' | 'mihomo-yaml' | 'uri-list';

export interface SubscriptionSourceVersion {
  id: string;
  sha256: string;
  raw_body?: string;
  source_id: string;
  created_at: string;
  fetched_at: string;
  diagnostics: unknown[];
  normalized_nodes: unknown[];
  format: SubscriptionSourceFormat;
}

export interface SubscriptionSourceVersionPage {
  next?: SubscriptionCursor;
  items: SubscriptionSourceVersion[];
}

export interface SubscriptionSourceVersionSave {
  source: SubscriptionSource;
  version: SubscriptionSourceVersion;
}

export interface SubscriptionUser {
  id: string;
  name: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  description: string;
}

export interface SubscriptionUserWrite {
  name: string;
  enabled: boolean;
  description: string;
}

export interface SubscriptionUserPage {
  items: SubscriptionUser[];
  next?: SubscriptionCursor;
}

export interface SubscriptionUserGrants {
  grants: string[];
  user: SubscriptionUser;
}

export interface SubscriptionNodeSummary {
  key: string;
  tag: string;
  type: string;
  source_id: string;
  credential?: string;
}

export interface SubscriptionNodeCatalog {
  diagnostics: unknown[];
  applied_bundle_id: string;
  nodes: SubscriptionNodeSummary[];
}

export interface SubscriptionSourceWrite {
  name: string;
  enabled: boolean;
  config: JsonObject;
  source_kind: SubscriptionSourceKind;
}

export interface SubscriptionToken {
  id: string;
  label: string;
  active: boolean;
  user_id: string;
  enabled: boolean;
  created_at: string;
  expires_at?: string;
  revoked_at?: string;
  bytes_served: number;
  last_used_at?: string;
  body_response_count: number;
  successful_request_count: number;
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

export interface SubscriptionPreview {
  user_id: string;
  applied_bundle_id: string;
  exact_core_version: string;
  startup_artifact_id: string;
  channel: SubscriptionChannel;
  canonical_revision_id: string;
  artifact_state: StartupArtifactState;
  result: {
    format: SubscriptionFormat;
    media_type: string;
    content: string;
    node_count: number;
    diagnostics: unknown[];
  };
}
