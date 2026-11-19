import { CirclePlus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { CreatedSubscriptionToken, SubscriptionToken } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

import type { useSubscriptionTokenPage } from './use-subscription-token-page';

import { SubscriptionContentState } from './subscription-content-state';

type KeyAction = 'rotate' | 'delete';

export function SubscriptionTokenPanel({ list, active = true, toolbarTarget }: {
  list: ReturnType<typeof useSubscriptionTokenPage>;
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
}) {
  const { t, i18n } = useTranslation();
  const client = useApiClient();
  const { items, page, pageSize, pages, loading, error, refresh, setPage, setPageSize } = list;
  const [creating, setCreating] = useState(false);
  const [label, setLabel] = useState('');
  const [expiry, setExpiry] = useState('');
  const [limit, setLimit] = useState('');
  const [issued, setIssued] = useState<CreatedSubscriptionToken | null>(null);
  const [confirmation, setConfirmation] = useState<{ action: KeyAction; key: SubscriptionToken } | null>(null);
  const [busy, setBusy] = useState(false);
  useUnsavedChanges(creating && (label !== '' || expiry !== '' || limit !== ''), () => {
    setCreating(false);
    setLabel('');
    setExpiry('');
    setLimit('');
  }, busy);
  const operationRef = useRef(0);
  useEffect(() => () => {
    operationRef.current++;
  }, []);

  const report = useCallback((reason: unknown) => {
    toast.add({ title: describeRequestError(reason), type: 'error' });
  }, []);

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
  async function run(action: KeyAction | 'toggle', key: SubscriptionToken) {
    if (busy) return;
    setBusy(true);
    try {
      if (action === 'rotate') {
        const result = await client.rotateSubscriptionToken(key.id);
        setIssued({ metadata: result.created, token: result.token });
      } else if (action === 'delete') {
        await client.deleteSubscriptionToken(key.id);
      } else {
        await client.setSubscriptionTokenEnabled(key.id, !key.enabled);
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
  async function reveal(key: SubscriptionToken) {
    if (busy) return;
    const generation = operationRef.current;
    setBusy(true);
    try {
      const secret = await client.getSubscriptionTokenSecret(key.id);
      if (generation === operationRef.current) setIssued({ metadata: key, token: secret.token });
    } catch (reason) {
      if (generation === operationRef.current) report(reason);
    } finally {
      if (generation === operationRef.current) setBusy(false);
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

  return (
    <section className='min-h-full min-w-0 flex h-full min-h-0 flex-col' aria-label={t('subscriptions.tabs.tokens')}>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='flex min-w-0 flex-1 items-center justify-end gap-2'>
          <Button aria-label={t('subscriptions.keys.create')} title={t('subscriptions.keys.create')} size='icon-sm' variant='flat' disabled={busy} onClick={() => setCreating(true)}><CirclePlus aria-hidden='true' /></Button>
        </div>
      </ToolbarActions>
      {error ? <ErrorNotice error={error} title={t('subscriptions.token.loadFailed')} /> : null}
      <div className='relative min-h-0 flex-1 overflow-auto flex flex-col *:[table]:flex-none' aria-busy={loading}>
        <table className='workspace-table w-full min-w-256 table-fixed border-collapse [&_td]:px-4 [&_td]:text-center [&_td]:whitespace-nowrap [&_td:first-child]:max-w-60 [&_td:first-child]:truncate [&_td:first-child]:text-left [&_tbody_tr]:border-b [&_th]:px-4 [&_th]:text-center [&_th]:whitespace-nowrap [&_th:first-child]:text-left [&_th:nth-child(1)]:w-[20%] [&_th:nth-child(2)]:w-[22%] [&_th:nth-child(3)]:w-[12%] [&_th:nth-child(4)]:w-[12%] [&_th:nth-child(5)]:w-[34%] [&_th]:sticky [&_th]:top-0 [&_th]:z-1 [&_th]:bg-[var(--color-surface-raised)] [&>thead]:text-muted-foreground'>
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
                <td>
                  <strong>{key.label}</strong>
                </td>
                <td>{formatDate(key.expires_at)}</td>
                <td>{usage(key)}</td>
                <td><span className={key.active ? 'text-xs text-[var(--color-success-text)]' : 'text-xs'}>{keyState(key)}</span></td>
                <td>
                  <div className='inline-grid w-full max-w-80 grid-cols-4 gap-2'>
                    <Button variant='outline' size='sm' disabled={busy || loading} onClick={() => void reveal(key)}>{t('subscriptions.keys.view')}</Button>
                    <Button variant='outline' size='sm' disabled={busy || loading || !!key.revoked_at} onClick={() => void run('toggle', key)}>{t(key.enabled ? 'subscriptions.common.disable' : 'subscriptions.common.enable')}</Button>
                    {(['rotate', 'delete'] as const).map(action => (
                      <Button key={action} variant={action === 'delete' ? 'destructive' : 'outline'} size='sm' disabled={busy || loading || (action === 'rotate' && !!key.revoked_at)} onClick={() => setConfirmation({ action, key })}>{t(`subscriptions.token.action.${action}.name`)}</Button>
                    ))}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {items.length === 0 && (loading
          ? <SubscriptionContentState loading />
          : !error && <SubscriptionContentState kind='keys' title={t('subscriptions.keys.empty')} />)}
      </div>
      <ListPagination
        className='border-t px-2 pb-2'
        page={page}
        pages={pages}
        pageSize={pageSize}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
      />

      <Dialog open={creating} onOpenChange={open => {
        if (!busy) setCreating(open);
      }}>
        <DialogContent className='sm:max-w-[640px]'>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.keys.create')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('subscriptions.keys.shared')}</DialogDescription>
          </DialogHeader>
          <form className='grid grid-cols-[minmax(100px,1fr)_minmax(0,2fr)] items-center gap-(--form-field-gap) [&_[data-slot=dialog-footer]]:col-span-full' onSubmit={event => {
            event.preventDefault();
            void create();
          }}>
            <label htmlFor='key-name'>{t('subscriptions.common.name')}</label>
            <Input id='key-name' value={label} onChange={event => setLabel(event.target.value)} required disabled={busy} maxLength={128} />
            <label htmlFor='key-expiry'>{t('subscriptions.keys.expiry')}</label>
            <Input id='key-expiry' type='datetime-local' value={expiry} onChange={event => setExpiry(event.target.value)} disabled={busy} />
            <label htmlFor='key-quota' title={t('subscriptions.keys.shared')}>{t('subscriptions.keys.quota')}</label>
            <Input id='key-quota' type='number' min={1} max={1_000_000_000} step={1} placeholder={t('subscriptions.keys.unlimited')} value={limit} onChange={event => setLimit(event.target.value)} disabled={busy} />
            <DialogFooter>
              <Button type='button' variant='outline' disabled={busy} onClick={() => setCreating(false)}>{t('subscriptions.keys.cancel')}</Button>
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
            <DialogTitle>{t('subscriptions.keys.details')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('subscriptions.keys.key')}</DialogDescription>
          </DialogHeader>
          {issued
            ? (
                <>
                  <dl className='grid grid-cols-[minmax(100px,1fr)_minmax(0,2fr)] items-center gap-(--form-field-gap) [&_dd]:m-0 [&_dd]:min-w-0 [&_dd]:wrap-anywhere [&_dt]:text-muted-foreground'>
                    <dt>{t('subscriptions.keys.key')}</dt>
                    <dd><code>{issued.token}</code></dd>
                    <dt>{t('subscriptions.keys.expiry')}</dt>
                    <dd>{formatDate(issued.metadata.expires_at)}</dd>
                    <dt>{t('subscriptions.keys.quota')}</dt>
                    <dd>{issued.metadata.download_limit ?? t('subscriptions.keys.unlimited')}</dd>
                  </dl>
                  <DialogFooter>
                    <Button variant='outline' onClick={() => {
                      setIssued(null);
                      operationRef.current++;
                    }}>
                      {t('common.cancel')}
                    </Button>
                    <Button onClick={() => void copy(issued.token)}>{t('subscriptions.token.secret.copy')}</Button>
                  </DialogFooter>
                </>
              )
            : null}
        </DialogContent>
      </Dialog>

      <Dialog open={confirmation !== null} onOpenChange={open => {
        if (!open && !busy) setConfirmation(null);
      }}>
        <DialogContent className='sm:max-w-[640px]'>
          <DialogHeader>
            <DialogTitle>{confirmation ? t(`subscriptions.token.action.${confirmation.action}.name`) : ''}</DialogTitle>
            <DialogDescription>{confirmation?.key.label}</DialogDescription>
          </DialogHeader>
          {confirmation
            ? (
                <>
                  <p>{t(`subscriptions.token.action.${confirmation.action}.prompt`)}</p>
                  <DialogFooter>
                    <Button variant='outline' disabled={busy} onClick={() => setConfirmation(null)}>{t('subscriptions.keys.cancel')}</Button>
                    <Button variant={confirmation.action === 'delete' ? 'destructive' : 'default'} disabled={busy} onClick={() => void run(confirmation.action, confirmation.key)}>{t(`subscriptions.token.action.${confirmation.action}.name`)}</Button>
                  </DialogFooter>
                </>
              )
            : null}
        </DialogContent>
      </Dialog>
    </section>
  );
}
