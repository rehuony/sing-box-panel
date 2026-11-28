import { useState } from 'react';
import { Eraser } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQueryClient } from '@tanstack/react-query';
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

import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { describeRequestError } from '@/components/error-notice';

/**
 * The clear action of the current node's runtime states and its
 * confirmation, an eraser named by its tooltip like the events' clear.
 * Clearing keeps the node's current state and leaves its events and core
 * log; onCleared runs once the states are gone.
 */
export function ClearRuntimeStatesAction({ className, onCleared }: { className?: string; onCleared?: () => void }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const [open, setOpen] = useState(false);
  const clear = useMutation({
    mutationFn: () => client.clearCoreRuntimeHistory(),
    onError: error => toast.add({ description: describeRequestError(error), title: t('productLogs.clearStates.failed'), type: 'error' }),
    onSuccess: (result) => {
      toast.add({ title: t('productLogs.clearStates.done', { count: result.deleted }), type: 'success' });
      setOpen(false);
      onCleared?.();
    },
    // The events show the entry the clearing recorded.
    onSettled: () => Promise.all([
      cache.invalidateQueries({ queryKey: ['fleetRuntimeHistory'] }),
      cache.invalidateQueries({ queryKey: ['panelLogs'] }),
    ]),
  });
  return (
    <>
      <IconAction className={className} label={t('productLogs.clearStates.title')} onClick={() => setOpen(true)} variant='flat'>
        <Eraser aria-hidden='true' />
      </IconAction>
      <AlertDialog
        onOpenChange={(next) => {
          if (!clear.isPending) setOpen(next);
        }}
        open={open}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('productLogs.clearStates.title')}</AlertDialogTitle>
            <AlertDialogDescription className='flex flex-col gap-1'>
              <span>{t('productLogs.clearStates.description')}</span>
              <span>{t('productLogs.clearStates.note')}</span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              aria-busy={clear.isPending}
              onClick={() => {
                if (!clear.isPending) clear.mutate();
              }}
              variant='destructive'
            >
              {t('productLogs.clearStates.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
