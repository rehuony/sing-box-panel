import type { ReactNode } from 'react';

import { useRef } from 'react';

import type { CanonicalDraftSession } from './canonical-draft.store';

import { CanonicalDraftContext } from './canonical-draft.store';

/** Session memory survives route changes, and is destroyed on sign-out. */
export function CanonicalDraftProvider({ children }: { children: ReactNode }) {
  const sessionRef = useRef<CanonicalDraftSession | null>(null);

  return <CanonicalDraftContext value={sessionRef}>{children}</CanonicalDraftContext>;
}
