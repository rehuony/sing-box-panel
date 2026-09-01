import type { ReactNode } from 'react';

import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from 'react';

import type { ThemeContextValue, ThemePreference } from './theme-context';

import { ThemeContext } from './theme-context';
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
  const [preference, setPreference] = useState<ThemePreference>(getStoredPreference);
  const [systemPrefersDark, setSystemPrefersDark] = useState(getSystemPreference);
  const resolvedTheme = resolveTheme(preference, systemPrefersDark);

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

    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, preference);
    } catch {
      // The selected theme still applies when storage is unavailable.
    }
  }, [preference, resolvedTheme]);

  const cycleTheme = useCallback(() => {
    setPreference((current) => nextThemePreference(current));
  }, []);

  const value = useMemo<ThemeContextValue>(() => ({
    preference,
    resolvedTheme,
    setPreference,
    cycleTheme,
  }), [cycleTheme, preference, resolvedTheme]);

  return <ThemeContext value={value}>{children}</ThemeContext>;
}
