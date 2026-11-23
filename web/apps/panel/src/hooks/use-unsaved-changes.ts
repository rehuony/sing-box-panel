import { useId, useLayoutEffect, useRef } from 'react';

import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';

export function useUnsavedChanges(
  dirty: boolean,
  discard: () => void,
  busy = false,
  options: { allowSamePathNavigation?: boolean } = {},
) {
  const id = useId();
  const latestDiscardRef = useRef(discard);
  const { register, confirmNavigation } = useUnsavedChangesContext();
  useLayoutEffect(() => {
    latestDiscardRef.current = discard;
  });
  useLayoutEffect(() => {
    if (dirty) {
      return register(id, {
        allowSamePathNavigation: options.allowSamePathNavigation,
        busy,
        discard: () => latestDiscardRef.current(),
      });
    }
  }, [busy, dirty, id, options.allowSamePathNavigation, register]);
  return confirmNavigation;
}
