import { useCallback, useEffect, useMemo, useState } from 'react';

import type {
  SubscriptionChannel,
  SubscriptionChannelWrite,
  SubscriptionFormat,
} from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

interface ChannelDraft {
  name: string;
  enabled: boolean;
  excludeTags: string;
  excludeTypes: string;
  format: SubscriptionFormat;
  editing?: SubscriptionChannel;
}

const emptyChannel: ChannelDraft = {
  enabled: true,
  excludeTags: '',
  excludeTypes: '',
  format: 'sing-box',
  name: '',
};

function splitList(value: string): string[] {
  return [...new Set(value.split(',').map((item) => item.trim()).filter(Boolean))];
}

export function SubscriptionChannelPanel() {
  const client = useApiClient();
  const [channels, setChannels] = useState<SubscriptionChannel[] | null>(null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [draft, setDraft] = useState<ChannelDraft | null>(null);
  const [actionError, setActionError] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      setLoadError(null);
      const result = await client.listSubscriptionChannels(signal);
      if (!signal?.aborted) {
        setChannels(result);
      }
    } catch (error) {
      if (!signal?.aborted) {
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

  function edit(channel: SubscriptionChannel) {
    setDraft({
      editing: channel,
      enabled: channel.enabled,
      excludeTags: channel.config.exclude_tags?.join(', ') ?? '',
      excludeTypes: channel.config.exclude_types?.join(', ') ?? '',
      format: channel.format,
      name: channel.name,
    });
    setActionError('');
    setMessage('');
  }

  async function save() {
    if (draft === null) return;
    if (draft.name.trim() === '') {
      setActionError('Enter a channel name.');
      return;
    }
    const input: SubscriptionChannelWrite = {
      name: draft.name.trim(),
      format: draft.format,
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
        setMessage(`Updated ${input.name}. The public result changes after apply.`);
      } else {
        await client.createSubscriptionChannel(input);
        setMessage(`Created ${input.name}. The public result changes after apply.`);
      }
      setDraft(null);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function remove(channel: SubscriptionChannel) {
    setBusy(true);
    setActionError('');
    setMessage('');
    try {
      await client.deleteSubscriptionChannel(channel.id, channel.updated_at);
      setMessage(`Deleted ${channel.name}.`);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className='subscription-panel' aria-labelledby='subscription-channels-title'>
      <div className='subscription-panel__heading'>
        <div>
          <p className='eyebrow'>01 / Output policy</p>
          <h2 id='subscription-channels-title'>Channels</h2>
          <p>One deterministic renderer contract per client format.</p>
        </div>
        <span className='count-label'>
          {channelCount}
          {' '}
          configured
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
          New channel
        </button>
      </div>

      <ActionError message={actionError} title='Channel change failed' />
      {loadError === null ? null : <ErrorNotice error={loadError} title='Could not load channels' />}
      {message === ''
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>Channel saved</strong>
              <p>{message}</p>
            </div>
          )}

      {channels === null ? <div className='inline-loading' aria-busy='true'>Loading channels…</div> : null}
      {channels?.length === 0
        ? (
            <div className='empty-state'>
              <strong>No publication channels.</strong>
              <p>Create a channel before issuing a channel-bound token.</p>
            </div>
          )
        : null}
      {channels && channels.length > 0
        ? (
            <div className='entity-table-wrap'>
              <table className='data-table'>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Format</th>
                    <th>State</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {channels.map((channel) => (
                    <tr key={channel.id}>
                      <td>
                        <strong>{channel.name}</strong>
                        <small className='table-subline'>{channel.id}</small>
                      </td>
                      <td><code>{channel.format}</code></td>
                      <td>
                        <span className={`state-label ${channel.enabled ? 'state-label--success' : 'state-label--neutral'}`}>
                          <span aria-hidden='true' />
                          {channel.enabled ? 'Enabled' : 'Disabled'}
                        </span>
                      </td>
                      <td>
                        <div className='table-actions'>
                          <button className='text-button' onClick={() => edit(channel)} type='button'>Edit</button>
                          <button className='text-button text-button--danger' disabled={busy} onClick={() => void remove(channel)} type='button'>Delete</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        : null}

      {draft === null
        ? null
        : (
            <form className='control-form' onSubmit={(event) => {
              event.preventDefault();
              void save();
            }}>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Channel editor</p>
                  <h3>{draft.editing ? `Edit ${draft.editing.name}` : 'Create channel'}</h3>
                </div>
                <span>CAS protected</span>
              </div>
              <div className='form-grid'>
                <div className='field-group'>
                  <label htmlFor='channel-name'>Name</label>
                  <input id='channel-name' maxLength={128} onChange={(event) => setDraft({ ...draft, name: event.target.value })} value={draft.name} />
                </div>
                <div className='field-group'>
                  <label htmlFor='channel-format'>Format</label>
                  <select id='channel-format' onChange={(event) => setDraft({ ...draft, format: event.target.value as SubscriptionFormat })} value={draft.format}>
                    <option value='sing-box'>sing-box</option>
                    <option value='mihomo'>Mihomo</option>
                    <option value='loon'>Loon</option>
                  </select>
                </div>
                <div className='field-group form-grid__wide'>
                  <label htmlFor='channel-tags'>Excluded tags</label>
                  <input id='channel-tags' onChange={(event) => setDraft({ ...draft, excludeTags: event.target.value })} placeholder='private, internal' value={draft.excludeTags} />
                  <span>Comma-separated exact tags.</span>
                </div>
                <div className='field-group form-grid__wide'>
                  <label htmlFor='channel-types'>Excluded outbound types</label>
                  <input id='channel-types' onChange={(event) => setDraft({ ...draft, excludeTypes: event.target.value })} placeholder='direct, block' value={draft.excludeTypes} />
                  <span>Unconvertible entries are omitted with diagnostics by the server.</span>
                </div>
              </div>
              <label className='check-field'>
                <input checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} type='checkbox' />
                <span>
                  <strong>Enable this channel</strong>
                  <small>It will be included in the next successful activation bundle.</small>
                </span>
              </label>
              <div className='inline-actions'>
                <button className='button button--primary' disabled={busy} type='submit'>{busy ? 'Saving…' : 'Save channel'}</button>
                <button className='button button--secondary' disabled={busy} onClick={() => setDraft(null)} type='button'>Cancel</button>
              </div>
            </form>
          )}
    </section>
  );
}
