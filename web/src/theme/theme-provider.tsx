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
  const [initialAppearance] = useState(readInitialAppearance);
  const [savedAppearance, setSavedAppearance] = useState(initialAppearance ?? DEFAULT_APPEARANCE);
  const [draftAppearance, setDraftAppearance] = useState<AppearanceSettings | null>(null);
  const [savedPreference, setSavedPreference] = useState<ThemePreference>(
    () => initialAppearance?.theme ?? getStoredPreference(),
  );
  const [systemPrefersDark, setSystemPrefersDark] = useState(getSystemPreference);
  const preference = draftAppearance?.theme ?? savedPreference;
  const resolvedTheme = resolveTheme(preference, systemPrefersDark);
  const activeAppearance = draftAppearance ?? savedAppearance;

  const setPreference = useCallback((value: ThemePreference) => {
    setSavedPreference(value);
    setDraftAppearance(current => current === null ? null : { ...current, theme: value });
  }, []);

  const setAppearance = useCallback((value: AppearanceSettings) => {
    setSavedAppearance(value);
    setSavedPreference(value.theme);
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
    applyAppearance(activeAppearance, resolvedTheme === 'dark');

    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, savedPreference);
    } catch {
      // The selected theme still applies when storage is unavailable.
    }
  }, [activeAppearance, preference, resolvedTheme, savedPreference]);

  const cycleTheme = useCallback(() => {
    setPreference(nextThemePreference(preference));
  }, [preference, setPreference]);

  const value = useMemo<ThemeContextValue>(() => ({
    preference,
    resolvedTheme,
    setPreference,
    setAppearance,
    previewAppearance: setDraftAppearance,
    cycleTheme,
  }), [cycleTheme, preference, resolvedTheme, setAppearance, setPreference]);

  return <ThemeContext value={value}>{children}</ThemeContext>;
}
