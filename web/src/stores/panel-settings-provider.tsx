import type { ReactNode } from 'react';

import { useLocation } from 'react-router-dom';
import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from 'react';

import type { AppearanceSettings, PanelSettingsView, PanelSettingsWrite } from '@/api/api-client';

import { setAppLanguage } from '@/i18n';
import { useTheme } from '@/theme/theme-context';
import { useApiClient } from '@/api/api-client-context';

import { PanelSettingsContext } from './panel-settings.store';

export function PanelSettingsProvider({ children }: { children: ReactNode }) {
  const api = useApiClient();
  const { pathname } = useLocation();
  const { setAppearance, previewAppearance } = useTheme();
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
      setAppearance(result.preferences.appearance);
      void setAppLanguage(result.preferences.language);
    }).catch((cause: unknown) => {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Settings unavailable');
    });
    return () => controller.abort();
  }, [api, reloadKey, setAppearance]);

  const preview = useCallback((value: AppearanceSettings | null) => setDraftAppearance(value), []);
  const isSettings = pathname === '/panel';
  useLayoutEffect(() => {
    previewAppearance(isSettings ? draftAppearance : null);
    return () => previewAppearance(null);
  }, [draftAppearance, isSettings, previewAppearance]);

  const accept = useCallback(async (result: PanelSettingsView) => {
    setView(result);
    setDraftAppearance(null);
    setAppearance(result.preferences.appearance);
    await setAppLanguage(result.preferences.language);
  }, [setAppearance]);
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
