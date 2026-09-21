import { useTranslation } from 'react-i18next';

import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog';

interface Props {
  busy: boolean;
  open: boolean;
  onCancel: () => void;
  onDiscard: () => void;
}

export function UnsavedChangesDialog({ open, busy, onCancel, onDiscard }: Props) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={value => {
      if (!value) onCancel();
    }}>
      <AlertDialogContent showCloseButton={false} className='gap-6 data-[size=default]:max-w-sm'>
        <AlertDialogHeader className='gap-2'>
          <AlertDialogTitle className='pr-0'>{t('common.unsaved.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('common.unsaved.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter className='gap-2'>
          <AlertDialogCancel variant='outline'>{t('common.unsaved.keepEditing')}</AlertDialogCancel>
          <AlertDialogAction variant='destructive' disabled={busy} onClick={onDiscard}>
            {t('common.unsaved.discard')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
