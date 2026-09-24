import { createContext, use } from 'react';

import type { AppearanceSettings } from '@/api/api-client';

export type ThemePreference = 'light' | 'system' | 'dark';
export type ResolvedTheme = 'light' | 'dark';

export interface ThemeContextValue {
  cycleTheme: () => void;
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setPreference: (preference: ThemePreference) => void;
  setAppearance: (appearance: AppearanceSettings) => void;
  previewAppearance: (appearance: AppearanceSettings | null) => void;
}

export const ThemeContext = createContext<ThemeContextValue | null>(null);

export function useTheme(): ThemeContextValue {
  const value = use(ThemeContext);
  if (value === null) {
    throw new Error('useTheme must be used within ThemeProvider');
  }
  return value;
}
