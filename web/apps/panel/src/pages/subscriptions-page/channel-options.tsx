import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import type { ChannelPolicy, SubscriptionFormat } from '@/api/api-client';

import { SelectField } from '@/components/select-field';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';

interface Props {
  name: string;
  onClose: () => void;
  policy: ChannelPolicy;
  onTemplate: () => void;
  format: SubscriptionFormat;
  onSave: (
    policy: ChannelPolicy,
    identity: { name: string; format: SubscriptionFormat },
  ) => void;
}
export function ChannelOptions({
  policy, name, format, onClose, onSave, onTemplate,
}: Props) {
  const { t } = useTranslation();
  const [draftName, setDraftName] = useState(name);
  const [draftFormat, setDraftFormat] = useState(format);
  const [draft, setDraft] = useState(() => structuredClone(policy));
  const dirty = draftName !== name || draftFormat !== format
    || draft.selection.new_node_policy !== policy.selection.new_node_policy;
  useUnsavedChanges(dirty, onClose);
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='flex max-h-[min(576px,calc(100dvh-48px))] w-170 max-w-[calc(100vw-48px)] flex-col overflow-hidden sm:max-w-170 [&>[data-slot=dialog-footer]]:flex-none [&>[data-slot=dialog-header]]:flex-none'>
        <DialogHeader>
          <DialogTitle>{t('channels.distribution')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
        </DialogHeader>
        <div className='-mx-6 flex min-h-0 flex-1 flex-col overflow-auto overscroll-contain px-6 py-0.5'>
          <div className='grid grid-cols-[minmax(max-content,1fr)_minmax(0,2fr)] items-center gap-x-6 gap-y-(--form-field-gap) pt-2 pb-(--form-field-gap) *:min-w-0 [&>label]:whitespace-nowrap [&>span]:whitespace-nowrap max-[720px]:grid-cols-[max-content_minmax(0,1fr)] max-[720px]:gap-3 [&_[aria-invalid=true]]:outline-1 [&_[aria-invalid=true]]:outline-[var(--color-danger)] [&_[data-slot=select-trigger]]:w-full [&_input:not([type=checkbox])]:w-full [&_input:not([type=checkbox])]:min-w-0 [&_input[type=checkbox]]:justify-self-start [&_textarea]:max-h-45 [&_textarea]:min-h-[90px] [&_textarea]:resize-y [&_textarea]:rounded-[var(--radius-control)] [&_textarea]:border-0 [&_textarea]:bg-[var(--color-surface-raised)] [&_textarea]:p-3'>
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
            <label htmlFor='channel-new-nodes'>{t('channels.newNodes')}</label>
            <SelectField<'include' | 'exclude'>
              id='channel-new-nodes'
              value={draft.selection.new_node_policy}
              onValueChange={(value) =>
                setDraft({
                  ...draft,
                  selection: {
                    ...draft.selection,
                    new_node_policy: value,
                  },
                })
              }
              items={[
                { value: 'include', label: t('channels.include') },
                { value: 'exclude', label: t('channels.exclude') },
              ]}
            />
            <span>{t('channels.template')}</span>
            <Button variant='outline' disabled={draftFormat !== format} onClick={onTemplate}>
              <span className='truncate'>{t(policy.template ? 'channels.customTemplate' : 'channels.defaultTemplate')}</span>
            </Button>
          </div>
          {draftFormat !== format && (
            <p role='status' className='text-sm text-muted-foreground'>{t(policy.template ? 'channels.replaceTemplateHint' : 'channels.applyClientFirst')}</p>
          )}
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant='default'
            disabled={!draftName.trim()}
            onClick={() => {
              onSave(
                draft,
                { name: draftName.trim(), format: draftFormat },
              );
              onClose();
            }}
          >
            {t(draftFormat !== format && policy.template ? 'channels.switchTemplate' : 'channels.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
