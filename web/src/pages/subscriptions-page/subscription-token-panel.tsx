import { useCallback, useEffect, useState } from 'react';

import type { SubscriptionCursor, SubscriptionToken, SubscriptionUser } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

interface IssuedSecret {
  token: string;
  kind: 'created' | 'rotated';
}

function localDateTimeToISO(value: string): string | undefined {
  if (value === '') return undefined;
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) throw new Error('Enter a valid token expiry.');
  return date.toISOString();
}

export function SubscriptionTokenPanel() {
  const client = useApiClient();
  const [tokens, setTokens] = useState<SubscriptionToken[] | null>(null);
  const [next, setNext] = useState<SubscriptionCursor>();
  const [loadError, setLoadError] = useState<unknown>(null);
  const [expiresAt, setExpiresAt] = useState('');
  const [userID, setUserID] = useState('');
  const [label, setLabel] = useState('');
  const [users, setUsers] = useState<SubscriptionUser[]>([]);
  const [secret, setSecret] = useState<IssuedSecret | null>(null);
  const [copied, setCopied] = useState(false);
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async (signal?: AbortSignal, cursor?: SubscriptionCursor, append = false) => {
    try {
      setLoadError(null);
      const page = await client.listSubscriptionTokens({
        limit: 50,
        beforeTime: cursor?.created_at,
        beforeID: cursor?.id,
      }, signal);
      if (!signal?.aborted) {
        setTokens((current) => append ? [...(current ?? []), ...page.items] : page.items);
        setNext(page.next);
      }
    } catch (error) {
      if (!signal?.aborted) setLoadError(error);
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    const controller = new AbortController();
    void client.listSubscriptionUsers({ limit: 100 }, controller.signal).then((page) => {
      setUsers(page.items);
      setUserID((current) => current || page.items[0]?.id || '');
    }).catch(() => {});
    return () => controller.abort();
  }, [client]);

  async function create() {
    try {
      if (userID === '') throw new Error('Select a subscription user.');
      if (label.trim() === '') throw new Error('Enter a token label.');
      setBusy(true);
      setActionError('');
      setSecret(null);
      const issued = await client.createSubscriptionToken({
        userID,
        label: label.trim(),
        expiresAt: localDateTimeToISO(expiresAt),
      });
      setSecret({ kind: 'created', token: issued.token });
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function toggle(token: SubscriptionToken) {
    try {
      setBusy(true);
      setActionError('');
      await client.setSubscriptionTokenEnabled(token.id, !token.enabled);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function remove(token: SubscriptionToken) {
    try {
      setBusy(true);
      setActionError('');
      await client.deleteSubscriptionToken(token.id);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function rotate(token: SubscriptionToken) {
    try {
      setBusy(true);
      setActionError('');
      setSecret(null);
      const rotated = await client.rotateSubscriptionToken(token.id);
      setSecret({ kind: 'rotated', token: rotated.token });
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function revoke(token: SubscriptionToken) {
    try {
      setBusy(true);
      setActionError('');
      await client.revokeSubscriptionToken(token.id);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function copySecret() {
    if (secret === null || !navigator.clipboard) return;
    await navigator.clipboard.writeText(secret.token);
    setCopied(true);
  }

  return (
    <section className='subscription-panel' aria-labelledby='subscription-tokens-title'>
      <div className='subscription-panel__heading'>
        <div>
          <p className='eyebrow'>03 / Public access</p>
          <h2 id='subscription-tokens-title'>Tokens</h2>
          <p>Plaintext appears once. Stored metadata never contains a recoverable token.</p>
        </div>
        <span className='count-label'>
          {tokens?.filter((token) => token.active).length ?? 0}
          {' '}
          active loaded
        </span>
      </div>
      <ActionError message={actionError} title='Token change failed' />
      {loadError === null ? null : <ErrorNotice error={loadError} title='Could not load tokens' />}

      {secret === null
        ? null
        : (
            <div className='one-time-secret' role='status'>
              <div>
                <p className='eyebrow'>One-time plaintext</p>
                <h3>{secret.kind === 'created' ? 'Save this token now' : 'Save the replacement now'}</h3>
                <p>It cannot be shown again after this notice is dismissed or the page reloads.</p>
              </div>
              <code>{secret.token}</code>
              <div className='inline-actions'>
                <button className='button button--primary' onClick={() => void copySecret()} type='button'>{copied ? 'Copied' : 'Copy token'}</button>
                <button className='button button--secondary' onClick={() => {
                  setSecret(null);
                  setCopied(false);
                }} type='button'>
                  I saved it
                </button>
              </div>
            </div>
          )}

      <form className='token-issuer' onSubmit={(event) => {
        event.preventDefault();
        void create();
      }}>
        <div className='field-group'>
          <label htmlFor='token-user'>User</label>
          <select id='token-user' onChange={(event) => setUserID(event.target.value)} value={userID}>
            <option value=''>Select a user</option>
            {users.map((user) => <option key={user.id} value={user.id}>{user.name}</option>)}
          </select>
        </div>
        <div className='field-group'>
          <label htmlFor='token-label'>Label</label>
          <input id='token-label' onChange={(event) => setLabel(event.target.value)} placeholder='phone, laptop' value={label} />
        </div>
        <div className='field-group'>
          <label htmlFor='token-expiry'>Expires at</label>
          <input id='token-expiry' onChange={(event) => setExpiresAt(event.target.value)} type='datetime-local' value={expiresAt} />
          <span>Leave empty for no expiry.</span>
        </div>
        <button className='button button--primary' disabled={busy} type='submit'>{busy ? 'Issuing…' : 'Issue token'}</button>
      </form>

      {tokens === null ? <div className='inline-loading' aria-busy='true'>Loading token metadata…</div> : null}
      {tokens?.length === 0
        ? (
            <div className='empty-state'>
              <strong>No public tokens.</strong>
              <p>
                Tokens inherit their user&apos;s live grants. A public download still requires
                {' '}
                an applied local-node version and an enabled channel.
              </p>
            </div>
          )
        : null}
      {tokens && tokens.length > 0
        ? (
            <div className='entity-table-wrap'>
              <table className='data-table'>
                <thead>
                  <tr>
                    <th>Token ID</th>
                    <th>State</th>
                    <th>Usage</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {tokens.map((token) => (
                    <tr key={token.id}>
                      <td>
                        <code>{token.id}</code>
                        <strong className='table-subline'>{token.label}</strong>
                        <small className='table-subline'>
                          User
                          {' '}
                          {token.user_id}
                          {' '}
                          · Issued
                          {new Date(token.created_at).toLocaleDateString()}
                        </small>
                      </td>
                      <td>
                        <span className={`state-label ${token.active ? 'state-label--success' : 'state-label--warning'}`}>
                          <span aria-hidden='true' />
                          {token.active ? 'Active' : token.revoked_at ? 'Revoked' : token.enabled ? 'Expired' : 'Disabled'}
                        </span>
                      </td>
                      <td>
                        <strong>
                          {token.successful_request_count}
                          {' '}
                          requests
                        </strong>
                        <small className='table-subline'>
                          {token.body_response_count}
                          {' '}
                          bodies ·
                          {' '}
                          {token.bytes_served}
                          {' '}
                          bytes
                        </small>
                      </td>
                      <td>
                        <div className='table-actions'>
                          <button className='text-button' disabled={busy || !token.active} onClick={() => void rotate(token)} type='button'>Rotate</button>
                          <button className='text-button' disabled={busy || token.revoked_at !== undefined} onClick={() => void toggle(token)} type='button'>{token.enabled ? 'Disable' : 'Enable'}</button>
                          <button className='text-button text-button--danger' disabled={busy || !token.active} onClick={() => void revoke(token)} type='button'>Revoke</button>
                          <button className='text-button text-button--danger' disabled={busy} onClick={() => void remove(token)} type='button'>Delete</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        : null}
      {next
        ? <button className='button button--secondary' disabled={busy} onClick={() => void load(undefined, next, true)} type='button'>Load more tokens</button>
        : null}
    </section>
  );
}
