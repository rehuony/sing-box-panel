import { useRef, useState } from 'react';
import { Pencil, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFields,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';
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

import type { SubscriptionFormat } from '@/api/api-client';

import { SelectField } from '@/components/select-field';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';

interface Props {
  name: string;
  /** Whether the channel has its own template rather than the client's default. */
  template: boolean;
  onClose: () => void;
  /** Asks to delete the channel. */
  onDelete: () => void;
  onTemplate: () => void;
  format: SubscriptionFormat;
  onSave: (identity: { name: string; format: SubscriptionFormat }) => void;
}

const formatLabels = { 'sing-box': 'singBox', 'mihomo': 'mihomo', 'loon': 'loon' } as const;

/**
 * The channel's settings: its name, its client and its template, with the
 * channel's delete apart at the footer's start. Choosing another client says
 * in the template's field what applying does; only replacing the channel's
 * own template asks first.
 */
export function ChannelOptions({
  template, name, format, onClose, onDelete, onSave, onTemplate,
}: Props) {
  const { t } = useTranslation();
  const [draftName, setDraftName] = useState(name);
  const [draftFormat, setDraftFormat] = useState(format);
  const [nameInvalid, setNameInvalid] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);
  const dirty = draftName !== name || draftFormat !== format;
  const switching = draftFormat !== format;
  const client = (value: SubscriptionFormat) => t(`subscriptions.channel.format.${formatLabels[value]}`);
  useUnsavedChanges(dirty, onClose);

  function commit() {
    onSave({ name: draftName.trim(), format: draftFormat });
    onClose();
  }

  function apply() {
    if (!draftName.trim()) {
      setNameInvalid(true);
      nameRef.current?.focus();
      toast.add({ title: t('channels.nameRequired'), type: 'error' });
      return;
    }
    if (switching && template) setConfirming(true);
    else commit();
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='flex max-h-[min(576px,calc(100dvh-48px))] flex-col overflow-hidden [&>[data-slot=dialog-footer]]:flex-none [&>[data-slot=dialog-header]]:flex-none'>
        <DialogHeader>
          <DialogTitle>{t('channels.distribution')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
        </DialogHeader>
        <ScrollArea className='-mx-6 min-h-0 flex-1' contentClassName='flex flex-col px-6 py-0.5'>
          <DialogFields className='*:min-w-0'>
            <label htmlFor='channel-edit-name'>{t('channels.name')}</label>
            <Input
              aria-invalid={nameInvalid || undefined}
              id='channel-edit-name'
              onChange={(event) => {
                setDraftName(event.target.value);
                setNameInvalid(false);
              }}
              ref={nameRef}
              value={draftName}
            />
            <label htmlFor='channel-output'>{t('channels.client')}</label>
            <SelectField<SubscriptionFormat>
              id='channel-output'
              value={draftFormat}
              onValueChange={setDraftFormat}
              items={(['sing-box', 'mihomo', 'loon'] as const).map(value => ({ value, label: client(value) }))}
            />
            <span>{t('channels.template')}</span>
            {/* The template reads as a field, its edit action at the field's end. */}
            <div className='flex h-(--control-height) min-w-0 items-center gap-2 rounded-lg border border-input bg-[var(--color-field)] ps-3 pe-1'>
              {switching
                ? <span className='min-w-0 flex-1 truncate text-sm text-[var(--color-warning-text)]'>{t('channels.templateAfterSwitch', { client: client(draftFormat) })}</span>
                : (
                    <>
                      <span className='min-w-0 flex-1 truncate text-sm'>{t(template ? 'channels.customTemplate' : 'channels.defaultTemplate')}</span>
                      <span className='flex-none rounded-[var(--radius-pill)] border px-2 text-xs leading-5 text-[var(--color-text-secondary)]'>{client(format)}</span>
                    </>
                  )}
              <Button
                aria-label={t('channels.editTemplate')}
                className='flex-none'
                onClick={() => {
                  // The template being edited is the applied client's.
                  if (switching) toast.add({ title: t('channels.applyClientFirst'), type: 'info' });
                  else onTemplate();
                }}
                size='xs'
                type='button'
                variant='ghost'
              >
                <Pencil aria-hidden='true' data-icon='inline-start' />
                {t('channels.edit')}
              </Button>
            </div>
          </DialogFields>
        </ScrollArea>
        <DialogFooter>
          {/* Shaped as Cancel, only its text in danger, so it stays apart without standing out. */}
          {/* Below 520px it keeps its icon alone, so the footer stays on one line. */}
          <Button
            className='me-auto text-[var(--color-danger-text)] hover:border-[var(--color-danger-border)] hover:text-[var(--color-danger-text)] max-[520px]:w-(--control-height) max-[520px]:px-0'
            onClick={() => {
              onClose();
              onDelete();
            }}
            type='button'
            variant='outline'
          >
            <Trash2 aria-hidden='true' data-icon='inline-start' />
            <span className='max-[520px]:sr-only'>{t('channels.remove')}</span>
          </Button>
          <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
          <Button onClick={apply} type='button'>{t('channels.apply')}</Button>
        </DialogFooter>
        <AlertDialog onOpenChange={open => !open && setConfirming(false)} open={confirming}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t('channels.switchTitle', { client: client(draftFormat) })}</AlertDialogTitle>
              <AlertDialogDescription>
                {t('channels.switchPrompt', { name: draftName.trim(), from: client(format), to: client(draftFormat) })}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
              <AlertDialogAction onClick={commit} variant='destructive'>{t('channels.switchTemplate')}</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </DialogContent>
    </Dialog>
  );
}
