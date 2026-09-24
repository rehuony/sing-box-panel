import type { FormEvent, ReactNode } from 'react';

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { AppearanceSettings, PanelPreferences, PanelServiceSettings, PanelSettingsView } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { Switch } from '@/components/ui/switch';
import { useHashTab } from '@/hooks/use-hash-tab';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/components/ui/toast-manager';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { ServerPathInput } from '@/components/server-path-input';
import { usePanelSettings } from '@/stores/panel-settings.store';
import { DEFAULT_APPEARANCE, THEME_PRESETS } from '@/theme/appearance';
import { FieldGroup, FieldLegend, FieldSet } from '@/components/ui/field';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

import { SettingsField } from './settings-field';
import { PanelBackupSettings } from './panel-backup-settings';
import { AppearanceColorPicker } from './appearance-color-picker';
import { invalidSettingsField, resolveSettingsCategory, settingsCategories, settingsHashValues } from './settings-categories';
import './panel-settings-page.css';

function SettingsGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <FieldSet className='settings-group'>
      <FieldLegend>{title}</FieldLegend>
      <FieldGroup>{children}</FieldGroup>
    </FieldSet>
  );
}

function SettingsEditor({ initial }: { initial: PanelSettingsView }) {
  const { t } = useTranslation();
  const { preview, save, accept } = usePanelSettings();
  const [preferences, setPreferences] = useState(initial.preferences);
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
    setPreferences(view.preferences);
    setService(view.service);
    setGithub('');
    setClearGithub(false);
    setToken('');
    setTokenConfirm('');
    setInvalidField(null);
    await accept(view);
  }
  const tokenBytes = new TextEncoder().encode(token).length;
  const tokenError = tokenBytes < 8
    ? 'panelSettings.tokenTooShort'
    : tokenBytes > 8192
      ? 'panelSettings.tokenTooLong'
      : /^[\s\u0085]|[\s\u0085]$/u.test(token) || /[\0\r\n]/.test(token)
        ? 'panelSettings.tokenInvalid'
        : undefined;
  const tokenValid = tokenError === undefined;
  const colorValid = /^#[\dA-F]{6}$/i.test(preferences.appearance.color);
  const dirty = JSON.stringify(preferences) !== JSON.stringify(initial.preferences) || JSON.stringify(service) !== JSON.stringify(initial.service) || github !== '' || clearGithub;
  useUnsavedChanges(dirty || token !== '' || tokenConfirm !== '', () => {
    setPreferences(initial.preferences);
    setService(initial.service);
    setGithub('');
    setClearGithub(false);
    setToken('');
    setTokenConfirm('');
    setTokenOpen(false);
    preview(null);
  }, saving, { allowSamePathNavigation: true });

  useEffect(() => {
    if (colorValid) preview(preferences.appearance);
  }, [preferences.appearance, preview, colorValid]);
  useEffect(() => () => preview(null), [preview]);

  const update = <K extends keyof PanelPreferences>(key: K, value: PanelPreferences[K]) =>
    setPreferences(current => ({ ...current, [key]: value }));
  const updateService = <K extends keyof PanelServiceSettings>(key: K, value: PanelServiceSettings[K]) =>
    setService(current => ({ ...current, [key]: value }));
  const appearance = (value: Partial<AppearanceSettings>) =>
    setPreferences(current => ({ ...current, appearance: { ...current.appearance, ...value } }));

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
      setPreferences(result.preferences);
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

  return (
    <form className='panel-settings-card' noValidate onSubmit={submit}>
      <div className='settings-mobile-category'>
        <Select value={category} onValueChange={value => value && setCategory(value)}>
          <SelectTrigger aria-label={t('panelSettings.category')}><SelectValue>{t(`panelSettings.${category}`)}</SelectValue></SelectTrigger>
          <SelectContent>{settingsCategories.map(key => <SelectItem key={key} value={key}>{t(`panelSettings.${key}`)}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <Tabs className='panel-settings-tabs' orientation='vertical' value={category} onValueChange={value => setCategory(String(value))}>
        <TabsList aria-label={t('panelSettings.title')} variant='line'>
          {settingsCategories.map(key => <TabsTrigger value={key} key={key}>{t(`panelSettings.${key}`)}</TabsTrigger>)}
        </TabsList>
        <div className='panel-settings-content'>
          {invalidField && <ErrorNotice error={t('panelSettings.invalidField')} />}
          <TabsContent value='service'>
            <SettingsGroup title={t('panelSettings.access')}>
              <SettingsField id='listen-host' invalid={invalidField === 'listen-host'} label={t('panelSettings.listenHost')}>
                <Input id='listen-host' aria-invalid={invalidField === 'listen-host' || undefined} required value={preferences.listen_host} onChange={e => update('listen_host', e.target.value)} />
              </SettingsField>
              <SettingsField id='listen-port' invalid={invalidField === 'listen-port'} label={t('panelSettings.listenPort')}>
                <Input id='listen-port' aria-invalid={invalidField === 'listen-port' || undefined} type='number' min={1} max={65535} required value={preferences.listen_port} onChange={e => update('listen_port', Number(e.target.value))} />
              </SettingsField>
              <SettingsField id='origin' invalid={invalidField === 'origin'} label={t('panelSettings.origin')} help={t('panelSettings.originHelp')}>
                <Input id='origin' aria-invalid={invalidField === 'origin' || undefined} type='url' placeholder='https://panel.example.com' value={preferences.external_origin} onChange={e => {
                  update('external_origin', e.target.value);
                  updateService('secure_cookie', e.target.value.startsWith('https://'));
                }} />
              </SettingsField>
              <SettingsField id='base-path' invalid={invalidField === 'base-path'} label={t('panelSettings.basePath')} help={t('panelSettings.basePathHelp')}>
                <Input id='base-path' aria-invalid={invalidField === 'base-path' || undefined} value={service.base_path} onChange={e => updateService('base_path', e.target.value)} />
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.authentication')}>
              <SettingsField id='management-token' invalid={invalidField === 'management-token'} label={t('panelSettings.managementToken')} help={t('panelSettings.tokenHelp')}>
                <div className='settings-inline'>
                  <Input id='management-token' aria-invalid={invalidField === 'management-token' || undefined} aria-label={t('panelSettings.managementToken')} readOnly value='••••••••••••' />
                  <Button type='button' variant='ghost' onClick={() => setTokenOpen(true)}>{t('panelSettings.change')}</Button>
                </div>
              </SettingsField>
              <SettingsField id='secure-cookie' invalid={invalidField === 'secure-cookie'} label={t('panelSettings.secureCookie')} help={t('panelSettings.secureCookieHelp')}>
                <Switch id='secure-cookie' aria-invalid={invalidField === 'secure-cookie' || undefined} checked={service.secure_cookie} onCheckedChange={value => updateService('secure_cookie', value)} />
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.publication')}>
              <SettingsField id='public-host' invalid={invalidField === 'public-host'} label={t('panelSettings.publicHost')} help={t('panelSettings.publicHostHelp')}>
                <Input id='public-host' aria-invalid={invalidField === 'public-host' || undefined} value={preferences.public_node_host} placeholder={initial.detected_public_ip || t('panelSettings.autoHost')} onChange={e => update('public_node_host', e.target.value)} />
              </SettingsField>
            </SettingsGroup>
          </TabsContent>
          <TabsContent value='traffic'>
            <SettingsGroup title={t('panelSettings.traffic')}>
              <SettingsField id='quota' invalid={invalidField === 'quota'} label={t('panelSettings.quota')}>
                <div className='settings-inline'>
                  <Input id='quota' aria-invalid={invalidField === 'quota' || undefined} type='number' min={0} max={8589934591} step={1} placeholder={t('panelSettings.unlimited')} value={preferences.traffic_quota_gib ?? ''} onChange={e => update('traffic_quota_gib', e.target.value === '' ? null : Number(e.target.value))} />
                  <span>GiB</span>
                </div>
              </SettingsField>
              <SettingsField id='traffic-period' invalid={invalidField === 'traffic-period'} label={t('panelSettings.trafficPeriod')}>
                <Input id='traffic-period' aria-invalid={invalidField === 'traffic-period' || undefined} type='number' min={1} max={120} required value={service.traffic_period_months} onChange={e => updateService('traffic_period_months', Number(e.target.value))} />
              </SettingsField>
              <SettingsField id='sample-retention' invalid={invalidField === 'sample-retention'} label={t('panelSettings.sampleRetention')}>
                <Input id='sample-retention' aria-invalid={invalidField === 'sample-retention' || undefined} type='number' min={1} max={366} required value={service.sample_retention_days} onChange={e => updateService('sample_retention_days', Number(e.target.value))} />
              </SettingsField>
            </SettingsGroup>
          </TabsContent>
          <TabsContent value='logs'>
            <SettingsGroup title={t('panelSettings.coreLogs')}>
              <SettingsField id='core-log-retention' invalid={invalidField === 'core-log-retention'} label={t('panelSettings.coreLogRetention')} help={t('panelSettings.coreLogRetentionHelp')}>
                <Input id='core-log-retention' aria-invalid={invalidField === 'core-log-retention' || undefined} type='number' min={1} max={3650} required value={service.core_log_retention_days ?? 7} onChange={e => updateService('core_log_retention_days', Number(e.target.value))} />
              </SettingsField>
              <SettingsField id='core-log-max-files' invalid={invalidField === 'core-log-max-files'} label={t('panelSettings.coreLogMaxFiles')} help={t('panelSettings.coreLogMaxFilesHelp')}>
                <Input id='core-log-max-files' aria-invalid={invalidField === 'core-log-max-files' || undefined} type='number' min={0} max={1024} required value={service.core_log_max_files ?? 0} onChange={e => updateService('core_log_max_files', Number(e.target.value))} />
              </SettingsField>
              <SettingsField id='core-log-size' invalid={invalidField === 'core-log-size'} label={t('panelSettings.coreLogSize')}>
                <div className='settings-inline'>
                  <Input id='core-log-size' aria-invalid={invalidField === 'core-log-size' || undefined} type='number' min={1} max={1024} required value={service.core_log_max_file_size_mib ?? 32} onChange={e => updateService('core_log_max_file_size_mib', Number(e.target.value))} />
                  <span>MiB</span>
                </div>
              </SettingsField>
            </SettingsGroup>
          </TabsContent>
          <TabsContent value='interface'>
            <SettingsGroup title={t('panelSettings.appearanceGroup')}>
              <SettingsField id='theme' invalid={invalidField === 'theme'} label={t('panelSettings.theme')}>
                <Select value={preferences.appearance.theme} onValueChange={value => {
                  if (value) appearance({ theme: value });
                }}>
                  <SelectTrigger id='theme'><SelectValue>{t(`panelSettings.${preferences.appearance.theme}`)}</SelectValue></SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {(['light', 'dark', 'system'] as const).map(value => <SelectItem key={value} value={value}>{t(`panelSettings.${value}`)}</SelectItem>)}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </SettingsField>
              <SettingsField id='accent-color' label={t('panelSettings.color')} invalid={!colorValid}>
                <div className='settings-colors'>
                  <ToggleGroup spacing={0} aria-label={t('panelSettings.color')} value={[preferences.appearance.color]} onValueChange={values => {
                    if (values[0]) appearance({ color: values[0] });
                  }}>
                    {THEME_PRESETS.slice(0, 5).map((color, index) => <ToggleGroupItem key={color} value={color} aria-label={t(`panelSettings.colors.${index}`)}><span className='settings-color-swatch' style={{ background: color }} /></ToggleGroupItem>)}
                  </ToggleGroup>
                  <AppearanceColorPicker id='accent-color' value={preferences.appearance.color} onChange={color => appearance({ color })} />
                </div>
              </SettingsField>
              <SettingsField id='radius' invalid={invalidField === 'radius'} label={t('panelSettings.radius')} help={t('panelSettings.radiusHelp')}>
                <div className='settings-radius'>
                  <Slider aria-label={t('panelSettings.radius')} min={0} max={32} step={1} value={[preferences.appearance.radius]} onValueChange={value => appearance({ radius: Array.isArray(value) ? value[0]! : value })} />
                  <div className='settings-radius-value'>
                    <Input id='radius' aria-invalid={invalidField === 'radius' || undefined} type='number' min={0} max={32} step={1} required value={preferences.appearance.radius} onChange={e => appearance({ radius: Math.max(0, Math.min(32, Math.round(Number(e.target.value)))) })} />
                    <span aria-hidden='true'>px</span>
                  </div>
                  <Button className='settings-reset' type='button' variant='outline' onClick={() => appearance({ color: DEFAULT_APPEARANCE.color, radius: DEFAULT_APPEARANCE.radius })}>{t('panelSettings.reset')}</Button>
                </div>
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.languageGroup')}>
              <SettingsField id='language' invalid={invalidField === 'language'} label={t('panelSettings.language')}>
                <Select value={preferences.language} onValueChange={value => {
                  if (value) update('language', value);
                }}>
                  <SelectTrigger id='language'><SelectValue>{preferences.language === 'zh-CN' ? '简体中文' : 'English'}</SelectValue></SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value='zh-CN'>简体中文</SelectItem>
                      <SelectItem value='en'>English</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </SettingsField>
            </SettingsGroup>
          </TabsContent>
          <TabsContent value='maintenance'>
            <SettingsGroup title={t('panelSettings.storage')}>
              <SettingsField id='data-dir' invalid={invalidField === 'data-dir'} label={t('panelSettings.dataDir')} help={t('panelSettings.dataDirHelp')}>
                <ServerPathInput id='data-dir' mode='directory' aria-label={t('panelSettings.dataDir')}
                  aria-invalid={invalidField === 'data-dir' || undefined} required value={service.data_dir}
                  onValueChange={value => updateService('data_dir', value)} />
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.updates')}>
              <SettingsField id='github-token' invalid={invalidField === 'github-token'} label={t('panelSettings.github')} help={t('panelSettings.githubHelp')}>
                <div className='settings-inline'>
                  <Input id='github-token' aria-invalid={invalidField === 'github-token' || undefined} aria-description={initial.github_token_configured && !clearGithub ? t('panelSettings.configured') : undefined} className={initial.github_token_configured && !clearGithub ? 'placeholder:text-foreground' : undefined} type='password' autoComplete='new-password' maxLength={8192} disabled={clearGithub} value={github} placeholder={clearGithub ? t('panelSettings.removed') : initial.github_token_configured ? '••••••••••••' : undefined} onChange={e => setGithub(e.target.value)} />
                  {initial.github_token_configured && (
                    <Button type='button' variant='ghost' onClick={() => {
                      setClearGithub(!clearGithub);
                      setGithub('');
                    }}>
                      {t(clearGithub ? 'panelSettings.undo' : 'panelSettings.remove')}
                    </Button>
                  )}
                </div>
              </SettingsField>
              <SettingsField id='catalog-refresh-interval' invalid={invalidField === 'catalog-refresh-interval'} label={t('panelSettings.catalogRefreshInterval')}>
                <Input id='catalog-refresh-interval' aria-invalid={invalidField === 'catalog-refresh-interval' || undefined} type='number' min={1} max={720} required value={service.catalog_refresh_interval_hours} onChange={e => updateService('catalog_refresh_interval_hours', Number(e.target.value))} />
              </SettingsField>
            </SettingsGroup>
          </TabsContent>
          <TabsContent value='backup'><PanelBackupSettings dirty={dirty} busy={saving} onBusyChange={setSaving} onRestored={restored} /></TabsContent>
        </div>
      </Tabs>
      <footer className='panel-settings-footer'>
        {initial.restart_required && <p className='panel-settings-status' role='status'>{t('panelSettings.restartPending')}</p>}
        <Button disabled={saving || !dirty} type='submit'>{t(saving ? 'panelSettings.saving' : 'panelSettings.save')}</Button>
      </footer>
      <Dialog open={tokenOpen} onOpenChange={open => {
        setTokenOpen(open);
        if (!open) {
          setToken('');
          setTokenConfirm('');
        }
      }}>
        <DialogContent className='settings-token-dialog'>
          <DialogHeader><DialogTitle>{t('panelSettings.managementToken')}</DialogTitle></DialogHeader>
          <FieldGroup>
            <SettingsField id='new-token' label={t('panelSettings.newToken')} help={t('panelSettings.tokenHelp')} invalid={token !== '' && !tokenValid}>
              <Input id='new-token' type='password' autoComplete='new-password' value={token} aria-invalid={token !== '' && !tokenValid} aria-describedby={token !== '' && !tokenValid ? 'token-error' : undefined} onChange={e => setToken(e.target.value)} />
              {token !== '' && tokenError && <ErrorNotice id='token-error' error={t(tokenError)} />}
            </SettingsField>
            <SettingsField id='confirm-token' invalid={invalidField === 'confirm-token'} label={t('panelSettings.confirmToken')}><Input id='confirm-token' type='password' autoComplete='new-password' value={tokenConfirm} aria-invalid={tokenConfirm !== '' && token !== tokenConfirm} onChange={e => setTokenConfirm(e.target.value)} /></SettingsField>
          </FieldGroup>
          <DialogFooter>
            <Button type='button' variant='outline' onClick={() => {
              setTokenOpen(false);
              setToken('');
              setTokenConfirm('');
            }}>
              {t('panelSettings.cancel')}
            </Button>
            <Button type='button' disabled={saving || !tokenValid || token !== tokenConfirm} onClick={() => void submit(undefined, token)}>{t('panelSettings.save')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </form>
  );
}

export function PanelSettingsPage() {
  const { t } = useTranslation();
  const { view, error, reload } = usePanelSettings();
  return (
    <section className='panel-settings-page panel-page'>
      <h1 className='sr-only'>{t('panelSettings.title')}</h1>
      {error
        ? (
            <>
              <ErrorNotice error={error} />
              <Button onClick={reload}>{t('panelSettings.retry')}</Button>
            </>
          )
        : view ? <SettingsEditor initial={view} /> : <Skeleton className='panel-settings-card' />}
    </section>
  );
}
