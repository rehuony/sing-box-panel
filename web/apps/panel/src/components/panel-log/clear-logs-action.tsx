import { useState } from 'react';
import { Eraser } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { toast } from '@shared/components/ui/toast-manager';
import { EM_DASH, formatBytes } from '@shared/lib/telemetry-format';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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

import type { LogScope } from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { StripAction } from '@/components/title-strip';
import { useApiClient } from '@/api/api-client-context';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

/**
 * The clear action of the log in one scope, the panel's own entries or one
 * node's events, and its confirmation. The dialog reads how many entries the
 * scope holds and how much disk the whole log takes now when it opens; what
 * clearing frees is only known after it, so the toast says that. Clearing
 * deletes every entry in the scope, whatever the strip filters, and leaves
 * other scopes and runtime states. In a node's logs window the action is an
 * eraser named by its tooltip, like the core log's own clear.
 */
export function ClearLogsAction({ className, scope }: { className?: string; scope: LogScope }) {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const [open, setOpen] = useState(false);
  const kind = 'nodeID' in scope ? 'node' : 'panel';
  const usage = useQuery({
    queryKey: ['logUsage', 'nodeID' in scope ? scope.nodeID : 'panel'],
    queryFn: ({ signal }) => client.getLogUsage(scope, signal),
    enabled: open, gcTime: 0, staleTime: 0,
  });
  const clear = useMutation({
    mutationFn: () => client.clearPanelLogs(scope),
    onError: error => toast.add({ description: describeRequestError(error), title: t('productLogs.clearLogs.failed'), type: 'error' }),
    onSuccess: (result) => {
      toast.add({
        title: t('productLogs.clearLogs.done', { count: result.deleted, size: formatBytes(result.freed_bytes, i18n.resolvedLanguage) }),
        type: 'success',
      });
      setOpen(false);
    },
    // The list shows the entry the clearing recorded.
    onSettled: () => cache.invalidateQueries({ queryKey: ['panelLogs'] }),
  });
  const known = usage.data;
  return (
    <>
      {kind === 'node'
        ? (
            <IconAction className={className} label={t('productLogs.clearLogs.nodeTitle')} onClick={() => setOpen(true)} variant='flat'>
              <Eraser aria-hidden='true' />
            </IconAction>
          )
        : (
            <StripAction
              className={className}
              icon={<Eraser aria-hidden='true' data-icon='inline-start' />}
              label={t('productLogs.clearLogs.action')}
              onClick={() => setOpen(true)}
            />
          )}
      <AlertDialog
        onOpenChange={(next) => {
          if (!clear.isPending) setOpen(next);
        }}
        open={open}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(`productLogs.clearLogs.${kind}Title`)}</AlertDialogTitle>
            <AlertDialogDescription className='flex flex-col gap-1'>
              <span>
                {t(`productLogs.clearLogs.${kind}Description`, {
                  count: known?.entries ?? 0,
                  entries: known ? known.entries : EM_DASH,
                  size: known ? formatBytes(known.bytes, i18n.resolvedLanguage) : EM_DASH,
                })}
              </span>
              <span>{t('productLogs.clearLogs.note')}</span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <ErrorNotice error={usage.error} />
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              aria-busy={clear.isPending}
              onClick={() => {
                if (!clear.isPending) clear.mutate();
              }}
              variant='destructive'
            >
              {t('productLogs.clearLogs.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
