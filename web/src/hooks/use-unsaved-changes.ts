import { useId, useLayoutEffect, useRef } from 'react';

import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';

export function useUnsavedChanges(dirty: boolean, discard: () => void, busy = false) {
  const id = useId();
  const latestDiscardRef = useRef(discard);
  const { register, confirmNavigation } = useUnsavedChangesContext();
  useLayoutEffect(() => {
    latestDiscardRef.current = discard;
  });
  useLayoutEffect(() => {
    if (dirty) return register(id, { busy, discard: () => latestDiscardRef.current() });
  }, [busy, dirty, id, register]);
  return confirmNavigation;
}
