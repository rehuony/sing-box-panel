import { createContext, use } from 'react';

export interface UnsavedChange {
  busy: boolean;
  discard: () => void;
  allowSamePathNavigation?: boolean;
}

interface UnsavedChangesContextValue {
  confirmNavigation: (action: () => void) => void;
  register: (id: string, change: UnsavedChange) => () => void;
}

export const UnsavedChangesContext = createContext<UnsavedChangesContextValue | null>(null);

export function useUnsavedChangesContext() {
  const context = use(UnsavedChangesContext);
  if (!context) throw new Error('UnsavedChangesProvider is required.');
  return context;
}
