import { useCallback, useEffect, useState } from 'react';

import type { SubscriptionChannel, SubscriptionToken } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

interface IssuedSecret {
  kind: 'created' | 'rotated';
  token: string;
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
  const [channels, setChannels] = useState<SubscriptionChannel[]>([]);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [channelID, setChannelID] = useState('');
  const [expiresAt, setExpiresAt] = useState('');
  const [secret, setSecret] = useState<IssuedSecret | null>(null);
  const [copied, setCopied] = useState(false);
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      setLoadError(null);
      const [listedTokens, listedChannels] = await Promise.all([
        client.listSubscriptionTokens(signal),
        client.listSubscriptionChannels(signal),
      ]);
      if (!signal?.aborted) {
        setTokens(listedTokens);
        setChannels(listedChannels);
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

  async function create() {
    try {
      setBusy(true);
      setActionError('');
      setSecret(null);
      const issued = await client.createSubscriptionToken({
        channelID: channelID || undefined,
        expiresAt: localDateTimeToISO(expiresAt),
      });
      setSecret({ kind: 'created', token: issued.token });
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally { setBusy(false); }
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
    } finally { setBusy(false); }
  }

  async function revoke(token: SubscriptionToken) {
    try {
      setBusy(true);
      setActionError('');
      await client.revokeSubscriptionToken(token.id);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally { setBusy(false); }
  }

  async function copySecret() {
    if (secret === null || !navigator.clipboard) return;
    await navigator.clipboard.writeText(secret.token);
    setCopied(true);
  }

  function channelName(id?: string): string {
    if (!id) return 'Format selected by request';
    return channels.find((channel) => channel.id === id)?.name ?? id;
  }

  return (
    <section className="subscription-panel" aria-labelledby="subscription-tokens-title">
      <div className="subscription-panel__heading"><div><p className="eyebrow">03 / Public access</p><h2 id="subscription-tokens-title">Tokens</h2><p>Plaintext appears once. Stored metadata never contains a recoverable token.</p></div><span className="count-label">{tokens?.filter((token) => token.active).length ?? 0} active</span></div>
      <ActionError message={actionError} title="Token change failed" />
      {loadError === null ? null : <ErrorNotice error={loadError} title="Could not load tokens" />}

      {secret === null ? null : (
        <div className="one-time-secret" role="status">
          <div><p className="eyebrow">One-time plaintext</p><h3>{secret.kind === 'created' ? 'Save this token now' : 'Save the replacement now'}</h3><p>It cannot be shown again after this notice is dismissed or the page reloads.</p></div>
          <code>{secret.token}</code>
          <div className="inline-actions"><button className="button button--primary" onClick={() => void copySecret()} type="button">{copied ? 'Copied' : 'Copy token'}</button><button className="button button--secondary" onClick={() => { setSecret(null); setCopied(false); }} type="button">I saved it</button></div>
        </div>
      )}

      <form className="token-issuer" onSubmit={(event) => { event.preventDefault(); void create(); }}>
        <div className="field-group"><label htmlFor="token-channel">Channel</label><select id="token-channel" onChange={(event) => setChannelID(event.target.value)} value={channelID}><option value="">Any format (explicit request required)</option>{channels.map((channel) => <option key={channel.id} value={channel.id}>{channel.name} · {channel.format}</option>)}</select></div>
        <div className="field-group"><label htmlFor="token-expiry">Expires at</label><input id="token-expiry" onChange={(event) => setExpiresAt(event.target.value)} type="datetime-local" value={expiresAt} /><span>Leave empty for no expiry.</span></div>
        <button className="button button--primary" disabled={busy} type="submit">{busy ? 'Issuing…' : 'Issue token'}</button>
      </form>

      {tokens === null ? <div className="inline-loading" aria-busy="true">Loading token metadata…</div> : null}
      {tokens?.length === 0 ? <div className="empty-state"><strong>No public tokens.</strong><p>Issue one only after you have a channel or intend callers to select an explicit format.</p></div> : null}
      {tokens && tokens.length > 0 ? (
        <div className="entity-table-wrap"><table className="data-table"><thead><tr><th>Token ID</th><th>Channel</th><th>State</th><th>Actions</th></tr></thead><tbody>{tokens.map((token) => <tr key={token.id}><td><code>{token.id}</code><small className="table-subline">Issued {new Date(token.created_at).toLocaleDateString()}</small></td><td>{channelName(token.channel_id)}</td><td><span className={`state-label ${token.active ? 'state-label--success' : 'state-label--warning'}`}><span aria-hidden="true" />{token.active ? 'Active' : token.revoked_at ? 'Revoked' : 'Expired'}</span></td><td><div className="table-actions"><button className="text-button" disabled={busy || !token.active} onClick={() => void rotate(token)} type="button">Rotate</button><button className="text-button text-button--danger" disabled={busy || !token.active} onClick={() => void revoke(token)} type="button">Revoke</button></div></td></tr>)}</tbody></table></div>
      ) : null}
    </section>
  );
}
