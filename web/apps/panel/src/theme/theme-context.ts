import type { ResolvedTheme, ThemePreference } from '@shared/theme/theme';

import { createContext, use } from 'react';

import type { AppearanceSettings } from '@/api/api-client';

export interface ThemeContextValue {
  cycleTheme: () => void;
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  appearance: AppearanceSettings;
  setPreference: (preference: ThemePreference) => void;
  previewAppearance: (appearance: Partial<AppearanceSettings> | null) => void;
  setAppearance: (appearance: AppearanceSettings, preserveDraft?: boolean) => void;
}

export const ThemeContext = createContext<ThemeContextValue | null>(null);

export function useTheme(): ThemeContextValue {
  const value = use(ThemeContext);
  if (value === null) {
    throw new Error('useTheme must be used within ThemeProvider');
  }
  return value;
}
