import { useStore } from 'zustand';
import { createContext, use } from 'react';
import { createStore } from 'zustand/vanilla';

import type { ConfigurationFile } from '@/api/api-client';

export interface ConfigurationDraftSession {
  content: string;
  file: ConfigurationFile;
}

interface ConfigurationSessionState {
  resetDraft: () => void;
  selectedVersion: string | null;
  draft: ConfigurationDraftSession | null;
  setSelectedVersion: (version: string | null) => void;
  replaceDraft: (draft: ConfigurationDraftSession) => void;
  reconcileVersion: (versions: string[], enabledVersion?: string) => void;
  updateDraftContent: (update: string | ((content: string) => string)) => void;
}

export function createConfigurationSessionStore() {
  return createStore<ConfigurationSessionState>()((set) => ({
    draft: null,
    selectedVersion: null,
    reconcileVersion: (versions, enabledVersion) => set((state) => {
      if (state.selectedVersion !== null && versions.includes(state.selectedVersion)) return state;
      return {
        selectedVersion: enabledVersion !== undefined && versions.includes(enabledVersion)
          ? enabledVersion
          : versions[0] ?? null,
      };
    }),
    replaceDraft: draft => set({ draft }),
    resetDraft: () => set((state) => state.draft === null
      ? state
      : { draft: { ...state.draft, content: state.draft.file.content } }),
    setSelectedVersion: selectedVersion => set({ selectedVersion }),
    updateDraftContent: update => set((state) => {
      if (state.draft === null) return state;
      const content = typeof update === 'function' ? update(state.draft.content) : update;
      return content === state.draft.content
        ? state
        : { draft: { ...state.draft, content } };
    }),
  }));
}

export type ConfigurationSessionStore = ReturnType<typeof createConfigurationSessionStore>;

export const ConfigurationSessionContext = createContext<ConfigurationSessionStore | null>(null);

export function useConfigurationSessionStore<T>(selector: (state: ConfigurationSessionState) => T): T {
  const store = use(ConfigurationSessionContext);
  if (store === null) throw new Error('ConfigurationSessionProvider is required.');
  return useStore(store, selector);
}

export function useConfigurationSessionStoreApi(): ConfigurationSessionStore {
  const store = use(ConfigurationSessionContext);
  if (store === null) throw new Error('ConfigurationSessionProvider is required.');
  return store;
}
