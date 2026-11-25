import type { FormEvent } from 'react';

import { useTranslation } from 'react-i18next';
import { useLayoutEffect, useState } from 'react';
import { toast } from '@shared/components/ui/toast-manager';

import type { AppearanceSettings, PanelPreferences, PanelServiceSettings, PanelSettingsView } from '@/api/api-client';

import { useTheme } from '@/theme/theme-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { usePanelSettings } from '@/stores/panel-settings.store';

import { invalidSettingsField, passwordError } from './settings-validation';

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
  const [saving, setSaving] = useState(false);
  const [invalidField, setInvalidField] = useState<string | null>(null);
  const [password, setPassword] = useState('');
  const [passwordOpen, setPasswordOpen] = useState(false);
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
    setPasswordOpen(false);
    setInvalidField(null);
    await accept(view);
  }
  const newPasswordError = passwordError(password);
  const passwordValid = newPasswordError === undefined;
  const colorValid = /^#[\dA-F]{6}$/i.test(preferences.appearance.color);
  const is = initial.service;
  // The rows whose value differs from the saved settings, by their field's id.
  const changed = new Set(Object.entries({
    'listen-host': preferencesDraft.listen_host !== initialPreferences.listen_host || preferencesDraft.listen_port !== initialPreferences.listen_port,
    'origin': preferencesDraft.external_origin !== initialPreferences.external_origin,
    'base-path': service.base_path !== is.base_path,
    'data-dir': service.data_dir !== is.data_dir,
    'admin-email': email !== initial.admin_email,
    'new-password': password !== '',
    'theme': activeAppearance.theme !== initialAppearance.theme,
    'accent-color': activeAppearance.color !== initialAppearance.color,
    'radius': activeAppearance.radius !== initialAppearance.radius,
    'language': preferencesDraft.language !== initialPreferences.language,
    'sample-retention': service.sample_retention_days !== is.sample_retention_days,
    'runtime-history-retention': service.runtime_history_retention_days !== is.runtime_history_retention_days,
    'core-log-retention': service.core_log_retention_days !== is.core_log_retention_days,
    'core-log-max-files': service.core_log_max_files !== is.core_log_max_files,
    'core-log-size': service.core_log_max_file_size_mib !== is.core_log_max_file_size_mib,
    'github-token': github !== '' || clearGithub,
    'catalog-refresh-interval': service.catalog_refresh_interval_hours !== is.catalog_refresh_interval_hours,
  }).filter(([, differs]) => differs).map(([field]) => field));
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
  /** Puts every field back to the saved settings. */
  function revert() {
    setEmail(initial.admin_email);
    setPreferencesDraft(initialPreferences);
    setService(initial.service);
    setGithub('');
    setClearGithub(false);
    setPassword('');
    setPasswordOpen(false);
    setInvalidField(null);
    preview(null);
  }
  useUnsavedChanges(dirty, revert, saving, { allowSamePathNavigation: true });

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
    setInvalidField(invalid);
    if (invalid) {
      document.getElementById(invalid)?.focus();
      toast.add({ title: t(invalid === 'new-password' ? 'panelSettings.passwordLength' : 'panelSettings.invalidField'), type: 'error' });
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
      setPasswordOpen(false);
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
    changed,
    saving,
    invalidField,
    password,
    newPasswordError,
    passwordValid,
    colorValid,
    dirty,
    setSaving,
    setPassword,
    passwordOpen,
    openPassword: () => setPasswordOpen(true),
    closePassword: () => {
      setPassword('');
      setPasswordOpen(false);
    },
    revert,
    setGithub,
    setClearGithub,
    restored,
    update,
    updateService,
    appearance,
    submit,
  };
}
