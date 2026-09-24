import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ThemeProvider, useTheme } from '@/theme';
import { appearanceTokens } from '@/theme/appearance';
import {
  applyThemeToDocument,
  nextThemePreference,
  readThemePreference,
  resolveTheme,
  SYSTEM_THEME_QUERY,
  THEME_STORAGE_KEY,
} from '@/theme/theme';

interface ColorSchemeController {
  listenerCount: () => number;
  set: (matches: boolean) => void;
}

function installColorSchemePreference(initialValue: boolean): ColorSchemeController {
  let matches = initialValue;
  const listeners = new Set<(event: MediaQueryListEvent) => void>();
  const mediaQueryList = {
    get matches() {
      return matches;
    },
    media: SYSTEM_THEME_QUERY,
    onchange: null,
    addEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) => listeners.add(listener),
    removeEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) => listeners.delete(listener),
  } as unknown as MediaQueryList;

  vi.stubGlobal('matchMedia', vi.fn(() => mediaQueryList));

  return {
    listenerCount: () => listeners.size,
    set(nextValue: boolean) {
      matches = nextValue;
      const event = new Event('change') as MediaQueryListEvent;
      Object.defineProperty(event, 'matches', { value: matches });
      act(() => listeners.forEach(listener => listener(event)));
    },
  };
}

function ThemeProbe() {
  const { cycleTheme, preference, resolvedTheme } = useTheme();
  return (
    <button data-resolved-theme={resolvedTheme} onClick={cycleTheme} type='button'>
      {preference}
    </button>
  );
}

function renderTheme() {
  return render(<ThemeProvider><ThemeProbe /></ThemeProvider>);
}

beforeEach(() => {
  window.localStorage.clear();
  document.documentElement.className = '';
  document.documentElement.removeAttribute('data-theme');
  document.documentElement.removeAttribute('data-resolved-theme');
  document.documentElement.removeAttribute('style');
  document.head.innerHTML = '<meta name="theme-color" content="#000000">';
});

afterEach(() => vi.unstubAllGlobals());

describe('theme contract', () => {
  it('falls back to system for missing, invalid, or inaccessible preferences', () => {
    expect(readThemePreference(window.localStorage)).toBe('system');
    window.localStorage.setItem(THEME_STORAGE_KEY, 'sepia');
    expect(readThemePreference(window.localStorage)).toBe('system');
    expect(readThemePreference({ getItem: () => {
      throw new Error('blocked');
    } })).toBe('system');
  });

  it('resolves system appearance and cycles light, system, dark, then light', () => {
    expect(resolveTheme('system', false)).toBe('light');
    expect(resolveTheme('system', true)).toBe('dark');
    expect(nextThemePreference('light')).toBe('system');
    expect(nextThemePreference('system')).toBe('dark');
    expect(nextThemePreference('dark')).toBe('light');
  });

  it('synchronizes all document theme signals and the browser chrome color', () => {
    const root = document.documentElement;
    const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');

    applyThemeToDocument(root, 'system', 'dark', meta);
    expect(root).toHaveClass('dark');
    expect(root).toHaveAttribute('data-theme', 'system');
    expect(root).toHaveAttribute('data-resolved-theme', 'dark');
    expect(root.style.colorScheme).toBe('dark');
    expect(meta).toHaveAttribute('content', '#090b12');

    applyThemeToDocument(root, 'light', 'light', meta);
    expect(root).not.toHaveClass('dark');
    expect(root.style.colorScheme).toBe('light');
    expect(meta).toHaveAttribute('content', '#f7f7fb');
  });
});

describe('themeProvider', () => {
  it('applies the server appearance before login and follows system changes with the same palette', () => {
    const colorScheme = installColorSchemePreference(false);
    const appearance = { theme: 'system', color: '#C65B13', radius: 8 } as const;
    const meta = document.createElement('meta');
    meta.name = 'sing-box-panel-appearance';
    meta.content = JSON.stringify(appearance);
    document.head.append(meta);
    window.localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    renderTheme();

    const assertTokens = (dark: boolean) => {
      for (const [name, value] of Object.entries(appearanceTokens(appearance, dark))) {
        expect(document.documentElement.style.getPropertyValue(name)).toBe(value);
      }
    };
    expect(screen.getByRole('button', { name: 'system' })).toHaveAttribute('data-resolved-theme', 'light');
    assertTokens(false);
    colorScheme.set(true);
    expect(document.documentElement).toHaveClass('dark');
    assertTokens(true);
  });

  it('defaults to system, follows live system changes, and persists the preference', () => {
    const colorScheme = installColorSchemePreference(true);
    const { unmount } = renderTheme();

    const trigger = screen.getByRole('button', { name: 'system' });
    expect(trigger).toHaveAttribute('data-resolved-theme', 'dark');
    expect(document.documentElement).toHaveClass('dark');
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe('system');

    colorScheme.set(false);
    expect(trigger).toHaveAttribute('data-resolved-theme', 'light');
    expect(document.documentElement).not.toHaveClass('dark');

    expect(colorScheme.listenerCount()).toBe(1);
    unmount();
    expect(colorScheme.listenerCount()).toBe(0);
  });

  it('restores a saved preference and cycles through all three states', () => {
    installColorSchemePreference(false);
    window.localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderTheme();

    let trigger = screen.getByRole('button', { name: 'light' });
    fireEvent.click(trigger);
    trigger = screen.getByRole('button', { name: 'system' });
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe('system');

    fireEvent.click(trigger);
    trigger = screen.getByRole('button', { name: 'dark' });
    expect(document.documentElement).toHaveClass('dark');

    fireEvent.click(trigger);
    expect(screen.getByRole('button', { name: 'light' })).toBeInTheDocument();
    expect(document.documentElement).not.toHaveClass('dark');
  });

  it('restores invalid saved state as system', () => {
    installColorSchemePreference(false);
    window.localStorage.setItem(THEME_STORAGE_KEY, 'invalid');
    renderTheme();

    expect(screen.getByRole('button', { name: 'system' })).toBeInTheDocument();
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe('system');
  });
});
