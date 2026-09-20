import type { FormEvent, ReactNode } from 'react';

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { AppearanceSettings, PanelPreferences, PanelSettingsView } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from '@/components/ui/toast-manager';
import { usePanelSettings } from '@/stores/panel-settings.store';
import { DEFAULT_APPEARANCE, THEME_PRESETS } from '@/theme/appearance';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { FieldError, FieldGroup, FieldLegend, FieldSet } from '@/components/ui/field';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

import { SettingsField } from './settings-field';
import { AppearanceColorPicker } from './appearance-color-picker';
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
  const { preview, save } = usePanelSettings();
  const [preferences, setPreferences] = useState(initial.preferences);
  const [github, setGithub] = useState('');
  const [clearGithub, setClearGithub] = useState(false);
  const [identityKey, setIdentityKey] = useState('');
  const [category, setCategory] = useState('security');
  const [saving, setSaving] = useState(false);
  const [tokenOpen, setTokenOpen] = useState(false);
  const [token, setToken] = useState('');
  const [tokenConfirm, setTokenConfirm] = useState('');
  const tokenBytes = new TextEncoder().encode(token).length;
  const tokenValid = tokenBytes >= 32 && tokenBytes <= 8192
    && !/^[\s\u0085]|[\s\u0085]$/u.test(token)
    && !token.includes('\0') && !token.includes('\r') && !token.includes('\n');
  const colorValid = /^#[\dA-F]{6}$/i.test(preferences.appearance.color);
  const dirty = JSON.stringify(preferences) !== JSON.stringify(initial.preferences) || github !== '' || identityKey !== '' || clearGithub;

  useEffect(() => {
    if (colorValid) preview(preferences.appearance);
  }, [preferences.appearance, preview, colorValid]);
  useEffect(() => () => preview(null), [preview]);

  const update = <K extends keyof PanelPreferences>(key: K, value: PanelPreferences[K]) =>
    setPreferences(current => ({ ...current, [key]: value }));
  const appearance = (value: Partial<AppearanceSettings>) =>
    setPreferences(current => ({ ...current, appearance: { ...current.appearance, ...value } }));

  async function submit(event?: FormEvent, managementToken?: string) {
    event?.preventDefault();
    if (!colorValid || saving || (managementToken !== undefined && !tokenValid)) return;
    setSaving(true);
    const identityChanged = identityKey !== '' || preferences.identity_name !== initial.preferences.identity_name;
    try {
      const result = await save({
        revision: initial.revision, preferences, github_token: github, clear_github_token: clearGithub,
        identity_key: identityKey, management_token: managementToken,
      });
      setPreferences(result.preferences);
      setGithub('');
      setIdentityKey('');
      setClearGithub(false);
      setToken('');
      setTokenConfirm('');
      setTokenOpen(false);
      toast.add({ title: t(managementToken ? 'panelSettings.tokenChanged' : result.restart_required ? 'panelSettings.restart' : identityChanged ? 'panelSettings.identitySaved' : 'panelSettings.saved'), type: 'success' });
    } catch (reason) {
      toast.add({ title: t('panelSettings.failed'), description: describeRequestError(reason), type: 'error', timeout: 0 });
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className='panel-settings-card' onSubmit={submit}>
      <Tabs className='panel-settings-tabs' orientation='vertical' value={category} onValueChange={value => setCategory(String(value))}>
        <TabsList aria-label={t('panelSettings.title')} variant='line'>
          {(['security', 'nodes', 'appearance'] as const).map(key => <TabsTrigger value={key} key={key}>{t(`panelSettings.${key}`)}</TabsTrigger>)}
        </TabsList>
        <div className='panel-settings-content'>
          <TabsContent value='security'>
            <SettingsGroup title={t('panelSettings.access')}>
              <SettingsField id='listen-host' label={t('panelSettings.listenHost')}>
                <Input id='listen-host' required value={preferences.listen_host} onChange={e => update('listen_host', e.target.value)} />
              </SettingsField>
              <SettingsField id='listen-port' label={t('panelSettings.listenPort')}>
                <Input id='listen-port' type='number' min={1} max={65535} required value={preferences.listen_port} onChange={e => update('listen_port', Number(e.target.value))} />
              </SettingsField>
              <SettingsField id='origin' label={t('panelSettings.origin')} help={t('panelSettings.originHelp')}>
                <Input id='origin' type='url' placeholder='https://panel.example.com' value={preferences.external_origin} onChange={e => update('external_origin', e.target.value)} />
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.authentication')}>
              <SettingsField id='management-token' label={t('panelSettings.managementToken')} help={t('panelSettings.tokenHelp')}>
                <div className='settings-inline'>
                  <Input id='management-token' aria-label={t('panelSettings.managementToken')} readOnly value='••••••••••••' />
                  <Button type='button' variant='ghost' onClick={() => setTokenOpen(true)}>{t('panelSettings.change')}</Button>
                </div>
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.updates')}>
              <SettingsField id='github-token' label={t('panelSettings.github')} help={t('panelSettings.githubHelp')}>
                <div className='settings-inline'>
                  <Input id='github-token' type='password' autoComplete='new-password' maxLength={8192} disabled={clearGithub} value={github} placeholder={t(clearGithub ? 'panelSettings.removed' : initial.github_token_configured ? 'panelSettings.configured' : 'panelSettings.optional')} onChange={e => setGithub(e.target.value)} />
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
            </SettingsGroup>
          </TabsContent>
          <TabsContent value='nodes'>
            <SettingsGroup title={t('panelSettings.publication')}>
              <SettingsField id='public-host' label={t('panelSettings.publicHost')} help={t('panelSettings.publicHostHelp')}>
                <Input id='public-host' value={preferences.public_node_host} placeholder={initial.detected_public_ip || t('panelSettings.autoHost')} onChange={e => update('public_node_host', e.target.value)} />
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.identity')}>
              <SettingsField id='identity-name' label={t('panelSettings.identityName')} help={t('panelSettings.identityHelp')}>
                <Input id='identity-name' maxLength={128} value={preferences.identity_name} onChange={e => update('identity_name', e.target.value)} />
              </SettingsField>
              <SettingsField id='identity-key' label={t('panelSettings.identityKey')}>
                <Input id='identity-key' type='password' autoComplete='new-password' maxLength={8192} value={identityKey} placeholder={t(initial.identity_key_configured ? 'panelSettings.configured' : 'panelSettings.optional')} onChange={e => setIdentityKey(e.target.value)} />
              </SettingsField>
            </SettingsGroup>
          </TabsContent>
          <TabsContent value='appearance'>
            <SettingsGroup title={t('panelSettings.traffic')}>
              <SettingsField id='quota' label={t('panelSettings.quota')}>
                <div className='settings-inline'>
                  <Input id='quota' type='number' min={0} max={8589934591} step={1} placeholder={t('panelSettings.unlimited')} value={preferences.traffic_quota_gib ?? ''} onChange={e => update('traffic_quota_gib', e.target.value === '' ? null : Number(e.target.value))} />
                  <span>GiB</span>
                </div>
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.interface')}>
              <SettingsField id='theme' label={t('panelSettings.theme')}>
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
              <SettingsField id='radius' label={t('panelSettings.radius')} help={t('panelSettings.radiusHelp')}>
                <div className='settings-radius'>
                  <Slider aria-label={t('panelSettings.radius')} min={0} max={32} step={1} value={[preferences.appearance.radius]} onValueChange={value => appearance({ radius: Array.isArray(value) ? value[0]! : value })} />
                  <div className='settings-radius-value'>
                    <Input id='radius' type='number' min={0} max={32} step={1} required value={preferences.appearance.radius} onChange={e => appearance({ radius: Math.max(0, Math.min(32, Math.round(Number(e.target.value)))) })} />
                    <span aria-hidden='true'>px</span>
                  </div>
                  <Button className='settings-reset' type='button' variant='secondary' onClick={() => appearance({ color: DEFAULT_APPEARANCE.color, radius: DEFAULT_APPEARANCE.radius })}>{t('panelSettings.reset')}</Button>
                </div>
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.languageGroup')}>
              <SettingsField id='language' label={t('panelSettings.language')}>
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
        </div>
      </Tabs>
      <footer className='panel-settings-footer'><Button disabled={saving || !colorValid || !dirty} type='submit'>{t(saving ? 'panelSettings.saving' : 'panelSettings.save')}</Button></footer>
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
              {token !== '' && !tokenValid && <FieldError id='token-error'>{t('panelSettings.tokenInvalid')}</FieldError>}
            </SettingsField>
            <SettingsField id='confirm-token' label={t('panelSettings.confirmToken')}><Input id='confirm-token' type='password' autoComplete='new-password' value={tokenConfirm} aria-invalid={tokenConfirm !== '' && token !== tokenConfirm} onChange={e => setTokenConfirm(e.target.value)} /></SettingsField>
          </FieldGroup>
          <DialogFooter>
            <Button type='button' variant='secondary' onClick={() => {
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
      <h1 className='panel-page-heading'>{t('panelSettings.title')}</h1>
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
