import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Slider } from '@shared/components/ui/slider';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { ToggleGroup, ToggleGroupItem } from '@shared/components/ui/toggle-group';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';

import type { PanelSettingsView } from '@/api/api-client';

import { ErrorNotice } from '@/components/error-notice';
import { LoadingState } from '@/components/loading-state';
import { ShellNavigationTrigger } from '@/components/app-shell';
import { usePanelSettings } from '@/stores/panel-settings.store';
import { DEFAULT_APPEARANCE, THEME_PRESETS } from '@/theme/appearance';
import { SettingsField, SettingsGroup } from '@/components/settings-field';

import { settingsCategories } from './settings-categories';
import { PanelBackupSettings } from './panel-backup-settings';
import { AppearanceColorPicker } from './appearance-color-picker';
import { usePanelSettingsDraft } from './use-panel-settings-draft';

function SettingsEditor({ initial }: { initial: PanelSettingsView }) {
  const { t } = useTranslation();
  const {
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
  } = usePanelSettingsDraft(initial);
  return (
    <form className='flex min-h-0 flex-1 flex-col rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)] px-4 pt-4 pb-2 sm:px-6 sm:pt-6' noValidate onSubmit={submit}>
      <div className='mb-4 sm:hidden'>
        <Select value={category} onValueChange={value => value && setCategory(value)}>
          <SelectTrigger aria-label={t('panelSettings.category')} className='w-full'><SelectValue>{t(`panelSettings.${category}`)}</SelectValue></SelectTrigger>
          <SelectContent>{settingsCategories.map(key => <SelectItem key={key} value={key}>{t(`panelSettings.${key}`)}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <Tabs className='min-h-0 flex-1 gap-7 max-sm:flex-col max-sm:gap-4' orientation='vertical' value={category} onValueChange={value => setCategory(String(value))}>
        <TabsList aria-label={t('panelSettings.title')} className='h-full w-52 flex-none items-stretch justify-start gap-2 overflow-y-auto border-r pr-5 max-[1000px]:w-40 max-sm:hidden *:data-[slot=tabs-trigger]:min-h-(--control-height) *:data-[slot=tabs-trigger]:flex-none *:data-[slot=tabs-trigger]:justify-start *:data-[slot=tabs-trigger]:px-4 *:data-[slot=tabs-trigger]:py-2 *:data-[slot=tabs-trigger]:text-left *:data-[slot=tabs-trigger]:whitespace-normal *:data-[slot=tabs-trigger]:after:hidden *:data-[slot=tabs-trigger]:data-active:bg-accent *:data-[slot=tabs-trigger]:data-active:text-primary' variant='line'>
          {settingsCategories.map(key => <TabsTrigger value={key} key={key}>{t(`panelSettings.${key}`)}</TabsTrigger>)}
        </TabsList>
        <ScrollArea className='-mr-4 min-w-0 flex-1 sm:-mr-6' viewportClassName='pt-1 pr-5 pb-6 pl-1 sm:pr-7 *:data-[slot=tabs-content]:flex *:data-[slot=tabs-content]:flex-col *:data-[slot=tabs-content]:gap-5'>
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
                <Input id='origin' aria-invalid={invalidField === 'origin' || undefined} type='url' placeholder='https://panel.example.com' value={preferences.external_origin} onChange={e => update('external_origin', e.target.value)} />
              </SettingsField>
              <SettingsField id='base-path' invalid={invalidField === 'base-path'} label={t('panelSettings.basePath')} help={t('panelSettings.basePathHelp')}>
                <Input id='base-path' aria-invalid={invalidField === 'base-path' || undefined} value={service.base_path} onChange={e => updateService('base_path', e.target.value)} />
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.authentication')}>
              <SettingsField id='admin-email' label={t('panelSettings.adminEmail')} invalid={invalidField === 'admin-email'}>
                <Input id='admin-email' type='text' inputMode='email' autoComplete='username' value={email} aria-invalid={invalidField === 'admin-email' || undefined} onChange={e => setEmail(e.target.value)} />
              </SettingsField>
              <SettingsField id='new-password' label={t('panelSettings.password')} help={t('panelSettings.passwordHelp')} invalid={!passwordValid}>
                <Input id='new-password' type='password' autoComplete='new-password' value={password} placeholder={t('panelSettings.passwordPlaceholder')} aria-invalid={!passwordValid || undefined} aria-describedby={!passwordValid ? 'password-error' : undefined} onChange={e => setPassword(e.target.value)} />
                {newPasswordError && <ErrorNotice id='password-error' error={t(newPasswordError)} />}
              </SettingsField>
            </SettingsGroup>
          </TabsContent>
          <TabsContent value='traffic'>
            <SettingsGroup title={t('panelSettings.traffic')}>
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
                <div className='flex items-center gap-3 *:data-[slot=input]:min-w-0 *:data-[slot=input]:flex-1'>
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
                <div className='@container flex min-h-10 items-center gap-2'>
                  <ToggleGroup className='min-w-24 flex-1 justify-between @max-[250px]:min-w-18' spacing={0} aria-label={t('panelSettings.color')} value={[preferences.appearance.color]} onValueChange={values => {
                    if (values[0]) appearance({ color: values[0] });
                  }}>
                    {THEME_PRESETS.slice(0, 5).map((color, index) => <ToggleGroupItem className='relative h-10 w-8 min-w-6 shrink bg-transparent p-0 aria-pressed:bg-transparent aria-pressed:before:absolute aria-pressed:before:size-7 aria-pressed:before:rounded-[var(--radius-control)] aria-pressed:before:border aria-pressed:before:border-primary aria-pressed:before:content-[""] @max-[250px]:nth-[n+4]:hidden' key={color} value={color} aria-label={t(`panelSettings.colors.${index}`)}><span className='block size-5 rounded-full' style={{ background: color }} /></ToggleGroupItem>)}
                  </ToggleGroup>
                  <AppearanceColorPicker id='accent-color' value={preferences.appearance.color} onChange={color => appearance({ color })} />
                </div>
              </SettingsField>
              <SettingsField id='radius' invalid={invalidField === 'radius'} label={t('panelSettings.radius')} help={t('panelSettings.radiusHelp')}>
                <div className='grid grid-cols-[minmax(24px,1fr)_56px_auto] items-center gap-1.5 min-[361px]:grid-cols-[minmax(40px,1fr)_56px_auto] min-[361px]:gap-2'>
                  <Slider aria-label={t('panelSettings.radius')} min={0} max={32} step={1} value={[preferences.appearance.radius]} onValueChange={value => appearance({ radius: Array.isArray(value) ? value[0]! : value })} />
                  <div className='flex h-(--control-height) items-center gap-1 rounded-[var(--radius-control)] border border-input bg-transparent pr-1.5 focus-within:border-[var(--input-focus)] dark:bg-[color-mix(in_oklab,var(--input)_30%,transparent)]'>
                    <Input id='radius' className='h-full min-h-0 [appearance:textfield] border-0 bg-transparent py-0 pr-0 pl-1.5 text-right tabular-nums shadow-none outline-none focus-visible:outline-none dark:bg-transparent [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none' aria-invalid={invalidField === 'radius' || undefined} type='number' min={0} max={32} step={1} required value={preferences.appearance.radius} onChange={e => appearance({ radius: Math.max(0, Math.min(32, Math.round(Number(e.target.value)))) })} />
                    <span aria-hidden='true' className='text-sm text-muted-foreground'>px</span>
                  </div>
                  <Button type='button' variant='outline' onClick={() => appearance({ color: DEFAULT_APPEARANCE.color, radius: DEFAULT_APPEARANCE.radius })}>{t('panelSettings.reset')}</Button>
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
                {/* The panel host has no browse API; the Browse control picks paths on a node. */}
                <Input id='data-dir' aria-invalid={invalidField === 'data-dir' || undefined} required autoComplete='off'
                  spellCheck={false} value={service.data_dir} onChange={e => updateService('data_dir', e.target.value)} />
              </SettingsField>
            </SettingsGroup>
            <SettingsGroup title={t('panelSettings.updates')}>
              <SettingsField id='github-token' invalid={invalidField === 'github-token'} label={t('panelSettings.github')} help={t('panelSettings.githubHelp')}>
                <div className='flex items-center gap-3 *:data-[slot=input]:min-w-0 *:data-[slot=input]:flex-1'>
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
          <TabsContent value='backup'><PanelBackupSettings busy={saving} onBusyChange={setSaving} onRestored={restored} /></TabsContent>
        </ScrollArea>
      </Tabs>
      <footer className='mt-2 flex flex-none flex-wrap items-center justify-end gap-x-3 gap-y-2 border-t pt-2'>
        {initial.restart_required && <p className='mr-auto min-w-0 flex-1 text-sm text-muted-foreground wrap-anywhere max-sm:basis-full' role='status'>{t('panelSettings.restartPending')}</p>}
        <Button disabled={saving || !dirty} type='submit'>{t(saving ? 'panelSettings.saving' : 'panelSettings.save')}</Button>
      </footer>
    </form>
  );
}

export function PanelSettingsPage() {
  const { t } = useTranslation();
  const { view, error, reload } = usePanelSettings();
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-4'>
      <h1 className='sr-only'>{t('panelSettings.title')}</h1>
      <div className='flex md:hidden'><ShellNavigationTrigger /></div>
      {error
        ? (
            <>
              <ErrorNotice error={error} />
              <Button onClick={reload}>{t('panelSettings.retry')}</Button>
            </>
          )
        : view ? <SettingsEditor initial={view} /> : <LoadingState label={t('common.loading')} fullScreen={false} />}
    </section>
  );
}
