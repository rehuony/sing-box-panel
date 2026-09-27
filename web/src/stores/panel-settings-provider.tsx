import type { ReactNode } from 'react';

import { useLocation } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useLayoutEffect, useMemo } from 'react';

import type { PanelSettingsView, PanelSettingsWrite } from '@/api/api-client';

import { queries } from '@/api/queries';
import { setAppLanguage } from '@/i18n';
import { useTheme } from '@/theme/theme-context';
import { useApiClient } from '@/api/api-client-context';

import { PanelSettingsContext } from './panel-settings.store';

export function PanelSettingsProvider({ children }: { children: ReactNode }) {
  const api = useApiClient();
  const { pathname } = useLocation();
  const { setAppearance, previewAppearance: preview } = useTheme();
  const cache = useQueryClient();
  const query = useQuery(queries.settings(api));
  const view = query.data ?? null;
  const error = query.error?.message ?? null;
  const reload = useCallback(() => {
    void cache.invalidateQueries({ queryKey: ['settings'] });
  }, [cache]);
  useEffect(() => {
    if (!view) return;
    setAppearance(view.preferences.appearance, true);
    void setAppLanguage(view.preferences.language);
  }, [view, setAppearance]);

  const isSettings = pathname === '/panel';
  useLayoutEffect(() => {
    if (!isSettings) preview(null);
    return () => preview(null);
  }, [isSettings, preview]);

  const accept = useCallback(async (result: PanelSettingsView) => {
    cache.setQueryData(queries.settings(api).queryKey, result);
    setAppearance(result.preferences.appearance);
    await setAppLanguage(result.preferences.language);
  }, [setAppearance, cache, api]);
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
