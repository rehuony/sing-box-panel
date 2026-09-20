import { Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useRef, useState } from 'react';

import type { CreatedSubscriptionToken, SubscriptionChannelSummary, SubscriptionCursor, SubscriptionToken } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

import { buildPublicSubscriptionURL } from './public-subscription-url';

type KeyAction = 'rotate' | 'revoke' | 'delete';

export function SubscriptionTokenPanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t, i18n } = useTranslation();
  const client = useApiClient();
  const [items, setItems] = useState<SubscriptionToken[]>([]);
  const [next, setNext] = useState<SubscriptionCursor>();
  const [cursors, setCursors] = useState<(SubscriptionCursor | undefined)[]>([undefined]);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(10);
  const [reload, setReload] = useState(0);
  const [completedRequest, setCompletedRequest] = useState<{ key: string; error: unknown } | null>(null);
  const [creating, setCreating] = useState(false);
  const [label, setLabel] = useState('');
  const [expiry, setExpiry] = useState('');
  const [limit, setLimit] = useState('');
  const [issued, setIssued] = useState<CreatedSubscriptionToken | null>(null);
  const [channels, setChannels] = useState<SubscriptionChannelSummary[]>([]);
  const [channelID, setChannelID] = useState('');
  const [detail, setDetail] = useState<SubscriptionToken | null>(null);
  const [confirmation, setConfirmation] = useState<KeyAction | null>(null);
  const [busy, setBusy] = useState(false);
  const cursor = cursors[page];
  const requestKey = `${pageSize}:${cursor?.id ?? ''}:${cursor?.created_at ?? ''}:${reload}`;
  const loading = completedRequest?.key !== requestKey;
  const error = loading ? null : completedRequest?.error;
  const operationRef = useRef(0);
  useEffect(() => () => {
    operationRef.current++;
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    void client.listSubscriptionTokens({
      limit: pageSize, beforeID: cursor?.id, beforeTime: cursor?.created_at,
    }, controller.signal)
      .then(result => {
        if (!controller.signal.aborted) {
          setItems(result.items);
          setNext(result.next);
          setCompletedRequest({ key: requestKey, error: null });
        }
      })
      .catch(reason => {
        if (!controller.signal.aborted) setCompletedRequest({ key: requestKey, error: reason });
      });
    return () => controller.abort();
  }, [client, cursor, pageSize, requestKey]);

  const report = useCallback((reason: unknown) => {
    toast.add({ title: describeRequestError(reason), type: 'error' });
  }, []);

  useEffect(() => {
    if (!issued) return;
    const controller = new AbortController();
    async function loadChannels() {
      const all: SubscriptionChannelSummary[] = [];
      let nextCursor: SubscriptionCursor | undefined;
      do {
        const result = await client.listSubscriptionChannels({
          limit: 100, beforeID: nextCursor?.id, beforeTime: nextCursor?.created_at,
        }, controller.signal);
        all.push(...result.items.filter(item => item.enabled));
        nextCursor = result.next;
      } while (nextCursor && !controller.signal.aborted);
      if (!controller.signal.aborted) {
        setChannels(all);
        setChannelID(all[0]?.id ?? '');
      }
    }
    void loadChannels().catch(reason => {
      if (!controller.signal.aborted) report(reason);
    });
    return () => controller.abort();
  }, [client, issued, report]);

  function refresh() {
    setCursors([undefined]);
    setPage(0);
    setReload(value => value + 1);
  }
  function keyState(key: SubscriptionToken) {
    if (key.revoked_at) return t('subscriptions.token.state.revoked');
    if (!key.enabled) return t('subscriptions.token.state.disabled');
    if (key.download_limit !== undefined && key.body_response_count >= key.download_limit) return t('subscriptions.keys.exhausted');
    return t(key.active ? 'subscriptions.token.state.active' : 'subscriptions.token.state.expired');
  }
  function formatDate(value?: string) {
    return value ? new Date(value).toLocaleString(i18n.resolvedLanguage, { dateStyle: 'medium', timeStyle: 'short' }) : t('subscriptions.keys.unlimited');
  }
  function usage(key: SubscriptionToken) {
    return `${key.body_response_count} / ${key.download_limit ?? '∞'}`;
  }
  async function create() {
    if (busy) return;
    const downloadLimit = limit === '' ? undefined : Number(limit);
    const invalidQuota = downloadLimit !== undefined
      && (!Number.isSafeInteger(downloadLimit) || downloadLimit < 1 || downloadLimit > 1_000_000_000);
    if (!label.trim() || invalidQuota) {
      report(new Error(t('subscriptions.keys.invalid')));
      return;
    }
    if (expiry && (!Number.isFinite(new Date(expiry).getTime()) || new Date(expiry).getTime() <= Date.now())) {
      report(new Error(t('subscriptions.token.validation.expiry')));
      return;
    }
    setBusy(true);
    try {
      const result = await client.createSubscriptionToken({
        label: label.trim(), expiresAt: expiry ? new Date(expiry).toISOString() : undefined, downloadLimit,
      });
      setCreating(false);
      setIssued(result);
      setChannels([]);
      setChannelID('');
      refresh();
      setLabel('');
      setExpiry('');
      setLimit('');
    } catch (reason) {
      report(reason);
    } finally {
      setBusy(false);
    }
  }
  async function run(action: KeyAction | 'toggle') {
    if (!detail || busy) return;
    setBusy(true);
    try {
      if (action === 'rotate') {
        const result = await client.rotateSubscriptionToken(detail.id);
        setIssued({ metadata: result.created, token: result.token });
        setChannels([]);
        setChannelID('');
        setDetail(null);
      } else if (action === 'delete') {
        await client.deleteSubscriptionToken(detail.id);
        setDetail(null);
      } else if (action === 'revoke') {
        setDetail(await client.revokeSubscriptionToken(detail.id));
      } else {
        setDetail(await client.setSubscriptionTokenEnabled(detail.id, !detail.enabled));
      }
      setConfirmation(null);
      refresh();
      if (action !== 'rotate') toast.add({ title: t('subscriptions.keys.updated'), type: 'success' });
    } catch (reason) {
      report(reason);
    } finally {
      setBusy(false);
    }
  }
  async function copy(value: string) {
    const generation = operationRef.current;
    try {
      await navigator.clipboard.writeText(value);
      if (generation === operationRef.current) toast.add({ title: t('subscriptions.token.secret.copied'), type: 'success' });
    } catch (reason) {
      if (generation === operationRef.current) report(reason);
    }
  }
  async function inspect(key: SubscriptionToken) {
    setBusy(true);
    try {
      setDetail(await client.getSubscriptionToken(key.id));
    } catch (reason) {
      report(reason);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className='subscription-panel subscription-keys' aria-label={t('subscriptions.tabs.tokens')}>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='subscription-keys__toolbar workspace-toolbar-content'>
          <Button aria-label={t('subscriptions.keys.create')} size='icon' variant='ghost' disabled={busy} onClick={() => setCreating(true)}><Plus className='size-5' /></Button>
        </div>
      </ToolbarActions>
      {error ? <ErrorNotice error={error} title={t('subscriptions.token.loadFailed')} /> : null}
      <div className='subscription-keys__scroll' aria-busy={loading}>
        <table className='subscription-table'>
          <thead>
            <tr>
              <th>{t('subscriptions.common.name')}</th>
              <th>{t('subscriptions.keys.expiry')}</th>
              <th>{t('subscriptions.keys.downloads')}</th>
              <th>{t('subscriptions.common.state')}</th>
              <th>{t('subscriptions.common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {items.map(key => (
              <tr key={key.id}>
                <td><strong>{key.label}</strong></td>
                <td>{formatDate(key.expires_at)}</td>
                <td>{usage(key)}</td>
                <td><span className={`state-label ${key.active ? 'state-label--success' : ''}`}>{keyState(key)}</span></td>
                <td><Button variant='ghost' disabled={busy} onClick={() => void inspect(key)}>{t('subscriptions.common.inspect')}</Button></td>
              </tr>
            ))}
          </tbody>
        </table>
        {!loading && items.length === 0 && !error ? <p className='subscription-empty'>{t('subscriptions.keys.empty')}</p> : null}
        {loading ? <p role='status'>{t('subscriptions.common.loading')}</p> : null}
      </div>
      <footer className='subscription-keys__pagination'>
        <SelectField
          aria-label={t('subscriptions.keys.pageSize')}
          value={pageSize}
          onValueChange={(value) => {
            setPageSize(value);
            refresh();
          }}
          items={[5, 10, 50].map((value) => ({ value, label: t('subscriptions.keys.perPage', { count: value }) }))}
        />
        <div>
          <Button variant='ghost' disabled={page === 0 || loading} onClick={() => setPage(value => value - 1)} aria-label={t('subscriptions.keys.previous')}>‹</Button>
          <span aria-current='page'>{page + 1}</span>
          <Button variant='ghost' disabled={!next || loading} onClick={() => {
            setCursors([...cursors.slice(0, page + 1), next]);
            setPage(value => value + 1);
          }} aria-label={t('subscriptions.keys.next')}>
            ›
          </Button>
        </div>
      </footer>

      <Dialog open={creating} onOpenChange={open => {
        if (!busy) setCreating(open);
      }}>
        <DialogContent className='sm:max-w-[640px]'>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.keys.create')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('subscriptions.keys.shared')}</DialogDescription>
          </DialogHeader>
          <form className='subscription-key-form' onSubmit={event => {
            event.preventDefault();
            void create();
          }}>
            <label htmlFor='key-name'>{t('subscriptions.common.name')}</label>
            <input id='key-name' value={label} onChange={event => setLabel(event.target.value)} required disabled={busy} maxLength={128} />
            <label htmlFor='key-expiry'>{t('subscriptions.keys.expiry')}</label>
            <input id='key-expiry' type='datetime-local' value={expiry} onChange={event => setExpiry(event.target.value)} disabled={busy} />
            <label htmlFor='key-quota' title={t('subscriptions.keys.shared')}>{t('subscriptions.keys.quota')}</label>
            <input id='key-quota' type='number' min={1} max={1_000_000_000} step={1} placeholder={t('subscriptions.keys.unlimited')} value={limit} onChange={event => setLimit(event.target.value)} disabled={busy} />
            <DialogFooter>
              <Button type='button' variant='secondary' disabled={busy} onClick={() => setCreating(false)}>{t('subscriptions.keys.cancel')}</Button>
              <Button type='submit' disabled={busy}>{t('subscriptions.keys.create')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={issued !== null} onOpenChange={open => {
        if (!open) {
          setIssued(null);
          operationRef.current++;
        }
      }}>
        <DialogContent className='sm:max-w-[640px]'>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.keys.created')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('subscriptions.token.secret.oneTime')}</DialogDescription>
          </DialogHeader>
          {issued
            ? (
                <>
                  <dl className='subscription-key-details'>
                    <dt>{t('subscriptions.keys.key')}</dt>
                    <dd><code>{issued.token}</code></dd>
                    <dt>{t('subscriptions.keys.expiry')}</dt>
                    <dd>{formatDate(issued.metadata.expires_at)}</dd>
                    <dt>{t('subscriptions.keys.quota')}</dt>
                    <dd>{issued.metadata.download_limit ?? t('subscriptions.keys.unlimited')}</dd>
                  </dl>
                  {channels.length
                    ? (
                        <div className='subscription-key-form'>
                          <label htmlFor='key-channel'>{t('subscriptions.token.secret.deliveryChannel')}</label>
                          <select id='key-channel' value={channelID} onChange={event => setChannelID(event.target.value)}>{channels.map(channel => <option key={channel.id} value={channel.id}>{channel.name}</option>)}</select>
                        </div>
                      )
                    : null}
                  <DialogFooter>
                    <Button variant='secondary' onClick={() => void copy(issued.token)}>{t('subscriptions.token.secret.copy')}</Button>
                    {channelID ? <Button variant='secondary' onClick={() => void copy(buildPublicSubscriptionURL(issued.token, channelID))}>{t('subscriptions.token.secret.copyURL')}</Button> : null}
                    <Button onClick={() => {
                      setIssued(null);
                      operationRef.current++;
                    }}>
                      {t('subscriptions.keys.done')}
                    </Button>
                  </DialogFooter>
                </>
              )
            : null}
        </DialogContent>
      </Dialog>

      <Dialog open={detail !== null} onOpenChange={open => {
        if (!open && !busy) {
          setDetail(null);
          setConfirmation(null);
        }
      }}>
        <DialogContent className='sm:max-w-[640px]'>
          <DialogHeader>
            <DialogTitle>{confirmation ? t(`subscriptions.token.action.${confirmation}.name`) : detail?.label}</DialogTitle>
            <DialogDescription className='sr-only'>{t('subscriptions.keys.shared')}</DialogDescription>
          </DialogHeader>
          {detail && !confirmation
            ? (
                <>
                  <dl className='subscription-key-details'>
                    <dt>{t('subscriptions.common.state')}</dt>
                    <dd>{keyState(detail)}</dd>
                    <dt>{t('subscriptions.keys.expiry')}</dt>
                    <dd>{formatDate(detail.expires_at)}</dd>
                    <dt>{t('subscriptions.keys.downloads')}</dt>
                    <dd>{usage(detail)}</dd>
                    {detail.user_id
                      ? (
                          <>
                            <dt>{t('subscriptions.keys.scope')}</dt>
                            <dd>{t('subscriptions.keys.legacy')}</dd>
                          </>
                        )
                      : null}
                  </dl>
                  <DialogFooter>
                    <Button variant='secondary' disabled={busy || !!detail.revoked_at} onClick={() => void run('toggle')}>{t(detail.enabled ? 'subscriptions.common.disable' : 'subscriptions.common.enable')}</Button>
                    {(['rotate', 'revoke', 'delete'] as const).map(action => <Button key={action} variant='secondary' disabled={busy || (action !== 'delete' && !detail.active)} onClick={() => setConfirmation(action)}>{t(`subscriptions.token.action.${action}.name`)}</Button>)}
                  </DialogFooter>
                </>
              )
            : null}
          {confirmation
            ? (
                <>
                  <p>{t(`subscriptions.token.action.${confirmation}.prompt`)}</p>
                  <DialogFooter>
                    <Button variant='secondary' disabled={busy} onClick={() => setConfirmation(null)}>{t('subscriptions.keys.cancel')}</Button>
                    <Button disabled={busy} onClick={() => void run(confirmation)}>{t(`subscriptions.token.action.${confirmation}.name`)}</Button>
                  </DialogFooter>
                </>
              )
            : null}
        </DialogContent>
      </Dialog>
    </section>
  );
}
