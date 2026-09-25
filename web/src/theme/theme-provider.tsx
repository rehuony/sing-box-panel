import type { ReactNode } from 'react';

import { useStore } from 'zustand';
import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from 'react';

import type { AppearanceSettings } from '@/api/api-client';

import type { ThemeContextValue } from './theme-context';

import { ThemeContext } from './theme-context';
import { createLocalThemeStore } from './theme.store';
import { applyAppearance, DEFAULT_APPEARANCE, readInitialAppearance } from './appearance';
import {
  applyThemeToDocument,
  nextThemePreference,
  resolveTheme,
  SYSTEM_THEME_QUERY,
} from './theme';

export interface ThemeProviderProps {
  children: ReactNode;
}

function getSystemPreference(): boolean {
  return typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia(SYSTEM_THEME_QUERY).matches;
}

export function ThemeProvider({ children }: ThemeProviderProps) {
  const [localThemeStore] = useState(createLocalThemeStore);
  const localPreference = useStore(localThemeStore, state => state.preference);
  const setPreference = useStore(localThemeStore, state => state.setPreference);
  const [appearanceState, setAppearanceState] = useState<{
    saved: AppearanceSettings;
    draft: AppearanceSettings | null;
  }>(() => ({
    saved: readInitialAppearance() ?? DEFAULT_APPEARANCE,
    draft: null,
  }));
  const { saved, draft } = appearanceState;
  const [systemPrefersDark, setSystemPrefersDark] = useState(getSystemPreference);
  const appearance = draft ?? saved;
  const preference = localPreference ?? appearance.theme;
  const resolvedTheme = resolveTheme(preference, systemPrefersDark);

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
  }, [appearance, preference, resolvedTheme]);

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
