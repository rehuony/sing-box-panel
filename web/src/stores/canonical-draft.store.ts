import type { RefObject } from 'react';

import { createContext, use } from 'react';

import type { ConfigurationFile } from '@/api/api-client';

export interface CanonicalDraftSession {
  dirty: boolean;
  content: string;
  file: ConfigurationFile;
}

export const CanonicalDraftContext = createContext<RefObject<CanonicalDraftSession | null> | null>(null);

export function useCanonicalDraftSession() {
  const session = use(CanonicalDraftContext);
  if (session === null) throw new Error('CanonicalDraftProvider is required.');
  return session;
}
