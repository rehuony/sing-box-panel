import type { ThemePreference } from '@shared/theme/theme';

import { useCallback, useEffect, useState } from 'react';
import {
  applyThemeToDocument,
  nextThemePreference,
  readThemePreference,
  resolveTheme,
  SYSTEM_THEME_QUERY,
  THEME_STORAGE_KEY,
} from '@shared/theme/theme';

function storage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

/** The theme preference remembered in this browser; it falls back to the system's appearance. */
export function storedThemePreference(): ThemePreference {
  return readThemePreference(storage()) ?? 'system';
}

/** Shows the preference on the document, following the system while it is "system". */
export function applyThemePreference(preference: ThemePreference): () => void {
  const query = window.matchMedia(SYSTEM_THEME_QUERY);
  const apply = () => applyThemeToDocument(
    document.documentElement,
    preference,
    resolveTheme(preference, query.matches),
    document.querySelector<HTMLMetaElement>('meta[name="theme-color"]'),
  );
  apply();
  query.addEventListener('change', apply);
  return () => query.removeEventListener('change', apply);
}

export function useThemePreference() {
  const [preference, setPreference] = useState(storedThemePreference);
  useEffect(() => applyThemePreference(preference), [preference]);
  const cycle = useCallback(() => {
    setPreference((current) => {
      const next = nextThemePreference(current);
      try {
        storage()?.setItem(THEME_STORAGE_KEY, next);
      } catch {
        // The choice still applies to this page.
      }
      return next;
    });
  }, []);
  return { preference, cycle };
}
