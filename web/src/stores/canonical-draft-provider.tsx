import type { ReactNode } from 'react';

import { useEffect, useRef } from 'react';

import type { CanonicalDraftSession } from './canonical-draft.store';

import { CanonicalDraftContext } from './canonical-draft.store';

/** Session memory survives route changes, and is destroyed on sign-out. */
export function CanonicalDraftProvider({ children }: { children: ReactNode }) {
  const sessionRef = useRef<CanonicalDraftSession | null>(null);

  useEffect(() => {
    function protectDraft(event: BeforeUnloadEvent) {
      if (!sessionRef.current?.dirty) return;
      event.preventDefault();
      event.returnValue = '';
    }
    window.addEventListener('beforeunload', protectDraft);
    return () => window.removeEventListener('beforeunload', protectDraft);
  }, []);

  return <CanonicalDraftContext value={sessionRef}>{children}</CanonicalDraftContext>;
}
