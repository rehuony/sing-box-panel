import { useState } from 'react';
import { ArrowLeft } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { ChannelPolicy, SubscriptionChannelConfig, SubscriptionFormat } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { SelectField } from '@/components/select-field';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { ChannelLinkDialog } from './channel-token-links';

interface Props {
  name: string;
  legacy: boolean;
  enabled: boolean;
  channelID: string;
  needsSave: boolean;
  onClose: () => void;
  policy: ChannelPolicy;
  onTemplate: () => void;
  format: SubscriptionFormat;
  config: SubscriptionChannelConfig;
  onSave: (
    policy: ChannelPolicy,
    config: SubscriptionChannelConfig,
    identity: { name: string; format: SubscriptionFormat },
  ) => void;
}
export function ChannelOptions({
  policy, config, channelID, needsSave, enabled, legacy, name, format, onClose, onSave, onTemplate,
}: Props) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<'organizer' | 'distribution'>('distribution');
  const [draftName, setDraftName] = useState(name);
  const [draftFormat, setDraftFormat] = useState(format);
  const [linkOpen, setLinkOpen] = useState(false);
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
  const organizerChanged = JSON.stringify(draft.organizer) !== JSON.stringify(policy.organizer)
    || exclusions !== policy.organizer.exclude_names.join('\n');
  const tagsChanged = tags !== (config.exclude_tags?.join('\n') ?? '');
  const typesChanged = types !== (config.exclude_types?.join('\n') ?? '');
  const dirty = draftName !== name || draftFormat !== format
    || draft.selection.new_node_policy !== policy.selection.new_node_policy
    || organizerChanged || tagsChanged || typesChanged;
  const deliveryChanged = needsSave || dirty;
  useUnsavedChanges(dirty, onClose);
  return (
    <>
      {linkOpen && <ChannelLinkDialog channelID={channelID} onClose={() => setLinkOpen(false)} />}
      <Dialog open onOpenChange={(open) => !open && onClose()}>
        <DialogContent className={`channel-rule-dialog${kind === 'organizer' ? ' channel-organizer-dialog' : ''}`}>
          <DialogHeader>
            <div className='channel-options-heading'>
              {kind === 'organizer' && (
                <Button size='icon-sm' variant='ghost' aria-label={t('channels.back')} title={t('channels.back')} onClick={() => setKind('distribution')}><ArrowLeft /></Button>
              )}
              <DialogTitle>{t(`channels.${kind}`)}</DialogTitle>
            </div>
            <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
          </DialogHeader>
          <div className='channel-dialog-scroll'>
            <div className={kind === 'organizer' ? 'channel-organizer-fields' : 'subscription-settings-fields'}>
              {kind === 'organizer'
                ? (
                    <>
                      <Field orientation='horizontal' className='channel-organizer-row'>
                        <FieldLabel htmlFor='channel-prefix'>{t('channels.prefix')}</FieldLabel>
                        <Input
                          id='channel-prefix'
                          maxLength={128}
                          value={draft.organizer.prefix}
                          onChange={(event) => update({ prefix: event.target.value })}
                        />
                      </Field>
                      <Field orientation='horizontal' className='channel-organizer-row'>
                        <FieldLabel htmlFor='channel-sort'>{t('channels.sort')}</FieldLabel>
                        <SelectField<'none' | 'name'>
                          id='channel-sort'
                          value={draft.organizer.sort}
                          onValueChange={(value) => update({ sort: value })}
                          items={[
                            { value: 'none', label: t('channels.original') },
                            { value: 'name', label: t('channels.byName') },
                          ]}
                        />
                      </Field>
                      <Field orientation='horizontal' className='channel-organizer-row'>
                        <FieldLabel htmlFor='channel-incompatible'>{t('channels.incompatible')}</FieldLabel>
                        <SelectField<'skip' | 'error'>
                          id='channel-incompatible'
                          value={draft.organizer.incompatible}
                          onValueChange={(value) => update({ incompatible: value })}
                          items={[
                            { value: 'skip', label: t('channels.skip') },
                            { value: 'error', label: t('channels.stop') },
                          ]}
                        />
                      </Field>
                      <Field orientation='horizontal' className='channel-organizer-row'>
                        <FieldLabel htmlFor='channel-dedup'>{t('channels.deduplicate')}</FieldLabel>
                        <Switch
                          id='channel-dedup'
                          size='sm'
                          checked={draft.organizer.deduplicate}
                          onCheckedChange={(deduplicate) => update({ deduplicate })}
                        />
                      </Field>
                      <Field>
                        <div className='channel-organizer-label'>
                          <FieldLabel htmlFor='channel-exclusions'>{t('channels.exclusions')}</FieldLabel>
                          <FieldDescription id='channel-exclusions-hint'>{t('channels.exclusionsHint')}</FieldDescription>
                        </div>
                        <Textarea
                          id='channel-exclusions'
                          aria-describedby='channel-exclusions-hint'
                          rows={3}
                          value={exclusions}
                          onChange={(event) => setExclusions(event.target.value)}
                        />
                      </Field>
                      {config.exclude_tags?.length || config.exclude_types?.length
                        ? (
                            <>
                              <Field>
                                <FieldLabel htmlFor='legacy-tags'>{t('channels.legacyTags')}</FieldLabel>
                                <Textarea id='legacy-tags' rows={2} value={tags} onChange={(event) => setTags(event.target.value)} />
                              </Field>
                              <Field>
                                <FieldLabel htmlFor='legacy-types'>{t('channels.legacyTypes')}</FieldLabel>
                                <Textarea id='legacy-types' rows={2} value={types} onChange={(event) => setTypes(event.target.value)} />
                              </Field>
                            </>
                          )
                        : null}
                    </>
                  )
                : (
                    <>
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
                          ...(format === 'loon' ? [{ value: 'loon' as const, label: t('subscriptions.channel.format.loon') }] : []),
                        ]}
                      />
                      <span>{t('channels.organizer')}</span>
                      <Button variant='outline' disabled={legacy} onClick={() => setKind('organizer')}>{t('channels.organizer')}</Button>
                      <span>{t('channels.template')}</span>
                      <Button variant='outline' disabled={legacy || draftFormat !== format} onClick={onTemplate}>
                        {t(draft.template ? 'channels.customTemplate' : 'channels.defaultTemplate')}
                      </Button>
                      <label htmlFor='channel-new-nodes'>{t('channels.newNodes')}</label>
                      <SelectField<'include' | 'exclude'>
                        id='channel-new-nodes'
                        disabled={legacy}
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
                    </>
                  )}
            </div>
            {kind === 'distribution' && draftFormat !== format && (
              <p role='status' className='channel-delivery-hint'>{t('channels.applyClientFirst')}</p>
            )}
            {kind === 'distribution' && (
              <Field className='channel-delivery'>
                <Button variant='outline' disabled={!enabled || deliveryChanged}
                  title={deliveryChanged ? t('channels.saveBeforeCopy') : undefined}
                  onClick={() => setLinkOpen(true)}>
                  {t('channels.copyURL')}
                </Button>
              </Field>
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
                  { ...draft, organizer: { ...draft.organizer, exclude_names: split(exclusions) } },
                  {
                    ...config,
                    ...(tagsChanged ? { exclude_tags: split(tags) } : {}),
                    ...(typesChanged ? { exclude_types: split(types) } : {}),
                  },
                  { name: draftName.trim(), format: draftFormat },
                );
                onClose();
              }}
            >
              {t('channels.done')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
