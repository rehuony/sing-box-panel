import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@shared/components/ui/alert-dialog';

import type { SystemUpdate } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { usePanelUpdate } from '@/hooks/use-panel-update';
import { describeRequestError } from '@/components/error-notice';
import { SettingsField, SettingsGroup } from '@/components/settings-field';

/** One line on where the update stands, or its last failure; empty while nothing happened. */
function updateLine(status: SystemUpdate, t: ReturnType<typeof useTranslation>['t']): string {
  if (status.state !== 'idle') return t(`panelSettings.updateState.${status.state}`, { version: status.target_version ?? '' });
  if (status.last_attempt && !status.last_attempt.succeeded) {
    return t('panelSettings.updateFailed', { version: status.last_attempt.version, code: status.last_attempt.code });
  }
  if (status.check_failure) return t('panelSettings.checkFailed', { code: status.check_failure });
  return '';
}

/** The panel's version, the newest release it knows, and the action that updates to it. */
export function PanelUpdateSettings() {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const cache = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const status = usePanelUpdate();
  const check = useMutation({
    mutationFn: () => client.checkSystemUpdate(),
    onSuccess: data => cache.setQueryData(queries.systemUpdate(client).queryKey, data),
  });
  const start = useMutation({
    mutationFn: () => client.startSystemUpdate(),
    onSuccess: (data) => {
      setConfirming(false);
      cache.setQueryData(queries.systemUpdate(client).queryKey, data);
    },
  });
  const value = status.data;
  const busy = check.isPending || start.isPending || (value !== undefined && value.state !== 'idle');
  const line = check.error ? describeRequestError(check.error) : value ? updateLine(value, t) : status.error ? describeRequestError(status.error) : '';
  return (
    <SettingsGroup title={t('panelSettings.version')}>
      <SettingsField id='panel-version' label={t('panelSettings.currentVersion')}>
        <output className='flex min-h-10 items-center font-mono text-sm' id='panel-version'>{value?.current_version ?? '—'}</output>
      </SettingsField>
      <SettingsField id='panel-latest-version' label={t('panelSettings.latestVersion')}>
        <div className='flex min-h-10 flex-wrap items-center gap-x-3 gap-y-2'>
          <output className='flex min-w-0 flex-1 items-baseline gap-2 text-sm' id='panel-latest-version'>
            <span className='font-mono'>{value?.latest_version ?? '—'}</span>
            {value?.checked_at && (
              <span className='truncate text-muted-foreground' title={formatDateTime(value.checked_at, locale)}>
                {t('panelSettings.checkedAt', { time: formatRelativeTime(value.checked_at, locale) })}
              </span>
            )}
          </output>
          {value?.supported !== false && (
            <Button type='button' variant='outline' onClick={() => {
              if (!busy) check.mutate();
            }}>
              {t(check.isPending ? 'panelSettings.checking' : 'panelSettings.checkUpdate')}
            </Button>
          )}
          {value?.update_available && value.state === 'idle' && (
            <Button type='button' variant='outline' onClick={() => {
              start.reset();
              setConfirming(true);
            }}>
              {t('panelSettings.updateTo', { version: value.latest_version })}
            </Button>
          )}
        </div>
      </SettingsField>
      {(line || value?.supported === false) && (
        <p className='min-h-5 text-sm text-muted-foreground wrap-anywhere' role='status'>{line || t('panelSettings.updateUnsupported')}</p>
      )}
      <AlertDialog open={confirming} onOpenChange={(open) => {
        if (!open && !start.isPending) setConfirming(false);
      }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('panelSettings.updateConfirmTitle', { version: value?.latest_version ?? '' })}</AlertDialogTitle>
            <AlertDialogDescription>{t('panelSettings.updateConfirm', { current: value?.current_version ?? '', version: value?.latest_version ?? '' })}</AlertDialogDescription>
          </AlertDialogHeader>
          {start.error ? <p className='text-sm text-[var(--color-danger-text)]' role='alert'>{describeRequestError(start.error)}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => {
              if (!start.isPending) start.mutate();
            }}>
              {t(start.isPending ? 'panelSettings.updating' : 'panelSettings.updateAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsGroup>
  );
}
