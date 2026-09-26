import type { FormEvent } from 'react';

import { useTranslation } from 'react-i18next';
import { useEffect, useLayoutEffect, useState } from 'react';

import type { AppearanceSettings, PanelPreferences, PanelServiceSettings, PanelSettingsView } from '@/api/api-client';

import { useTheme } from '@/theme/theme-context';
import { useHashTab } from '@/hooks/use-hash-tab';
import { toast } from '@/components/ui/toast-manager';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { usePanelSettings } from '@/stores/panel-settings.store';

import { invalidSettingsField, managementTokenError, resolveSettingsCategory, settingsHashValues } from './settings-categories';

export function usePanelSettingsDraft(initial: PanelSettingsView) {
  const { t } = useTranslation();
  const { preview, save, accept } = usePanelSettings();
  const { appearance: activeAppearance } = useTheme();
  const { appearance: initialAppearance, ...initialPreferences } = initial.preferences;
  const [preferencesDraft, setPreferencesDraft] = useState(initialPreferences);
  const preferences = { ...preferencesDraft, appearance: activeAppearance };
  const [service, setService] = useState(initial.service);
  const [github, setGithub] = useState('');
  const [clearGithub, setClearGithub] = useState(false);
  const [categoryHash, setCategory] = useHashTab('panel-', settingsHashValues, 'service');
  const category = resolveSettingsCategory(categoryHash);
  const [saving, setSaving] = useState(false);
  const [invalidField, setInvalidField] = useState<string | null>(null);
  useEffect(() => {
    if (invalidField) document.getElementById(invalidField)?.focus();
  }, [invalidField, category]);
  const [tokenOpen, setTokenOpen] = useState(false);
  const [token, setToken] = useState('');
  const [tokenConfirm, setTokenConfirm] = useState('');
  async function restored(view: PanelSettingsView) {
    const { appearance: _appearance, ...restoredPreferences } = view.preferences;
    setPreferencesDraft(restoredPreferences);
    setService(view.service);
    setGithub('');
    setClearGithub(false);
    setToken('');
    setTokenConfirm('');
    setInvalidField(null);
    await accept(view);
  }
  const tokenError = managementTokenError(token);
  const tokenValid = tokenError === undefined;
  const colorValid = /^#[\dA-F]{6}$/i.test(preferences.appearance.color);
  const dirty = JSON.stringify(preferencesDraft) !== JSON.stringify(initialPreferences)
    || JSON.stringify(activeAppearance) !== JSON.stringify(initialAppearance)
    || JSON.stringify(service) !== JSON.stringify(initial.service) || github !== '' || clearGithub;
  useUnsavedChanges(dirty || token !== '' || tokenConfirm !== '', () => {
    setPreferencesDraft(initialPreferences);
    setService(initial.service);
    setGithub('');
    setClearGithub(false);
    setToken('');
    setTokenConfirm('');
    setTokenOpen(false);
    preview(null);
  }, saving, { allowSamePathNavigation: true });

  useLayoutEffect(() => {
    preview({});
    return () => preview(null);
  }, [preview]);

  const update = <K extends keyof typeof preferencesDraft>(key: K, value: PanelPreferences[K]) =>
    setPreferencesDraft(current => ({ ...current, [key]: value }));
  const updateService = <K extends keyof PanelServiceSettings>(key: K, value: PanelServiceSettings[K]) =>
    setService(current => ({ ...current, [key]: value }));
  const appearance = (value: Partial<AppearanceSettings>) =>
    preview(value);

  async function submit(event?: FormEvent, managementToken?: string) {
    event?.preventDefault();
    if (saving || (managementToken !== undefined && !tokenValid)) return;
    const invalid = invalidSettingsField(preferences, service, github);
    setInvalidField(invalid?.field ?? null);
    if (invalid) {
      setCategory(invalid.category);
      setTokenOpen(false);
      return;
    }
    setSaving(true);
    try {
      const result = await save({
        revision: initial.revision, preferences, github_token: github, clear_github_token: clearGithub,
        service,
        management_token: managementToken,
      });
      const { appearance: _appearance, ...savedPreferences } = result.preferences;
      setPreferencesDraft(savedPreferences);
      setService(result.service);
      setGithub('');
      setClearGithub(false);
      setToken('');
      setTokenConfirm('');
      setTokenOpen(false);
      toast.add({ title: t(managementToken ? 'panelSettings.tokenChanged' : result.restart_required ? 'panelSettings.restart' : 'panelSettings.saved'), type: 'success' });
    } catch (reason) {
      toast.add({ title: t('panelSettings.failed'), description: describeRequestError(reason), type: 'error' });
    } finally {
      setSaving(false);
    }
  }

  return {
    preferences,
    service,
    github,
    clearGithub,
    category,
    saving,
    invalidField,
    tokenOpen,
    token,
    tokenConfirm,
    tokenError,
    tokenValid,
    colorValid,
    dirty,
    setCategory,
    setSaving,
    setTokenOpen,
    setToken,
    setTokenConfirm,
    setGithub,
    setClearGithub,
    restored,
    update,
    updateService,
    appearance,
    submit,
  };
}
