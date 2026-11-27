import type { ReactNode } from 'react';

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { formatDateTime } from '@shared/lib/format-time';
import { formatBytes } from '@shared/lib/telemetry-format';
import { toast } from '@shared/components/ui/toast-manager';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Archive, DatabaseBackup, Download, History, Lock, MoreHorizontal, RotateCcw, Trash2 } from 'lucide-react';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@shared/components/ui/dropdown-menu';
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

import type { PanelBackupInfo } from '@/api/api-client';

import { queries } from '@/api/queries';
import { IconAction } from '@/components/icon-action';
import { StripAction } from '@/components/title-strip';
import { useApiClient } from '@/api/api-client-context';
import { toastFailure } from '@/components/error-notice';
import { LoadingState } from '@/components/loading-state';
import { RegionError, RegionState } from '@/components/region-state';
import { formatLogTimeCompact } from '@/utils/panel-log-presentation';
import { SettingsCard, SettingsField } from '@/components/settings-field';

import { contentsLabel, downloadBackup } from './panel-backup';
import { CreateBackupDialog, RestoreBackupDialog } from './panel-backup-dialogs';

const dataFace = 'font-[family-name:var(--font-data)] text-compact tabular-nums';

/**
 * Backup and restore: the schedule's rows the editor passes in, then the
 * backups in the panel's folder, how many and their size, with Back up now,
 * and the backups themselves, newest first, each with its download and a menu
 * to restore or delete it. In two columns the list takes the rest of the
 * column and scrolls inside it; in one it shows five rows. Restore in the
 * title bar picks a backup or a file.
 */
export function PanelBackupSettings({ children }: { children: ReactNode }) {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const cache = useQueryClient();
  const backups = useQuery(queries.panelBackups(client));
  const [creating, setCreating] = useState(false);
  const [restoring, setRestoring] = useState<{ backup: PanelBackupInfo | null } | null>(null);
  const [deleting, setDeleting] = useState<PanelBackupInfo | null>(null);
  const items = backups.data?.items ?? [];
  const total = items.reduce((sum, backup) => sum + backup.size, 0);
  const time = (backup: PanelBackupInfo) => formatLogTimeCompact(backup.created_at, locale, { seconds: false });

  async function remove(backup: PanelBackupInfo) {
    try {
      await client.deletePanelBackup(backup.name);
      setDeleting(null);
      void cache.invalidateQueries({ queryKey: ['panelBackups'] });
      toast.add({ title: t('panelSettings.backupDeleted'), type: 'success' });
    } catch (cause) {
      setDeleting(null);
      toastFailure(t('panelSettings.backupFailed'), cause);
    }
  }

  // States fill the list's region, as tall as it is beside the other column.
  const state = 'h-full min-h-40';
  let list: ReactNode;
  if (backups.isPending) {
    list = <LoadingState className={state} fullScreen={false} label={t('panelSettings.backupLoading')} />;
  } else if (backups.error) {
    list = <RegionError className={state} error={backups.error} reload={() => void backups.refetch()} title={t('panelSettings.backupLoadFailed')} />;
  } else if (items.length === 0) {
    list = <RegionState className={state} icon={Archive} title={t('panelSettings.backupNone')} />;
  } else {
    list = (
      // One column shows five rows; two give the list the rest of the column. Either way it scrolls inside.
      <ScrollArea className='max-h-[calc(5*2.75rem+4px)] @5xl/settings:absolute @5xl/settings:inset-0 @5xl/settings:max-h-none'>
        <ul aria-label={t('panelSettings.backupLocal')} className='divide-y'>
          {items.map((backup) => {
            const contents = contentsLabel(t, backup.categories);
            return (
              <li
                className='grid h-11 grid-cols-[auto_minmax(0,1fr)_1rem_4.5rem_auto] items-center gap-2.5 ps-4 pe-2 hover:bg-[var(--color-hover)] max-sm:grid-cols-[auto_minmax(0,1fr)_1rem_auto] max-sm:ps-3'
                key={backup.name}
              >
                <time
                  className={dataFace}
                  dateTime={backup.created_at}
                  title={formatDateTime(backup.created_at, locale)}
                >
                  {time(backup)}
                </time>
                <span className='truncate text-compact text-[var(--color-text-faint)]' title={contents || undefined}>
                  {t(`panelSettings.backupKind.${backup.kind}`)}
                  {contents ? ` · ${contents}` : ''}
                </span>
                <span className='flex justify-center text-[var(--color-text-faint)]'>
                  {backup.encrypted && <Lock aria-label={t('panelSettings.backupEncrypted')} className='size-3.5' role='img' />}
                </span>
                <span className={`text-end text-[var(--color-text-secondary)] max-sm:hidden ${dataFace}`}>{formatBytes(backup.size, locale)}</span>
                <span className='flex items-center'>
                  <IconAction label={t('panelSettings.backupDownload')} onClick={() => downloadBackup(client, backup.name)}>
                    <Download aria-hidden='true' />
                  </IconAction>
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={(
                        <IconAction
                          className='data-popup-open:bg-[var(--color-hover)] data-popup-open:text-foreground'
                          label={t('panelSettings.backupMore', { time: time(backup) })}
                        />
                      )}
                    >
                      <MoreHorizontal aria-hidden='true' />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align='end' className='w-auto min-w-36'>
                      <DropdownMenuGroup>
                        <DropdownMenuItem onClick={() => setRestoring({ backup })}>
                          <RotateCcw aria-hidden='true' />
                          {t('panelSettings.backupRestoreAction')}
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => setDeleting(backup)} variant='destructive'>
                          <Trash2 aria-hidden='true' />
                          {t('panelSettings.backupDelete')}
                        </DropdownMenuItem>
                      </DropdownMenuGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </span>
              </li>
            );
          })}
        </ul>
      </ScrollArea>
    );
  }

  return (
    <SettingsCard
      actions={(
        <StripAction
          icon={<History aria-hidden='true' data-icon='inline-start' />}
          label={t('panelSettings.backupRestoreAction')}
          onClick={() => setRestoring({ backup: null })}
        />
      )}
      fill
      title={t('panelSettings.backup')}
    >
      {children}
      <SettingsField id='panel-backups' label={t('panelSettings.backupLocal')}>
        <div className='flex min-h-10 items-center gap-3'>
          <output className='flex min-w-0 flex-1 items-baseline gap-2 text-sm' id='panel-backups'>
            <span className={dataFace}>{backups.data ? t('panelSettings.backupCount', { count: items.length }) : '—'}</span>
            {items.length > 0 && <span className={`truncate text-[var(--color-text-faint)] ${dataFace}`}>{`· ${formatBytes(total, locale)}`}</span>}
          </output>
          <Button onClick={() => setCreating(true)} size='sm' type='button' variant='outline'>
            <DatabaseBackup aria-hidden='true' data-icon='inline-start' />
            {t('panelSettings.backupNow')}
          </Button>
        </div>
      </SettingsField>
      {/* At least three rows beside the other column; the list never makes the column taller. */}
      <div className='@5xl/settings:relative @5xl/settings:min-h-[calc(3*2.75rem)] @5xl/settings:flex-1 @5xl/settings:basis-0'>{list}</div>
      <CreateBackupDialog onOpenChange={setCreating} open={creating} />
      <RestoreBackupDialog
        initial={restoring?.backup ?? null}
        onClose={() => setRestoring(null)}
        open={restoring !== null}
      />
      <AlertDialog onOpenChange={open => !open && setDeleting(null)} open={deleting !== null}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('panelSettings.backupDeleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription className='wrap-anywhere'>
              {t('panelSettings.backupDeleteDescription', { name: deleting?.name ?? '' })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => deleting && void remove(deleting)} variant='destructive'>{t('panelSettings.backupDelete')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsCard>
  );
}
