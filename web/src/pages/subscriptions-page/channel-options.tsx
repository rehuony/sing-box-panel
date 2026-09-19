import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChannelPolicy, SubscriptionChannelConfig } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

interface Props {
  onClose: () => void;
  policy: ChannelPolicy;
  onTemplate: () => void;
  config: SubscriptionChannelConfig;
  kind: 'organizer' | 'distribution';
  onSave: (policy: ChannelPolicy, config: SubscriptionChannelConfig) => void;
}
export function ChannelOptions({ kind, policy, config, onClose, onSave, onTemplate }: Props) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(() => structuredClone(policy));
  const [exclusions, setExclusions] = useState(policy.organizer.exclude_names.join('\n'));
  const [tags, setTags] = useState(config.exclude_tags?.join('\n') ?? '');
  const [types, setTypes] = useState(config.exclude_types?.join('\n') ?? '');
  const update = (patch: Partial<ChannelPolicy['organizer']>) =>
    setDraft((value) => ({ ...value, organizer: { ...value.organizer, ...patch } }));
  const split = (value: string) => [
    ...new Set(
      value
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
    ),
  ];
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='channel-rule-dialog'>
        <DialogHeader>
          <DialogTitle>{t(`channels.${kind}`)}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
        </DialogHeader>
        <div className='channel-dialog-scroll'>
          <div className='subscription-settings-fields'>
            {kind === 'organizer'
              ? (
                  <>
                    <label htmlFor='channel-prefix'>{t('channels.prefix')}</label>
                    <input
                      id='channel-prefix'
                      maxLength={128}
                      value={draft.organizer.prefix}
                      onChange={(event) => update({ prefix: event.target.value })}
                    />
                    <label htmlFor='channel-exclusions'>{t('channels.exclusions')}</label>
                    <textarea
                      id='channel-exclusions'
                      placeholder={t('channels.exclusionsHint')}
                      value={exclusions}
                      onChange={(event) => setExclusions(event.target.value)}
                    />
                    <label htmlFor='channel-sort'>{t('channels.sort')}</label>
                    <select
                      id='channel-sort'
                      value={draft.organizer.sort}
                      onChange={(event) => update({ sort: event.target.value as 'none' | 'name' })}
                    >
                      <option value='none'>{t('channels.original')}</option>
                      <option value='name'>{t('channels.byName')}</option>
                    </select>
                    <label htmlFor='channel-dedup'>{t('channels.deduplicate')}</label>
                    <input
                      id='channel-dedup'
                      type='checkbox'
                      checked={draft.organizer.deduplicate}
                      onChange={(event) => update({ deduplicate: event.target.checked })}
                    />
                    <label htmlFor='channel-incompatible'>{t('channels.incompatible')}</label>
                    <select
                      id='channel-incompatible'
                      value={draft.organizer.incompatible}
                      onChange={(event) => update({ incompatible: event.target.value as 'skip' | 'error' })}
                    >
                      <option value='skip'>{t('channels.skip')}</option>
                      <option value='error'>{t('channels.stop')}</option>
                    </select>
                    {config.exclude_tags?.length || config.exclude_types?.length
                      ? (
                          <>
                            <label htmlFor='legacy-tags'>{t('channels.legacyTags')}</label>
                            <textarea
                              id='legacy-tags'
                              value={tags}
                              onChange={(event) => setTags(event.target.value)}
                            />
                            <label htmlFor='legacy-types'>{t('channels.legacyTypes')}</label>
                            <textarea
                              id='legacy-types'
                              value={types}
                              onChange={(event) => setTypes(event.target.value)}
                            />
                          </>
                        )
                      : null}
                  </>
                )
              : (
                  <>
                    <span>{t('channels.template')}</span>
                    <Button variant='ghost' onClick={onTemplate}>
                      {t(draft.template ? 'channels.customTemplate' : 'channels.defaultTemplate')}
                    </Button>
                    <label htmlFor='channel-new-nodes'>{t('channels.newNodes')}</label>
                    <select
                      id='channel-new-nodes'
                      value={draft.selection.new_node_policy}
                      onChange={(event) =>
                        setDraft({
                          ...draft,
                          selection: {
                            ...draft.selection,
                            new_node_policy: event.target.value as 'include' | 'exclude',
                          },
                        })
                      }
                    >
                      <option value='include'>{t('channels.include')}</option>
                      <option value='exclude'>{t('channels.exclude')}</option>
                    </select>
                  </>
                )}
          </div>
        </div>
        <DialogFooter>
          <Button variant='secondary' onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant='secondary'
            onClick={() => {
              onSave(
                { ...draft, organizer: { ...draft.organizer, exclude_names: split(exclusions) } },
                { ...config, exclude_tags: split(tags), exclude_types: split(types) },
              );
              onClose();
            }}
          >
            {t('channels.done')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
