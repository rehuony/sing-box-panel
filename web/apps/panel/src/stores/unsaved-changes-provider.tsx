import type { ReactNode } from 'react';

import { NavigationType, useBlocker } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { UnsavedChangesDialog } from '@/components/unsaved-changes-dialog';

import type { UnsavedChange } from './unsaved-changes.store';

import { UnsavedChangesContext } from './unsaved-changes.store';

/** One blocker covers all mounted editors, including nested dialogs. */
export function UnsavedChangesProvider({ children }: { children: ReactNode }) {
  const [changes, setChanges] = useState(() => new Map<string, UnsavedChange>());
  const [pending, setPending] = useState<(() => void) | null>(null);
  const dirty = changes.size > 0;
  const busy = [...changes.values()].some(change => change.busy);
  // Read when the router asks, which can be before a settled change has left the guard.
  const skippingRef = useRef(false);
  const blocker = useBlocker(({ currentLocation, historyAction, nextLocation }) => {
    if (!dirty || (skippingRef.current && historyAction !== NavigationType.Pop)) return false;
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
  const skipGuard = useCallback((action: () => void) => {
    skippingRef.current = true;
    try {
      action();
    } finally {
      skippingRef.current = false;
    }
  }, []);
  const value = useMemo(() => ({ register, confirmNavigation, skipGuard }), [register, confirmNavigation, skipGuard]);

  useEffect(() => {
    if (!dirty) return;
    // Browsers show their own prompt; older ones need a return value as well.
    const prevent = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = true;
    };
    window.addEventListener('beforeunload', prevent);
    return () => window.removeEventListener('beforeunload', prevent);
  }, [dirty]);

  useEffect(() => {
    // Authentication loss unmounts editors. A pending route must not keep an
    // obsolete draft dialog in front of the login page after that cleanup.
    if (!dirty && blocker.state === 'blocked') blocker.proceed();
  }, [dirty, blocker]);

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
