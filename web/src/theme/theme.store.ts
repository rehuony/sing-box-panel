import type { PersistStorage } from 'zustand/middleware';

import { persist } from 'zustand/middleware';
import { createStore } from 'zustand/vanilla';

import type { ThemePreference } from './theme-context';

import { readThemePreference, THEME_STORAGE_KEY } from './theme';

interface LocalThemeState {
  preference: ThemePreference | null;
  setPreference: (preference: ThemePreference) => void;
}

// Keep the existing raw light/dark/system storage format for saved browser choices.
const storage: PersistStorage<Pick<LocalThemeState, 'preference'>> = {
  getItem: () => {
    try {
      return { state: { preference: readThemePreference(window.localStorage) } };
    } catch {
      return null;
    }
  },
  setItem: (name, { state }) => {
    try {
      if (state.preference === null) window.localStorage.removeItem(name);
      else window.localStorage.setItem(name, state.preference);
    } catch {
      // Theme changes still apply when browser storage is unavailable.
    }
  },
  removeItem: name => {
    try {
      window.localStorage.removeItem(name);
    } catch {
      // Storage can be blocked by the browser.
    }
  },
};

export function createLocalThemeStore() {
  return createStore<LocalThemeState>()(persist(set => ({
    preference: null,
    setPreference: preference => set({ preference }),
  }), {
    name: THEME_STORAGE_KEY,
    storage,
    partialize: state => ({ preference: state.preference }),
  }));
}
