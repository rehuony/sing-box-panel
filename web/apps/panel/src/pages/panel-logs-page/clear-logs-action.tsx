import { useState } from 'react';
import { Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
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

import { useApiClient } from '@/api/api-client-context';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

/**
 * The log's clear action and its confirmation. The dialog reads how many
 * entries the log holds and how much disk it takes now when it opens; what
 * clearing frees is only known after it, so the toast says that. Clearing
 * deletes every entry, whatever the strip filters, and leaves runtime states.
 */
export function ClearLogsAction() {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const [open, setOpen] = useState(false);
  const usage = useQuery({
    queryKey: ['logUsage'],
    queryFn: ({ signal }) => client.getLogUsage(signal),
    enabled: open, gcTime: 0, staleTime: 0,
  });
  const clear = useMutation({
    mutationFn: () => client.clearPanelLogs(),
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
      <Button className='max-sm:w-(--control-height-sm) max-sm:px-0' onClick={() => setOpen(true)} size='sm' variant='flat'>
        <Trash2 aria-hidden='true' data-icon='inline-start' />
        <span className='max-sm:sr-only'>{t('productLogs.clearLogs.action')}</span>
      </Button>
      <AlertDialog
        onOpenChange={(next) => {
          if (!clear.isPending) setOpen(next);
        }}
        open={open}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('productLogs.clearLogs.title')}</AlertDialogTitle>
            <AlertDialogDescription className='flex flex-col gap-1'>
              <span>
                {t('productLogs.clearLogs.description', {
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
