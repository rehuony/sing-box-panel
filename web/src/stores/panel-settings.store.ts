import { createContext, use } from 'react';

import type { AppearanceSettings, PanelSettingsView, PanelSettingsWrite } from '@/api/api-client';

export interface PanelSettingsContextValue {
  reload: () => void;
  error: string | null;
  view: PanelSettingsView | null;
  accept: (view: PanelSettingsView) => Promise<void>;
  preview: (value: Partial<AppearanceSettings> | null) => void;
  save: (input: PanelSettingsWrite) => Promise<PanelSettingsView>;
}

export const PanelSettingsContext = createContext<PanelSettingsContextValue | null>(null);
export function usePanelSettings() {
  const value = use(PanelSettingsContext);
  if (!value) throw new Error('usePanelSettings requires PanelSettingsProvider');
  return value;
}
