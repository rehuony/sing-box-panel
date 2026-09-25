import type { ReactNode } from 'react';

import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from 'react';

import type { AppearanceSettings } from '@/api/api-client';

import type { ThemeContextValue, ThemePreference } from './theme-context';

import { ThemeContext } from './theme-context';
import { applyAppearance, DEFAULT_APPEARANCE, readInitialAppearance } from './appearance';
import {
  applyThemeToDocument,
  nextThemePreference,
  readThemePreference,
  resolveTheme,
  SYSTEM_THEME_QUERY,
  THEME_STORAGE_KEY,
} from './theme';

export interface ThemeProviderProps {
  children: ReactNode;
}

function getSystemPreference(): boolean {
  return typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia(SYSTEM_THEME_QUERY).matches;
}

function getStoredPreference(): ThemePreference {
  if (typeof window === 'undefined') return 'system';
  try {
    return readThemePreference(window.localStorage);
  } catch {
    return 'system';
  }
}

export function ThemeProvider({ children }: ThemeProviderProps) {
  const [appearanceState, setAppearanceState] = useState<{
    saved: AppearanceSettings;
    draft: AppearanceSettings | null;
  }>(() => ({
    saved: readInitialAppearance() ?? { ...DEFAULT_APPEARANCE, theme: getStoredPreference() },
    draft: null,
  }));
  const { saved, draft } = appearanceState;
  const [systemPrefersDark, setSystemPrefersDark] = useState(getSystemPreference);
  const appearance = draft ?? saved;
  const preference = appearance.theme;
  const resolvedTheme = resolveTheme(preference, systemPrefersDark);

  const setPreference = useCallback((value: ThemePreference) => {
    setAppearanceState(current => current.draft === null
      ? { ...current, saved: { ...current.saved, theme: value } }
      : { ...current, draft: { ...current.draft, theme: value } });
  }, []);

  const setAppearance = useCallback((value: AppearanceSettings) => {
    setAppearanceState(current => ({ saved: value, draft: current.draft === null ? null : value }));
  }, []);

  const previewAppearance = useCallback((value: Partial<AppearanceSettings> | null) => {
    setAppearanceState(current => ({
      ...current,
      draft: value === null ? null : { ...(current.draft ?? current.saved), ...value },
    }));
  }, []);

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return undefined;

    const mediaQuery = window.matchMedia(SYSTEM_THEME_QUERY);
    const handleChange = (event: MediaQueryListEvent) => setSystemPrefersDark(event.matches);
    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  useLayoutEffect(() => {
    const root = document.documentElement;
    const themeColorMeta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    applyThemeToDocument(root, preference, resolvedTheme, themeColorMeta);
    applyAppearance(appearance, resolvedTheme === 'dark');

    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, saved.theme);
    } catch {
      // The selected theme still applies when storage is unavailable.
    }
  }, [appearance, preference, resolvedTheme, saved.theme]);

  const cycleTheme = useCallback(() => {
    setPreference(nextThemePreference(preference));
  }, [preference, setPreference]);

  const value = useMemo<ThemeContextValue>(() => ({
    appearance,
    preference,
    resolvedTheme,
    setPreference,
    setAppearance,
    previewAppearance,
    cycleTheme,
  }), [appearance, cycleTheme, preference, previewAppearance, resolvedTheme, setAppearance, setPreference]);

  return <ThemeContext value={value}>{children}</ThemeContext>;
}
