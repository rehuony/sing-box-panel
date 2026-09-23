import type { ReactNode } from 'react';

import { useLocation } from 'react-router-dom';
import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from 'react';

import type { AppearanceSettings, PanelSettingsView, PanelSettingsWrite } from '@/api/api-client';

import { setAppLanguage } from '@/i18n';
import { useTheme } from '@/theme/theme-context';
import { useApiClient } from '@/api/api-client-context';
import { applyAppearance, DEFAULT_APPEARANCE } from '@/theme/appearance';

import { PanelSettingsContext } from './panel-settings.store';

export function PanelSettingsProvider({ children }: { children: ReactNode }) {
  const api = useApiClient();
  const { pathname } = useLocation();
  const { resolvedTheme, setPreference } = useTheme();
  const [view, setView] = useState<PanelSettingsView | null>(null);
  const [draftAppearance, setDraftAppearance] = useState<AppearanceSettings | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const reload = useCallback(() => {
    setError(null);
    setReloadKey(key => key + 1);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void api.getPanelSettings(controller.signal).then(result => {
      if (controller.signal.aborted) return;
      setView(result);
      setPreference(result.preferences.appearance.theme);
      void setAppLanguage(result.preferences.language);
    }).catch((cause: unknown) => {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Settings unavailable');
    });
    return () => controller.abort();
  }, [api, reloadKey, setPreference]);

  const preview = useCallback((value: AppearanceSettings | null) => setDraftAppearance(value), []);
  const isSettings = pathname === '/panel';
  const appearance = (isSettings ? draftAppearance : null) ?? view?.preferences.appearance ?? DEFAULT_APPEARANCE;
  useLayoutEffect(() => {
    applyAppearance(appearance, resolvedTheme === 'dark');
  }, [appearance, resolvedTheme]);
  useEffect(() => {
    if (view !== null) setPreference(appearance.theme);
  }, [appearance.theme, setPreference, view]);

  const accept = useCallback(async (result: PanelSettingsView) => {
    setView(result);
    setDraftAppearance(null);
    setPreference(result.preferences.appearance.theme);
    await setAppLanguage(result.preferences.language);
  }, [setPreference]);
  const save = useCallback(async (input: PanelSettingsWrite) => {
    const result = await api.savePanelSettings(input);
    await accept(result);
    return result;
  }, [api, accept]);
  const value = useMemo(
    () => ({ view, error, reload, preview, save, accept }),
    [view, error, reload, preview, save, accept],
  );
  return <PanelSettingsContext value={value}>{children}</PanelSettingsContext>;
}
