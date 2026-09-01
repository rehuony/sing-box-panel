import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type {
  SubscriptionChannel,
  SubscriptionChannelSummary,
  SubscriptionChannelWrite,
  SubscriptionCursor,
  SubscriptionFormat,
  SubscriptionPreview,
  SubscriptionUser,
} from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

interface ChannelDraft {
  name: string;
  enabled: boolean;
  publicHost: string;
  excludeTags: string;
  excludeTypes: string;
  format: SubscriptionFormat;
  editing?: SubscriptionChannel;
}

interface SubscriptionChannelPanelProps {
  users: SubscriptionUser[] | null;
}

const emptyChannel: ChannelDraft = {
  enabled: true,
  excludeTags: '',
  excludeTypes: '',
  format: 'sing-box',
  name: '',
  publicHost: '',
};

function splitList(value: string): string[] {
  return [...new Set(value.split(',').map((item) => item.trim()).filter(Boolean))];
}

export function SubscriptionChannelPanel({ users }: SubscriptionChannelPanelProps) {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const [channels, setChannels] = useState<SubscriptionChannelSummary[] | null>(null);
  const [next, setNext] = useState<SubscriptionCursor>();
  const [loadError, setLoadError] = useState<unknown>(null);
  const [draft, setDraft] = useState<ChannelDraft | null>(null);
  const [actionError, setActionError] = useState('');
  const [deleteCandidateID, setDeleteCandidateID] = useState<string | null>(null);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [previewUserID, setPreviewUserID] = useState('');
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const loadGenerationRef = useRef(0);
  const loadingMoreRef = useRef(false);

  const load = useCallback(async (signal?: AbortSignal, cursor?: SubscriptionCursor, append = false) => {
    const generation = ++loadGenerationRef.current;
    if (!append) {
      setChannels(null);
      setNext(undefined);
    }
    try {
      setLoadError(null);
      const result = await client.listSubscriptionChannels({
        limit: 50,
        beforeTime: cursor?.created_at,
        beforeID: cursor?.id,
      }, signal);
      if (!signal?.aborted && generation === loadGenerationRef.current) {
        setChannels((current) => append ? [...(current ?? []), ...result.items] : result.items);
        setNext(result.next);
      }
    } catch (error) {
      if (!signal?.aborted && generation === loadGenerationRef.current) {
        if (!append) setChannels(null);
        setLoadError(error);
      }
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const channelCount = useMemo(() => channels?.length ?? 0, [channels]);
  const numberFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language ?? 'en'),
    [i18n.language, i18n.resolvedLanguage],
  );
  const selectedPreviewUserID = users?.some((user) => user.id === previewUserID)
    ? previewUserID
    : users?.[0]?.id ?? '';

  async function edit(summary: SubscriptionChannelSummary) {
    setBusy(true);
    setActionError('');
    setMessage('');
    try {
      const channel = await client.getSubscriptionChannel(summary.id);
      setDraft({
        editing: channel,
        enabled: channel.enabled,
        excludeTags: channel.config.exclude_tags?.join(', ') ?? '',
        excludeTypes: channel.config.exclude_types?.join(', ') ?? '',
        format: channel.format,
        name: channel.name,
        publicHost: channel.public_host,
      });
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    if (draft === null) return;
    if (draft.name.trim() === '') {
      setActionError(t('subscriptions.channel.validation.name'));
      return;
    }
    if (draft.publicHost.trim() === '') {
      setActionError(t('subscriptions.channel.validation.publicHost'));
      return;
    }
    const input: SubscriptionChannelWrite = {
      name: draft.name.trim(),
      format: draft.format,
      public_host: draft.publicHost.trim(),
      enabled: draft.enabled,
      config: {
        exclude_tags: splitList(draft.excludeTags),
        exclude_types: splitList(draft.excludeTypes),
      },
    };
    setBusy(true);
    setActionError('');
    try {
      if (draft.editing) {
        await client.updateSubscriptionChannel(
          draft.editing.id,
          input,
          draft.editing.updated_at,
        );
        setMessage(t('subscriptions.channel.message.updated', { name: input.name }));
      } else {
        await client.createSubscriptionChannel(input);
        setMessage(t('subscriptions.channel.message.created', { name: input.name }));
      }
      setDraft(null);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function previewChannel(channel: SubscriptionChannelSummary) {
    if (selectedPreviewUserID === '') {
      setActionError(t('subscriptions.channel.validation.previewUser'));
      return;
    }
    try {
      setBusy(true);
      setActionError('');
      setPreview(await client.previewSubscriptionChannel(channel.id, selectedPreviewUserID));
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function remove(channel: SubscriptionChannelSummary) {
    setBusy(true);
    setActionError('');
    setMessage('');
    try {
      await client.deleteSubscriptionChannel(channel.id, channel.updated_at);
      setMessage(t('subscriptions.channel.message.deleted', { name: channel.name }));
      setDeleteCandidateID(null);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function loadMore() {
    if (next === undefined || loadingMoreRef.current) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    try {
      await load(undefined, next, true);
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }

  return (
    <section className='subscription-panel' id='subscription-channels' aria-labelledby='subscription-channels-title'>
      <div className='subscription-panel__heading'>
        <div>
          <h2 id='subscription-channels-title'>{t('subscriptions.channel.title')}</h2>
          <p>{t('subscriptions.channel.description')}</p>
        </div>
        <span className='count-label'>
          {t('subscriptions.channel.loadedCount', { count: numberFormatter.format(channelCount) })}
        </span>
        <button
          className='button button--primary'
          onClick={() => {
            setDraft(emptyChannel);
            setActionError('');
            setMessage('');
          }}
          type='button'
        >
          {t('subscriptions.channel.new')}
        </button>
      </div>

      {draft === null ? <ActionError message={actionError} title={t('subscriptions.channel.actionFailed')} /> : null}
      {loadError === null ? null : <ErrorNotice error={loadError} title={t('subscriptions.channel.loadFailed')} />}
      {message === ''
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>{t('subscriptions.channel.saved')}</strong>
              <p>{message}</p>
            </div>
          )}

      <div className='channel-preview-control'>
        <div className='field-group'>
          <label htmlFor='preview-user'>{t('subscriptions.channel.previewAsUser')}</label>
          <select
            disabled={busy || users === null || users.length === 0}
            id='preview-user'
            onChange={(event) => setPreviewUserID(event.target.value)}
            value={selectedPreviewUserID}
          >
            <option value=''>{t('subscriptions.common.selectUser')}</option>
            {(users ?? []).map((user) => <option key={user.id} value={user.id}>{user.name}</option>)}
          </select>
        </div>
        {preview
          ? (
              <p role='status'>
                {t('subscriptions.channel.previewSummary', {
                  bundle: preview.applied_bundle_id,
                  diagnostics: numberFormatter.format(preview.result.diagnostics.length),
                  nodes: numberFormatter.format(preview.result.node_count),
                })}
              </p>
            )
          : null}
      </div>

      {channels === null ? <div className='inline-loading' aria-busy='true'>{t('subscriptions.channel.loading')}</div> : null}
      {channels?.length === 0
        ? (
            <div className='empty-state'>
              <strong>{t('subscriptions.channel.empty.title')}</strong>
              <p>{t('subscriptions.channel.empty.description')}</p>
            </div>
          )
        : null}
      {channels && channels.length > 0
        ? (
            <div className='entity-table-wrap'>
              <table className='data-table'>
                <thead>
                  <tr>
                    <th>{t('subscriptions.common.name')}</th>
                    <th>{t('subscriptions.channel.field.format')}</th>
                    <th>{t('subscriptions.channel.field.publicHost')}</th>
                    <th>{t('subscriptions.common.state')}</th>
                    <th>{t('subscriptions.common.actions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {channels.map((channel) => (
                    <tr key={channel.id}>
                      <td data-label={t('subscriptions.common.name')}>
                        <strong>{channel.name}</strong>
                        <small className='table-subline'>{channel.id}</small>
                      </td>
                      <td data-label={t('subscriptions.channel.field.format')}><code>{channel.format}</code></td>
                      <td data-label={t('subscriptions.channel.field.publicHost')}><code>{channel.public_host}</code></td>
                      <td data-label={t('subscriptions.common.state')}>
                        <span className={`state-label ${channel.enabled ? 'state-label--success' : 'state-label--neutral'}`}>
                          <span aria-hidden='true' />
                          {channel.enabled ? t('common.enabled') : t('common.disabled')}
                        </span>
                      </td>
                      <td data-label={t('subscriptions.common.actions')}>
                        {deleteCandidateID === channel.id
                          ? (
                              <div
                                aria-label={t('subscriptions.channel.delete.confirmationAria', { name: channel.name })}
                                className='inline-confirmation'
                                role='group'
                              >
                                <span className='inline-confirmation__prompt'>{t('subscriptions.channel.delete.prompt')}</span>
                                <button
                                  aria-label={t('subscriptions.channel.delete.confirmAria', { name: channel.name })}
                                  className='button button--danger button--small'
                                  disabled={busy}
                                  onClick={() => void remove(channel)}
                                  type='button'
                                >
                                  {busy ? t('subscriptions.common.deleting') : t('subscriptions.common.confirmDelete')}
                                </button>
                                <button
                                  aria-label={t('subscriptions.channel.delete.keepAria', { name: channel.name })}
                                  autoFocus
                                  className='text-button'
                                  disabled={busy}
                                  onClick={() => setDeleteCandidateID(null)}
                                  type='button'
                                >
                                  {t('subscriptions.channel.delete.keep')}
                                </button>
                              </div>
                            )
                          : (
                              <div className='table-actions'>
                                <button className='text-button' disabled={busy} onClick={() => void edit(channel)} type='button'>{t('common.edit')}</button>
                                <button className='text-button' disabled={busy} onClick={() => void previewChannel(channel)} type='button'>{t('subscriptions.channel.preview')}</button>
                                <button
                                  aria-label={t('subscriptions.channel.delete.actionAria', { name: channel.name })}
                                  className='text-button text-button--danger'
                                  disabled={busy}
                                  onClick={() => setDeleteCandidateID(channel.id)}
                                  type='button'
                                >
                                  {t('common.delete')}
                                </button>
                              </div>
                            )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        : null}
      {next
        ? (
            <button
              aria-busy={loadingMore}
              className='button button--secondary'
              disabled={busy || loadingMore}
              onClick={() => void loadMore()}
              type='button'
            >
              {loadingMore ? t('subscriptions.channel.loadingMore') : t('subscriptions.channel.loadMore')}
            </button>
          )
        : null}

      <Dialog open={draft !== null} onOpenChange={(open) => {
        if (!open && !busy) setDraft(null);
      }}>
        <DialogContent className='subscription-editor-dialog'>
          <form className='subscription-dialog-form' onSubmit={(event) => {
            event.preventDefault();
            void save();
          }}>
            <DialogHeader>
              <DialogTitle>
                {draft?.editing
                  ? t('subscriptions.channel.edit', { name: draft.editing.name })
                  : t('subscriptions.channel.create')}
              </DialogTitle>
              <DialogDescription>
                {t('subscriptions.channel.concurrency')}
              </DialogDescription>
            </DialogHeader>
            <ActionError message={actionError} title={t('subscriptions.channel.actionFailed')} />
            {draft === null
              ? null
              : (
                  <div className='subscription-dialog-form__body'>
                    <div className='form-grid'>
                      <div className='field-group'>
                        <label htmlFor='channel-name'>{t('subscriptions.common.name')}</label>
                        <input id='channel-name' maxLength={128} onChange={(event) => setDraft({ ...draft, name: event.target.value })} value={draft.name} />
                      </div>
                      <div className='field-group'>
                        <label htmlFor='channel-public-host'>{t('subscriptions.channel.field.publicHost')}</label>
                        <input id='channel-public-host' onChange={(event) => setDraft({ ...draft, publicHost: event.target.value })} placeholder={t('subscriptions.channel.field.publicHostPlaceholder')} value={draft.publicHost} />
                        <span>{t('subscriptions.channel.field.publicHostHelp')}</span>
                      </div>
                      <div className='field-group'>
                        <label htmlFor='channel-format'>{t('subscriptions.channel.field.format')}</label>
                        <select id='channel-format' onChange={(event) => setDraft({ ...draft, format: event.target.value as SubscriptionFormat })} value={draft.format}>
                          <option value='sing-box'>{t('subscriptions.channel.format.singBox')}</option>
                          <option value='mihomo'>{t('subscriptions.channel.format.mihomo')}</option>
                          <option value='loon'>{t('subscriptions.channel.format.loon')}</option>
                        </select>
                      </div>
                      <div className='field-group form-grid__wide'>
                        <label htmlFor='channel-tags'>{t('subscriptions.channel.field.excludedTags')}</label>
                        <input id='channel-tags' onChange={(event) => setDraft({ ...draft, excludeTags: event.target.value })} placeholder={t('subscriptions.channel.field.excludedTagsPlaceholder')} value={draft.excludeTags} />
                        <span>{t('subscriptions.channel.field.excludedTagsHelp')}</span>
                      </div>
                      <div className='field-group form-grid__wide'>
                        <label htmlFor='channel-types'>{t('subscriptions.channel.field.excludedTypes')}</label>
                        <input id='channel-types' onChange={(event) => setDraft({ ...draft, excludeTypes: event.target.value })} placeholder={t('subscriptions.channel.field.excludedTypesPlaceholder')} value={draft.excludeTypes} />
                        <span>{t('subscriptions.channel.field.excludedTypesHelp')}</span>
                      </div>
                    </div>
                    <label className='check-field'>
                      <input checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} type='checkbox' />
                      <span>
                        <strong>{t('subscriptions.channel.field.enabled')}</strong>
                        <small>{t('subscriptions.channel.field.enabledHelp')}</small>
                      </span>
                    </label>
                  </div>
                )}
            <DialogFooter>
              <button className='button button--secondary' disabled={busy} onClick={() => setDraft(null)} type='button'>{t('common.cancel')}</button>
              <button className='button button--primary' disabled={busy || draft === null} type='submit'>
                {busy ? t('subscriptions.common.saving') : t('subscriptions.channel.save')}
              </button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  );
}
