import { use, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Download, Upload } from 'lucide-react';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@shared/components/ui/alert-dialog';

import type { PanelBackup, PanelSettingsView } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { AuthSessionContext } from '@/stores/auth-session.store';
import { usePanelSettings } from '@/stores/panel-settings.store';
import { SettingsCard, SettingsField } from '@/components/settings-field';
import { describeRequestError, toastFailure } from '@/components/error-notice';

import { backupFile } from './panel-backup';

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

export function PanelBackupSettings({ busy, onBusyChange, onRestored }: {
  busy: boolean;
  onBusyChange: (busy: boolean) => void;
  onRestored: (view: PanelSettingsView) => Promise<void>;
}) {
  const { t } = useTranslation();
  const api = useApiClient();
  const { view } = usePanelSettings();
  const auth = use(AuthSessionContext);
  const inputRef = useRef<HTMLInputElement>(null);
  // A failed restore stays in its dialog; export and read failures go to a toast.
  const [error, setError] = useState<unknown>(null);
  const [pending, setPending] = useState<{
    backup: PanelBackup;
    settingsRevision: number;
  } | null>(null);
  const [working, setWorking] = useState(false);
  const locked = working || busy;

  function failed(cause: unknown) {
    toastFailure(t('panelSettings.backupFailed'), cause);
  }

  async function exportBackup() {
    setWorking(true);
    try {
      const backup = await api.exportPanelBackup();
      const url = URL.createObjectURL(new Blob([JSON.stringify(backup, null, 2)], { type: 'application/json' }));
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `sing-box-panel-${backup.exported_at.slice(0, 10)}.json`;
      anchor.click();
      URL.revokeObjectURL(url);
    } catch (cause) {
      failed(cause);
    } finally {
      setWorking(false);
    }
  }

  async function readBackup(file: File) {
    setWorking(true);
    try {
      if (file.size > 2 * 1024 * 1024) throw new Error(t('panelSettings.backupInvalid'));
      const backup = backupFile(JSON.parse(await file.text()));
      if (!backup) throw new Error(t('panelSettings.backupInvalid'));
      const settings = await api.getPanelSettings();
      setError(null);
      setPending({ backup, settingsRevision: settings.revision });
    } catch (cause) {
      failed(cause);
    } finally {
      setWorking(false);
      if (inputRef.current) inputRef.current.value = '';
    }
  }

  async function restore() {
    if (!pending || locked) return;
    setWorking(true);
    onBusyChange(true);
    setError(null);
    try {
      const result = await api.restorePanelBackup({
        backup: pending.backup,
        settings_revision: pending.settingsRevision,
      });
      setPending(null);
      await onRestored(result.settings);
      toast.add({ title: t('panelSettings.backupRestored'), type: 'success' });
      if (result.reauthentication_required) auth?.retrySession();
    } catch (cause) {
      setError(cause);
    } finally {
      onBusyChange(false);
      setWorking(false);
    }
  }

  const source = record(pending?.backup.panel_settings);
  const server = record(source.server);
  return (
    <SettingsCard title={t('panelSettings.backup')}>
      <SettingsField help={t('panelSettings.backupHelp')} id='panel-backup' label={t('panelSettings.backupContent')}>
        <div className='flex items-center justify-end gap-2' id='panel-backup'>
          <Button disabled={locked} onClick={() => void exportBackup()} size='sm' type='button' variant='outline'>
            <Download aria-hidden='true' />
            {t('panelSettings.backupExport')}
          </Button>
          <Button disabled={locked} onClick={() => inputRef.current?.click()} size='sm' type='button' variant='outline'>
            <Upload aria-hidden='true' />
            {t('panelSettings.backupImport')}
          </Button>
          <input ref={inputRef} type='file' className='sr-only' tabIndex={-1} accept='.json,application/json' aria-label={t('panelSettings.backupImport')} onChange={event => {
            const file = event.target.files?.[0];
            if (file) void readBackup(file);
          }} />
        </div>
      </SettingsField>
      <AlertDialog open={pending !== null} onOpenChange={open => {
        if (!open && !locked) setPending(null);
      }}>
        <AlertDialogContent showCloseButton={!locked}>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('panelSettings.backupConfirm')}</AlertDialogTitle>
            <AlertDialogDescription>{t('panelSettings.backupOverwrite')}</AlertDialogDescription>
          </AlertDialogHeader>
          <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2.5 text-sm [&_dd]:wrap-anywhere [&_dt]:text-[var(--color-text-secondary)]'>
            <dt>{t('panelSettings.backupDate')}</dt>
            <dd>{pending?.backup.exported_at}</dd>
            <dt>{t('panelSettings.listenHost')}</dt>
            <dd>
              {view?.preferences.listen_host}
              {' '}
              →
              {' '}
              {String(server.host ?? '')}
            </dd>
            <dt>{t('panelSettings.listenPort')}</dt>
            <dd>
              {view?.preferences.listen_port}
              {' '}
              →
              {' '}
              {String(server.port ?? '')}
            </dd>
            <dt>{t('panelSettings.origin')}</dt>
            <dd>
              {view?.preferences.external_origin || '—'}
              {' '}
              →
              {' '}
              {String(server.external_origin || '—')}
            </dd>
            <dt>{t('panelSettings.basePath')}</dt>
            <dd>
              {view?.service.base_path || '/'}
              {' '}
              →
              {' '}
              {String(server.base_path || '/')}
            </dd>
            <dt>{t('panelSettings.dataDir')}</dt>
            <dd>
              {view?.service.data_dir}
              {' '}
              →
              {' '}
              {String(source.data_dir ?? '')}
            </dd>
          </dl>
          <p className='mb-4 text-sm leading-relaxed text-[var(--color-text-secondary)]'>{t('panelSettings.backupActivation')}</p>
          {error != null && <p className='text-sm text-[var(--color-danger-text)]' role='alert'>{describeRequestError(error)}</p>}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={locked}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction variant='destructive' disabled={locked} onClick={() => void restore()}>{t('panelSettings.backupRestore')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsCard>
  );
}
