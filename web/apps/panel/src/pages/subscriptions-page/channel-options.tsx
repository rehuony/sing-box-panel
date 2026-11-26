import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFields,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

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

/** The channel's settings: its name, its client and its template, with the channel's delete. */
export function ChannelOptions({
  template, name, format, onClose, onDelete, onSave, onTemplate,
}: Props) {
  const { t } = useTranslation();
  const [draftName, setDraftName] = useState(name);
  const [draftFormat, setDraftFormat] = useState(format);
  const dirty = draftName !== name || draftFormat !== format;
  useUnsavedChanges(dirty, onClose);
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='flex max-h-[min(576px,calc(100dvh-48px))] flex-col overflow-hidden [&>[data-slot=dialog-footer]]:flex-none [&>[data-slot=dialog-header]]:flex-none'>
        <DialogHeader>
          <DialogTitle>{t('channels.distribution')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
        </DialogHeader>
        <div className='-mx-6 flex min-h-0 flex-1 flex-col overflow-auto overscroll-contain px-6 py-0.5'>
          <DialogFields className='*:min-w-0'>
            <label htmlFor='channel-edit-name'>{t('channels.name')}</label>
            <Input id='channel-edit-name' value={draftName} onChange={(event) => setDraftName(event.target.value)} />
            <label htmlFor='channel-output'>{t('channels.client')}</label>
            <SelectField<SubscriptionFormat>
              id='channel-output'
              value={draftFormat}
              onValueChange={setDraftFormat}
              items={[
                { value: 'sing-box', label: t('subscriptions.channel.format.singBox') },
                { value: 'mihomo', label: t('subscriptions.channel.format.mihomo') },
                { value: 'loon', label: t('subscriptions.channel.format.loon') },
              ]}
            />
            <span>{t('channels.template')}</span>
            <div className='flex min-h-(--control-height) min-w-0 items-center justify-between gap-3'>
              <span className='min-w-0 truncate text-sm'>{t(template ? 'channels.customTemplate' : 'channels.defaultTemplate')}</span>
              <Button aria-label={t('channels.editTemplate')} disabled={draftFormat !== format} onClick={onTemplate} size='sm' variant='outline'>
                {t('channels.edit')}
              </Button>
            </div>
          </DialogFields>
          {draftFormat !== format && (
            <p role='status' className='text-sm text-muted-foreground'>{t(template ? 'channels.replaceTemplateHint' : 'channels.applyClientFirst')}</p>
          )}
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant='destructive'
            onClick={() => {
              onClose();
              onDelete();
            }}
          >
            {t('channels.remove')}
          </Button>
          <Button
            variant='default'
            disabled={!draftName.trim()}
            onClick={() => {
              onSave({ name: draftName.trim(), format: draftFormat });
              onClose();
            }}
          >
            {t(draftFormat !== format && template ? 'channels.switchTemplate' : 'channels.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
