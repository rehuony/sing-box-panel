import type { JsonObject } from './common';

export interface ManagedConfigurationEntry extends JsonObject {
  tag?: string;
  type?: string;
  _panel: {
    id: string;
    enabled: boolean;
  };
}

export interface SingBoxConfiguration extends JsonObject {
  dns?: JsonObject;
  log?: JsonObject;
  ntp?: JsonObject;
  route?: JsonObject;
  certificate?: JsonObject;
  experimental?: JsonObject;
  inbounds?: ManagedConfigurationEntry[];
  services?: ManagedConfigurationEntry[];
  endpoints?: ManagedConfigurationEntry[];
  outbounds?: ManagedConfigurationEntry[];
}

// CanonicalDocument is the only configuration history shared by every exact
// sing-box adapter. Unsupported fields remain here and are omitted only from
// an exact-version projection.
export interface CanonicalDocument {
  schema_version: 2;
  configuration: SingBoxConfiguration;
}

export interface CanonicalSnapshot {
  id: string;
  sha256: string;
  sequence: number;
  schema_version: 2;
  created_at: string;
  parent_id?: string;
  document_json: string;
  document: CanonicalDocument;
}

export type CanonicalChange
  = | { op: 'set'; path: string; value_json: string }
    | { op: 'unset'; path: string };

export interface CanonicalSave {
  task_id?: string;
  no_change: boolean;
  revision: CanonicalSnapshot;
}

export interface CanonicalRevisionPage {
  items: CanonicalSnapshot[];
  next_before_sequence?: number;
}

export interface CanonicalDiffValue {
  value?: unknown;
  present: boolean;
}

export interface CanonicalRevisionDiff {
  to: CanonicalSnapshot;
  from: CanonicalSnapshot;
  changes: Array<{
    path: string;
    from: CanonicalDiffValue;
    to: CanonicalDiffValue;
  }>;
}
