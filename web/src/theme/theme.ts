import type { ResolvedTheme, ThemePreference } from './theme-context';

export const THEME_STORAGE_KEY = 'sing-box-panel.theme';
export const SYSTEM_THEME_QUERY = '(prefers-color-scheme: dark)';

const THEME_COLORS: Record<ResolvedTheme, string> = {
  light: '#f7f7fb',
  dark: '#090b12',
};

export function isThemePreference(value: unknown): value is ThemePreference {
  return value === 'light' || value === 'system' || value === 'dark';
}

export function readThemePreference(storage: Pick<Storage, 'getItem'> | null = null): ThemePreference | null {
  try {
    const value = storage?.getItem(THEME_STORAGE_KEY);
    return isThemePreference(value) ? value : null;
  } catch {
    return null;
  }
}

export function resolveTheme(preference: ThemePreference, systemPrefersDark: boolean): ResolvedTheme {
  if (preference === 'system') {
    return systemPrefersDark ? 'dark' : 'light';
  }
  return preference;
}

export function nextThemePreference(preference: ThemePreference): ThemePreference {
  if (preference === 'light') return 'system';
  if (preference === 'system') return 'dark';
  return 'light';
}

export function applyThemeToDocument(
  root: HTMLElement,
  preference: ThemePreference,
  resolvedTheme: ResolvedTheme,
  themeColorMeta?: HTMLMetaElement | null,
): void {
  root.classList.toggle('dark', resolvedTheme === 'dark');
  root.dataset.theme = preference;
  root.dataset.resolvedTheme = resolvedTheme;
  root.style.colorScheme = resolvedTheme;
  themeColorMeta?.setAttribute('content', THEME_COLORS[resolvedTheme]);
}
