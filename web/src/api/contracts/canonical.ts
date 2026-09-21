import type { CanonicalDocument } from '../generated';

export type {
  CanonicalDocument,
  CanonicalSnapshot,
} from '../generated';

export interface ManagedConfigurationEntry extends CanonicalDocument {
  tag?: string;
  type?: string;
}
