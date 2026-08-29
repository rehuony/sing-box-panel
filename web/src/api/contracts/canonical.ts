import type { JsonObject } from './common';

export type {
  CanonicalChange,
  CanonicalDocument,
  CanonicalRevisionDiff,
  CanonicalRevisionPage,
  CanonicalSave,
  CanonicalSnapshot,
} from '../generated';

export interface ManagedConfigurationEntry extends JsonObject {
  tag?: string;
  type?: string;
  _panel: {
    id: string;
    enabled: boolean;
  };
}
