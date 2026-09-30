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

import { invalidSettingsField, passwordError, resolveSettingsCategory, settingsHashValues } from './settings-categories';

export function usePanelSettingsDraft(incoming: PanelSettingsView) {
  const [initial, setInitial] = useState(incoming);
  const [observedIncoming, setObservedIncoming] = useState(incoming);
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
  const [password, setPassword] = useState('');
  const [email, setEmail] = useState(initial.admin_email);
  async function restored(view: PanelSettingsView) {
    setInitial(view);
    setEmail(view.admin_email);
    const { appearance: _appearance, ...restoredPreferences } = view.preferences;
    setPreferencesDraft(restoredPreferences);
    setService(view.service);
    setGithub('');
    setClearGithub(false);
    setPassword('');
    setInvalidField(null);
    await accept(view);
  }
  const newPasswordError = passwordError(password);
  const passwordValid = newPasswordError === undefined;
  const colorValid = /^#[\dA-F]{6}$/i.test(preferences.appearance.color);
  const dirty = JSON.stringify(preferencesDraft) !== JSON.stringify(initialPreferences)
    || JSON.stringify(activeAppearance) !== JSON.stringify(initialAppearance)
    || JSON.stringify(service) !== JSON.stringify(initial.service) || github !== '' || clearGithub
    || email !== initial.admin_email || password !== '';
  if (!dirty && !saving && incoming !== observedIncoming) {
    setObservedIncoming(incoming);
    if (incoming.revision !== initial.revision) {
      setInitial(incoming);
      setEmail(incoming.admin_email);
      const { appearance: _appearance, ...nextPreferences } = incoming.preferences;
      setPreferencesDraft(nextPreferences);
      setService(incoming.service);
    }
  }
  useUnsavedChanges(dirty, () => {
    setEmail(initial.admin_email);
    setPreferencesDraft(initialPreferences);
    setService(initial.service);
    setGithub('');
    setClearGithub(false);
    setPassword('');
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

  async function submit(event?: FormEvent) {
    event?.preventDefault();
    if (saving) return;
    const invalid = invalidSettingsField(preferences, service, github, email, password);
    setInvalidField(invalid?.field ?? null);
    if (invalid) {
      setCategory(invalid.category);
      return;
    }
    setSaving(true);
    try {
      const saved = await save({
        revision: initial.revision, preferences, github_token: github, clear_github_token: clearGithub,
        service,
        credentials: email !== initial.admin_email || password !== ''
          ? {
              ...(email !== initial.admin_email && { email: email.trim().toLowerCase() }),
              ...(password !== '' && { new_password: password }),
            }
          : undefined,
      });
      setPassword('');
      if (saved.reauthentication_required) {
        toast.add({ title: t('panelSettings.credentialsChanged'), type: 'success' });
        return;
      }
      const result = saved.settings;
      setInitial(result);
      setEmail(result.admin_email);
      const { appearance: _appearance, ...savedPreferences } = result.preferences;
      setPreferencesDraft(savedPreferences);
      setService(result.service);
      setGithub('');
      setClearGithub(false);
      toast.add({ title: t(result.restart_required ? 'panelSettings.restart' : 'panelSettings.saved'), type: 'success' });
    } catch (reason) {
      if (reason instanceof DOMException && reason.name === 'AbortError') return;
      toast.add({ title: t('panelSettings.failed'), description: describeRequestError(reason), type: 'error' });
    } finally {
      setSaving(false);
    }
  }

  return {
    email, setEmail,
    preferences,
    service,
    github,
    clearGithub,
    category,
    saving,
    invalidField,
    password,
    newPasswordError,
    passwordValid,
    colorValid,
    dirty,
    setCategory,
    setSaving,
    setPassword,
    setGithub,
    setClearGithub,
    restored,
    update,
    updateService,
    appearance,
    submit,
  };
}
