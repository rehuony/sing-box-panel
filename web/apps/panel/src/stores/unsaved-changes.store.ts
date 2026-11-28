import { createContext, use } from 'react';

export interface UnsavedChange {
  busy: boolean;
  discard: () => void;
  allowSamePathNavigation?: boolean;
}

interface UnsavedChangesContextValue {
  /**
   * Runs action, letting the route changes it starts pass without asking:
   * for a page whose changes are already settled, such as a form just saved
   * or an object just deleted, before the change leaves the guard. Going back
   * or forward is never let through.
   */
  skipGuard: (action: () => void) => void;
  confirmNavigation: (action: () => void) => void;
  register: (id: string, change: UnsavedChange) => () => void;
}

export const UnsavedChangesContext = createContext<UnsavedChangesContextValue | null>(null);

export function useUnsavedChangesContext() {
  const context = use(UnsavedChangesContext);
  if (!context) throw new Error('UnsavedChangesProvider is required.');
  return context;
}
