import type { CanonicalDocument } from '../generated';

export type {
  CanonicalChange,
  CanonicalDocument,
  CanonicalRevisionDiff,
  CanonicalRevisionPage,
  CanonicalSave,
  CanonicalSnapshot,
} from '../generated';

export interface CanonicalRevisionListFilter {
  limit?: number;
  beforeSequence?: number;
}

export interface ManagedConfigurationEntry extends CanonicalDocument {
  tag?: string;
  type?: string;
}
