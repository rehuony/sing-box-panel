import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChannelPolicy, SubscriptionFormat } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { SelectField } from '@/components/select-field';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

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
      <DialogContent className='channel-rule-dialog'>
        <DialogHeader>
          <DialogTitle>{t('channels.distribution')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
        </DialogHeader>
        <div className='channel-dialog-scroll'>
          <div className='subscription-settings-fields'>
            <label htmlFor='channel-edit-name'>{t('channels.name')}</label>
            <input id='channel-edit-name' value={draftName} onChange={(event) => setDraftName(event.target.value)} />
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
            <Button variant='outline' disabled={draftFormat !== format} onClick={onTemplate}>
              {t(policy.template ? 'channels.customTemplate' : 'channels.defaultTemplate')}
            </Button>
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
          </div>
          {draftFormat !== format && (
            <p role='status' className='channel-delivery-hint'>{t(policy.template ? 'channels.replaceTemplateHint' : 'channels.applyClientFirst')}</p>
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
            {t(draftFormat !== format && policy.template ? 'channels.switchTemplate' : 'channels.done')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
