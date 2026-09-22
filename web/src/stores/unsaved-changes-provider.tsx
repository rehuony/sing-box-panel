import type { ReactNode } from 'react';

import { useBlocker } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { UnsavedChangesDialog } from '@/components/unsaved-changes-dialog';

import type { UnsavedChange } from './unsaved-changes.store';

import { UnsavedChangesContext } from './unsaved-changes.store';

/** One blocker covers all mounted editors, including nested dialogs. */
export function UnsavedChangesProvider({ children }: { children: ReactNode }) {
  const [changes, setChanges] = useState(() => new Map<string, UnsavedChange>());
  const [pending, setPending] = useState<(() => void) | null>(null);
  const dirty = changes.size > 0;
  const busy = [...changes.values()].some(change => change.busy);
  const blocker = useBlocker(({ currentLocation, nextLocation }) => {
    if (!dirty) return false;
    if (currentLocation.pathname !== nextLocation.pathname) return true;
    if ([...changes.values()].every(change => change.allowSamePathNavigation)) return false;
    return currentLocation.search !== nextLocation.search || currentLocation.hash !== nextLocation.hash;
  });

  const register = useCallback((id: string, change: UnsavedChange) => {
    setChanges(current => new Map(current).set(id, change));
    return () => setChanges(current => {
      const next = new Map(current);
      next.delete(id);
      return next;
    });
  }, []);
  const confirmNavigation = useCallback((action: () => void) => {
    if (dirty) setPending(() => action);
    else action();
  }, [dirty]);
  const value = useMemo(() => ({ register, confirmNavigation }), [register, confirmNavigation]);

  useEffect(() => {
    if (!dirty) return;
    const prevent = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', prevent);
    return () => window.removeEventListener('beforeunload', prevent);
  }, [dirty]);

  function cancel() {
    setPending(null);
    if (blocker.state === 'blocked') blocker.reset();
  }
  function discard() {
    if (busy) return;
    changes.forEach(change => change.discard());
    setPending(null);
    if (blocker.state === 'blocked') blocker.proceed();
    else pending?.();
  }

  return (
    <UnsavedChangesContext value={value}>
      {children}
      <UnsavedChangesDialog open={blocker.state === 'blocked' || pending !== null} busy={busy} onCancel={cancel} onDiscard={discard} />
    </UnsavedChangesContext>
  );
}
