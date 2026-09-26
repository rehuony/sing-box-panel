import { use, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Download, Upload } from 'lucide-react';

import type { PanelBackup, PanelSettingsView } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { AuthSessionContext } from '@/stores/auth-session.store';
import { usePanelSettings } from '@/stores/panel-settings.store';
import { ControlPlaneContext } from '@/stores/control-plane.store';
import { ConfigurationSessionContext } from '@/stores/configuration-session.store';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

import { backupFile } from './panel-backup';

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

export function PanelBackupSettings({ dirty, busy, onBusyChange, onRestored }: {
  dirty: boolean;
  busy: boolean;
  onBusyChange: (busy: boolean) => void;
  onRestored: (view: PanelSettingsView) => Promise<void>;
}) {
  const { t } = useTranslation();
  const api = useApiClient();
  const { view } = usePanelSettings();
  const auth = use(AuthSessionContext);
  const control = use(ControlPlaneContext);
  const configurationSession = use(ConfigurationSessionContext);
  const inputRef = useRef<HTMLInputElement>(null);
  const [error, setError] = useState<unknown>(null);
  const [pending, setPending] = useState<{
    backup: PanelBackup;
    settingsRevision: number;
    configurationRevision: number;
  } | null>(null);
  const [working, setWorking] = useState(false);
  const locked = working || busy;

  async function exportBackup() {
    setWorking(true);
    setError(null);
    try {
      const backup = await api.exportPanelBackup();
      const url = URL.createObjectURL(new Blob([JSON.stringify(backup, null, 2)], { type: 'application/json' }));
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `sing-box-panel-${backup.exported_at.slice(0, 10)}.json`;
      anchor.click();
      URL.revokeObjectURL(url);
    } catch (cause) {
      setError(cause);
    } finally {
      setWorking(false);
    }
  }

  async function readBackup(file: File) {
    setWorking(true);
    setError(null);
    try {
      if (file.size > 13 * 1024 * 1024 + 4096) throw new Error(t('panelSettings.backupInvalid'));
      const backup = backupFile(JSON.parse(await file.text()));
      if (!backup) throw new Error(t('panelSettings.backupInvalid'));
      const [settings, configuration] = await Promise.all([api.getPanelSettings(), api.getConfigurationFile()]);
      setPending({ backup, settingsRevision: settings.revision, configurationRevision: configuration.revision });
    } catch (cause) {
      setError(cause);
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
        configuration_revision: pending.configurationRevision,
      });
      setPending(null);
      await onRestored(result.settings);
      toast.add({ title: t('panelSettings.backupRestored'), type: 'success' });
      if (result.reauthentication_required) {
        auth?.retrySession();
      } else {
        const file = await api.getConfigurationFile();
        configurationSession?.getState().replaceDraft({ file, content: file.content });
        await control?.refresh();
      }
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
    <div className='settings-backup'>
      <h2>{t('panelSettings.backup')}</h2>
      <p className='settings-notice'>{t('panelSettings.backupDescription')}</p>
      {dirty && <p className='settings-notice' role='status'>{t('panelSettings.backupDraftWarning')}</p>}
      {error != null && <ErrorNotice error={error} />}
      <div className='settings-backup-actions'>
        <Button type='button' variant='outline' disabled={locked} onClick={() => void exportBackup()}>
          <Download aria-hidden='true' />
          {t('panelSettings.backupExport')}
        </Button>
        <Button type='button' variant='outline' disabled={locked} onClick={() => inputRef.current?.click()}>
          <Upload aria-hidden='true' />
          {t('panelSettings.backupImport')}
        </Button>
        <input ref={inputRef} type='file' className='sr-only' tabIndex={-1} accept='.json,application/json' aria-label={t('panelSettings.backupImport')} onChange={event => {
          const file = event.target.files?.[0];
          if (file) void readBackup(file);
        }} />
      </div>
      <AlertDialog open={pending !== null} onOpenChange={open => {
        if (!open && !locked) setPending(null);
      }}>
        <AlertDialogContent showCloseButton={!locked}>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('panelSettings.backupConfirm')}</AlertDialogTitle>
            <AlertDialogDescription>{t('panelSettings.backupOverwrite')}</AlertDialogDescription>
          </AlertDialogHeader>
          <dl className='settings-backup-summary'>
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
          <p className='settings-notice'>{t('panelSettings.backupActivation')}</p>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={locked}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction variant='destructive' disabled={locked} onClick={() => void restore()}>{t('panelSettings.backupRestore')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
