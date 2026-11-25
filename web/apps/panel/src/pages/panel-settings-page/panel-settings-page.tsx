import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Slider } from '@shared/components/ui/slider';
import { RotateCcw, Save, Undo2 } from 'lucide-react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { ToggleGroup, ToggleGroupItem } from '@shared/components/ui/toggle-group';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';

import type { PanelSettingsView } from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { ErrorNotice } from '@/components/error-notice';
import { LoadingState } from '@/components/loading-state';
import { usePanelSettings } from '@/stores/panel-settings.store';
import { StripDivider, TitleStrip } from '@/components/title-strip';
import { DEFAULT_APPEARANCE, THEME_PRESETS } from '@/theme/appearance';
import { SettingsCard, SettingsField } from '@/components/settings-field';

import { PanelBackupSettings } from './panel-backup-settings';
import { PanelUpdateSettings } from './panel-update-settings';
import { AppearanceColorPicker } from './appearance-color-picker';
import { usePanelSettingsDraft } from './use-panel-settings-draft';

/** A whole number with its unit at the end of the field. */
function UnitInput({ id, unit, value, min, max, invalid, className, onChange }: {
  id: string;
  unit: string;
  value: number;
  min: number;
  max: number;
  invalid?: boolean;
  className?: string;
  onChange: (value: number) => void;
}) {
  return (
    <InputGroup className={className}>
      <InputGroupInput aria-invalid={invalid || undefined} className='tabular-nums' id={id} inputMode='numeric' max={max} min={min} onChange={event => onChange(Number(event.target.value))} required type='number' value={value} />
      <InputGroupAddon align='inline-end'>{unit}</InputGroupAddon>
    </InputGroup>
  );
}

function SettingsEditor({ initial }: { initial: PanelSettingsView }) {
  const { t } = useTranslation();
  const {
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
    openPassword,
    closePassword,
    revert,
    setGithub,
    setClearGithub,
    restored,
    update,
    updateService,
    appearance,
    submit,
  } = usePanelSettingsDraft(initial);
  const input = (id: string) => ({ id, invalid: invalidField === id });
  const field = (id: string) => ({ ...input(id), changed: changed.has(id) });
  const days = t('panelSettings.days');
  return (
    <form className='flex min-h-0 flex-1 flex-col gap-[var(--workspace-content-gap)]' noValidate onSubmit={submit}>
      <TitleStrip>
        <h1 className='me-auto min-w-0 truncate text-sm font-medium'>{t('panelSettings.title')}</h1>
        <span className='text-compact text-[var(--color-text-faint)] tabular-nums' role='status'>
          {changed.size > 0 ? t('panelSettings.unsaved', { count: changed.size }) : null}
        </span>
        {changed.size > 0 && (
          <IconAction disabled={saving} label={t('panelSettings.revert')} onClick={revert} variant='flat'>
            <Undo2 aria-hidden='true' />
          </IconAction>
        )}
        <StripDivider />
        <IconAction disabled={saving || !dirty} label={t(saving ? 'panelSettings.saving' : 'panelSettings.save')} onClick={() => void submit()} variant='flat'>
          <Save aria-hidden='true' />
        </IconAction>
      </TitleStrip>
      <ScrollArea className='min-h-0 min-w-0 flex-1'>
        <div className='grid items-start gap-3 @5xl:grid-cols-2'>
          <div className='flex min-w-0 flex-col gap-3'>
            <SettingsCard
              mark={(
                <span className={initial.restart_required
                  ? 'me-2 rounded-[var(--radius-pill)] border border-[var(--color-warning-border)] bg-[var(--color-warning-soft)] px-1.5 text-2xs leading-4 whitespace-nowrap text-[var(--color-warning-text)]'
                  : 'me-2 rounded-[var(--radius-pill)] border border-[var(--color-border-strong)] px-1.5 text-2xs leading-4 whitespace-nowrap text-[var(--color-text-secondary)]'}
                >
                  {t(initial.restart_required ? 'panelSettings.restartPending' : 'panelSettings.restartRequired')}
                </span>
              )}
              title={t('panelSettings.access')}
            >
              <SettingsField {...field('listen-host')} invalid={invalidField === 'listen-host' || invalidField === 'listen-port'} label={t('panelSettings.listenHost')}>
                <div className='flex items-center gap-2'>
                  <Input aria-invalid={invalidField === 'listen-host' || undefined} className='min-w-0 flex-1' id='listen-host' required spellCheck={false} value={preferences.listen_host} onChange={e => update('listen_host', e.target.value)} />
                  <span aria-hidden='true' className='text-[var(--color-text-faint)]'>:</span>
                  <Input aria-invalid={invalidField === 'listen-port' || undefined} aria-label={t('panelSettings.listenPort')} className='w-22 flex-none tabular-nums' id='listen-port' max={65535} min={1} required type='number' value={preferences.listen_port} onChange={e => update('listen_port', Number(e.target.value))} />
                </div>
              </SettingsField>
              <SettingsField {...field('origin')} help={t('panelSettings.originHelp')} label={t('panelSettings.origin')}>
                <Input aria-invalid={invalidField === 'origin' || undefined} id='origin' placeholder='https://panel.example.com' spellCheck={false} type='url' value={preferences.external_origin} onChange={e => update('external_origin', e.target.value)} />
              </SettingsField>
              <SettingsField {...field('base-path')} help={t('panelSettings.basePathHelp')} label={t('panelSettings.basePath')}>
                <Input aria-invalid={invalidField === 'base-path' || undefined} id='base-path' placeholder='/' spellCheck={false} value={service.base_path} onChange={e => updateService('base_path', e.target.value)} />
              </SettingsField>
              <SettingsField {...field('data-dir')} help={t('panelSettings.dataDirHelp')} label={t('panelSettings.dataDir')}>
                {/* The panel host has no browse API; the Browse control picks paths on a node. */}
                <Input aria-invalid={invalidField === 'data-dir' || undefined} autoComplete='off' id='data-dir' required spellCheck={false} value={service.data_dir} onChange={e => updateService('data_dir', e.target.value)} />
              </SettingsField>
            </SettingsCard>

            <SettingsCard title={t('panelSettings.account')}>
              <SettingsField {...field('admin-email')} label={t('panelSettings.adminEmail')}>
                <Input aria-invalid={invalidField === 'admin-email' || undefined} autoComplete='username' id='admin-email' inputMode='email' type='text' value={email} onChange={e => setEmail(e.target.value)} />
              </SettingsField>
              <SettingsField {...field('new-password')} help={t('panelSettings.passwordHelp')} invalid={!passwordValid} label={t('panelSettings.password')}>
                {passwordOpen
                  ? (
                      <div className='flex items-center gap-2'>
                        <Input aria-describedby={!passwordValid ? 'password-error' : undefined} aria-invalid={!passwordValid || undefined} autoComplete='new-password' autoFocus className='min-w-0 flex-1' id='new-password' type='password' value={password} onChange={e => setPassword(e.target.value)} />
                        <Button onClick={closePassword} size='sm' type='button' variant='ghost'>{t('common.cancel')}</Button>
                      </div>
                    )
                  : (
                      <div className='flex items-center gap-2'>
                        <span aria-hidden='true' className='min-w-0 flex-1 truncate font-[family-name:var(--font-data)] text-[var(--color-text-faint)]'>••••••••••••</span>
                        <Button onClick={openPassword} size='sm' type='button' variant='outline'>{t('panelSettings.changePassword')}</Button>
                      </div>
                    )}
                {newPasswordError && <span className='sr-only' id='password-error'>{t(newPasswordError)}</span>}
              </SettingsField>
            </SettingsCard>

            <SettingsCard
              actions={(
                <IconAction label={t('panelSettings.reset')} onClick={() => appearance({ color: DEFAULT_APPEARANCE.color, radius: DEFAULT_APPEARANCE.radius })} size='icon-xs'>
                  <RotateCcw aria-hidden='true' />
                </IconAction>
              )}
              title={t('panelSettings.appearanceGroup')}
            >
              <SettingsField {...field('theme')} label={t('panelSettings.theme')}>
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
              <SettingsField {...field('accent-color')} invalid={!colorValid || invalidField === 'accent-color'} label={t('panelSettings.color')}>
                <div className='@container flex min-h-10 items-center gap-2'>
                  <ToggleGroup aria-label={t('panelSettings.color')} spacing={1} value={[preferences.appearance.color]} onValueChange={values => {
                    if (values[0]) appearance({ color: values[0] });
                  }}>
                    {THEME_PRESETS.map((color, index) => (
                      <ToggleGroupItem
                        aria-label={t(`panelSettings.colors.${index}`)}
                        className='size-7 min-w-0 rounded-[var(--radius-control)] border border-transparent bg-transparent p-0 hover:bg-[var(--color-hover)] aria-pressed:border-primary aria-pressed:bg-transparent @max-[18rem]:nth-[n+5]:hidden'
                        key={color}
                        value={color}
                      >
                        <span className='block size-4.5 rounded-full' style={{ background: color }} />
                      </ToggleGroupItem>
                    ))}
                  </ToggleGroup>
                  <AppearanceColorPicker id='accent-color' value={preferences.appearance.color} onChange={color => appearance({ color })} />
                </div>
              </SettingsField>
              <SettingsField {...field('radius')} help={t('panelSettings.radiusHelp')} label={t('panelSettings.radius')}>
                <div className='grid grid-cols-[minmax(0,1fr)_5.5rem] items-center gap-3'>
                  <Slider aria-label={t('panelSettings.radius')} max={32} min={0} step={1} value={[preferences.appearance.radius]} onValueChange={value => appearance({ radius: Array.isArray(value) ? value[0]! : value })} />
                  <UnitInput id='radius' invalid={invalidField === 'radius'} max={32} min={0} unit='px' value={preferences.appearance.radius} onChange={value => appearance({ radius: Math.max(0, Math.min(32, Math.round(value))) })} />
                </div>
              </SettingsField>
              <SettingsField {...field('language')} label={t('panelSettings.language')}>
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
            </SettingsCard>
          </div>

          <div className='flex min-w-0 flex-col gap-3'>
            <SettingsCard title={t('panelSettings.retention')}>
              <SettingsField {...field('sample-retention')} label={t('panelSettings.sampleRetention')}>
                <UnitInput {...input('sample-retention')} max={366} min={1} unit={days} value={service.sample_retention_days} onChange={value => updateService('sample_retention_days', value)} />
              </SettingsField>
              <SettingsField {...field('runtime-history-retention')} help={t('panelSettings.runtimeHistoryRetentionHelp')} label={t('panelSettings.runtimeHistoryRetention')}>
                <UnitInput {...input('runtime-history-retention')} max={3650} min={1} unit={days} value={service.runtime_history_retention_days ?? 90} onChange={value => updateService('runtime_history_retention_days', value)} />
              </SettingsField>
              <SettingsField {...field('core-log-retention')} help={t('panelSettings.coreLogRetentionHelp')} label={t('panelSettings.coreLogRetention')}>
                <UnitInput {...input('core-log-retention')} max={3650} min={1} unit={days} value={service.core_log_retention_days ?? 7} onChange={value => updateService('core_log_retention_days', value)} />
              </SettingsField>
              <SettingsField {...field('core-log-max-files')} help={t('panelSettings.coreLogMaxFilesHelp')} label={t('panelSettings.coreLogMaxFiles')}>
                <UnitInput {...input('core-log-max-files')} max={1024} min={0} unit={t('panelSettings.files')} value={service.core_log_max_files ?? 0} onChange={value => updateService('core_log_max_files', value)} />
              </SettingsField>
              <SettingsField {...field('core-log-size')} label={t('panelSettings.coreLogSize')}>
                <UnitInput {...input('core-log-size')} max={1024} min={1} unit='MiB' value={service.core_log_max_file_size_mib ?? 32} onChange={value => updateService('core_log_max_file_size_mib', value)} />
              </SettingsField>
            </SettingsCard>

            <PanelUpdateSettings>
              <SettingsField {...field('github-token')} help={t('panelSettings.githubHelp')} label={t('panelSettings.github')}>
                <div className='flex items-center gap-2'>
                  <Input
                    aria-description={initial.github_token_configured && !clearGithub ? t('panelSettings.configured') : undefined}
                    aria-invalid={invalidField === 'github-token' || undefined}
                    autoComplete='new-password'
                    className={initial.github_token_configured && !clearGithub ? 'min-w-0 flex-1 placeholder:text-foreground' : 'min-w-0 flex-1'}
                    disabled={clearGithub}
                    id='github-token'
                    maxLength={8192}
                    placeholder={clearGithub ? t('panelSettings.removed') : initial.github_token_configured ? '••••••••••••' : undefined}
                    type='password'
                    value={github}
                    onChange={e => setGithub(e.target.value)}
                  />
                  {initial.github_token_configured && (
                    <Button size='sm' type='button' variant='ghost' onClick={() => {
                      setClearGithub(!clearGithub);
                      setGithub('');
                    }}>
                      {t(clearGithub ? 'panelSettings.undo' : 'panelSettings.remove')}
                    </Button>
                  )}
                </div>
              </SettingsField>
              <SettingsField {...field('catalog-refresh-interval')} label={t('panelSettings.catalogRefreshInterval')}>
                <UnitInput {...input('catalog-refresh-interval')} max={720} min={1} unit={t('panelSettings.hours')} value={service.catalog_refresh_interval_hours} onChange={value => updateService('catalog_refresh_interval_hours', value)} />
              </SettingsField>
            </PanelUpdateSettings>

            <PanelBackupSettings busy={saving} onBusyChange={setSaving} onRestored={restored} />
          </div>
        </div>
      </ScrollArea>
    </form>
  );
}

export function PanelSettingsPage() {
  const { t } = useTranslation();
  const { view, error, reload } = usePanelSettings();
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-3'>
      {error
        ? (
            <>
              <h1 className='sr-only'>{t('panelSettings.title')}</h1>
              <ErrorNotice error={error} />
              <Button onClick={reload}>{t('panelSettings.retry')}</Button>
            </>
          )
        : view
          ? <SettingsEditor initial={view} />
          : (
              <>
                <h1 className='sr-only'>{t('panelSettings.title')}</h1>
                <LoadingState fullScreen={false} label={t('common.loading')} />
              </>
            )}
    </section>
  );
}
