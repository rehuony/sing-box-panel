import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Check, CirclePause, CirclePlay, CirclePlus, Copy, Eye, RefreshCw, Trash2 } from 'lucide-react';
import { InputGroup, InputGroupAddon, InputGroupInput, InputGroupText } from '@shared/components/ui/input-group';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';
import { Dialog, DialogContent, DialogDescription, DialogFields, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { CreatedSubscriptionToken, SubscriptionToken } from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { DateTimeField } from '@/components/date-time-field';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

import type { useSubscriptionTokenPage } from './use-subscription-token-page';

import { SubscriptionContentState } from './subscription-content-state';

type KeyAction = 'rotate' | 'delete';

/** How long a new key lasts: never, a number of days from now, or a chosen time. */
type Expiry = 'never' | '1' | '7' | '30' | '90' | '365' | 'custom';
const expiries: Expiry[] = ['never', '1', '7', '30', '90', '365', 'custom'];

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
  const [expiry, setExpiry] = useState<Expiry>('never');
  const [expiresAt, setExpiresAt] = useState<Date | null>(null);
  // The earliest time a chosen expiry may take, read when the choice opens.
  const [earliest, setEarliest] = useState(() => new Date());
  const [limit, setLimit] = useState('');
  // A form problem names the field it focuses; a refused request names none.
  const [issue, setIssue] = useState<{ field?: 'name' | 'expiry' | 'quota'; message: string } | null>(null);
  const [issued, setIssued] = useState<{ key: CreatedSubscriptionToken; kind: 'created' | 'rotated' | 'viewed' } | null>(null);
  const [copied, setCopied] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);
  const quotaRef = useRef<HTMLInputElement>(null);
  const [confirmation, setConfirmation] = useState<{ action: KeyAction; key: SubscriptionToken } | null>(null);
  const [busy, setBusy] = useState(false);
  function resetForm() {
    setLabel('');
    setExpiry('never');
    setExpiresAt(null);
    setLimit('');
    setIssue(null);
  }
  useUnsavedChanges(creating && (label !== '' || expiry !== 'never' || limit !== ''), () => {
    setCreating(false);
    resetForm();
  }, busy);
  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(setCopied, 1500, false);
    return () => window.clearTimeout(timer);
  }, [copied]);
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
  function expiryDate(choice: Exclude<Expiry, 'never' | 'custom'>) {
    return new Date(Date.now() + Number(choice) * 86_400_000);
  }
  function expiryLabel(choice: Expiry) {
    if (choice === 'never') return t('subscriptions.keys.expiryNever');
    if (choice === 'custom') return t('subscriptions.keys.expiryCustom');
    if (choice === '365') return t('subscriptions.keys.expiryYear');
    return t('subscriptions.keys.expiryDays', { count: Number(choice) });
  }
  /** Saying why the form cannot be saved focuses the field at fault. */
  function refuse(field: 'name' | 'expiry' | 'quota', message: string) {
    setIssue({ field, message });
    if (field === 'name') nameRef.current?.focus();
    else if (field === 'quota') quotaRef.current?.focus();
    else document.getElementById('key-expiry')?.focus();
  }
  async function create() {
    if (busy) return;
    if (!label.trim()) return refuse('name', t('subscriptions.keys.nameRequired'));
    let expires: Date | undefined;
    if (expiry === 'custom') {
      if (!expiresAt) return refuse('expiry', t('subscriptions.keys.expiryMissing'));
      if (expiresAt.getTime() <= Date.now()) return refuse('expiry', t('subscriptions.keys.expiryPast'));
      expires = expiresAt;
    } else if (expiry !== 'never') {
      expires = expiryDate(expiry);
    }
    const quota = limit.trim();
    const downloadLimit = quota === '' ? undefined : Number(quota);
    if (downloadLimit !== undefined && (!/^\d+$/.test(quota) || downloadLimit < 1 || downloadLimit > 1_000_000_000)) {
      return refuse('quota', t('subscriptions.keys.quotaInvalid'));
    }
    setBusy(true);
    setIssue(null);
    try {
      const result = await client.createSubscriptionToken({
        label: label.trim(), expiresAt: expires?.toISOString(), downloadLimit,
      });
      setCreating(false);
      setIssued({ key: result, kind: 'created' });
      refresh();
      resetForm();
    } catch (reason) {
      setIssue({ message: describeRequestError(reason) });
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
        setIssued({ key: { metadata: result.created, token: result.token }, kind: 'rotated' });
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
      if (generation === operationRef.current) setIssued({ key: { metadata: key, token: secret.token }, kind: 'viewed' });
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
      if (generation !== operationRef.current) return;
      setCopied(true);
      toast.add({ title: t('subscriptions.token.secret.copied'), type: 'success' });
    } catch (reason) {
      if (generation === operationRef.current) report(reason);
    }
  }

  return (
    <section className='min-h-full min-w-0 flex h-full min-h-0 flex-col' aria-label={t('subscriptions.tabs.tokens')}>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='flex min-w-0 flex-1 items-center justify-end gap-2'>
          {/* Like every tab's add action: its label beside the icon, the icon alone on a phone. */}
          <Button
            aria-label={t('subscriptions.keys.create')}
            className='max-sm:w-(--control-height-sm) max-sm:px-0'
            disabled={busy}
            onClick={() => setCreating(true)}
            size='sm'
            variant='flat'
          >
            <CirclePlus aria-hidden='true' data-icon='inline-start' />
            <span className='max-sm:sr-only'>{t('subscriptions.keys.create')}</span>
          </Button>
        </div>
      </ToolbarActions>
      {error ? <ErrorNotice error={error} title={t('subscriptions.token.loadFailed')} /> : null}
      <div className='relative min-h-0 flex-1 overflow-auto flex flex-col *:[table]:flex-none' aria-busy={loading}>
        <table className='workspace-table w-full min-w-160 table-fixed border-collapse [&_td]:px-4 [&_td]:text-center [&_td]:whitespace-nowrap [&_td:first-child]:max-w-60 [&_td:first-child]:truncate [&_td:first-child]:text-left [&_tbody_tr]:border-b [&_th]:px-4 [&_th]:text-center [&_th]:whitespace-nowrap [&_th:first-child]:text-left [&_th:nth-child(1)]:w-[30%] [&_th:nth-child(2)]:w-[20%] [&_th:nth-child(3)]:w-[15%] [&_th:nth-child(4)]:w-[15%] [&_th:nth-child(5)]:w-[20%] [&_th]:sticky [&_th]:top-0 [&_th]:z-1 [&_th]:bg-[var(--color-surface-raised)] [&>thead]:text-muted-foreground'>
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
                  <IconAction disabled={busy || loading} label={t('subscriptions.keys.view')} onClick={() => void reveal(key)}>
                    <Eye aria-hidden='true' />
                  </IconAction>
                  <IconAction
                    disabled={busy || loading || !!key.revoked_at}
                    label={t(key.enabled ? 'subscriptions.common.disable' : 'subscriptions.common.enable')}
                    onClick={() => void run('toggle', key)}
                  >
                    {key.enabled ? <CirclePause aria-hidden='true' /> : <CirclePlay aria-hidden='true' />}
                  </IconAction>
                  <IconAction
                    disabled={busy || loading || !!key.revoked_at}
                    label={t('subscriptions.token.action.rotate.name')}
                    onClick={() => setConfirmation({ action: 'rotate', key })}
                  >
                    <RefreshCw aria-hidden='true' />
                  </IconAction>
                  <IconAction disabled={busy || loading} label={t('subscriptions.token.action.delete.name')} onClick={() => setConfirmation({ action: 'delete', key })}>
                    <Trash2 aria-hidden='true' />
                  </IconAction>
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.keys.create')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('subscriptions.keys.shared')}</DialogDescription>
          </DialogHeader>
          <form className='contents' noValidate onSubmit={event => {
            event.preventDefault();
            void create();
          }}>
            <DialogFields>
              <label htmlFor='key-name'>{t('subscriptions.common.name')}</label>
              <Input
                aria-invalid={issue?.field === 'name' || undefined}
                id='key-name'
                maxLength={128}
                onChange={event => {
                  setLabel(event.target.value);
                  setIssue(null);
                }}
                ref={nameRef}
                value={label}
              />
              <label htmlFor='key-expiry'>{t('subscriptions.keys.expiry')}</label>
              {expiry === 'custom'
                ? (
                    <DateTimeField
                      clearLabel={t('subscriptions.keys.backToPresets')}
                      defaultOpen
                      id='key-expiry'
                      invalid={issue?.field === 'expiry'}
                      min={earliest}
                      onChange={(value) => {
                        setExpiresAt(value);
                        setIssue(null);
                      }}
                      onClear={() => {
                        setExpiry('never');
                        setExpiresAt(null);
                        setIssue(null);
                      }}
                      value={expiresAt}
                    />
                  )
                : (
                    <Select<Expiry>
                      items={expiries.map(value => ({ value, label: expiryLabel(value) }))}
                      onValueChange={(next) => {
                        if (next === null) return;
                        if (next === 'custom') setEarliest(new Date());
                        setExpiry(next);
                        setIssue(null);
                      }}
                      value={expiry}
                    >
                      <SelectTrigger id='key-expiry'>
                        <SelectValue />
                        {expiry === 'never'
                          ? null
                          : (
                              <span className='ms-auto flex-none ps-2 text-xs text-muted-foreground'>
                                {t('subscriptions.keys.expiresOn', { date: expiryDate(expiry).toLocaleDateString(i18n.resolvedLanguage, { dateStyle: 'medium' }) })}
                              </span>
                            )}
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {expiries.map(value => (
                            <SelectItem key={value} value={value}>{expiryLabel(value)}</SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  )}
              <label htmlFor='key-quota'>{t('subscriptions.keys.quota')}</label>
              <InputGroup>
                <InputGroupInput
                  aria-invalid={issue?.field === 'quota' || undefined}
                  id='key-quota'
                  inputMode='numeric'
                  onChange={event => {
                    setLimit(event.target.value);
                    setIssue(null);
                  }}
                  placeholder={t('subscriptions.keys.unlimited')}
                  ref={quotaRef}
                  value={limit}
                />
                <InputGroupAddon align='inline-end'>
                  <InputGroupText>{t('subscriptions.keys.quotaUnit')}</InputGroupText>
                </InputGroupAddon>
              </InputGroup>
            </DialogFields>
            <DialogFooter message={issue?.message ?? ''}>
              <Button type='button' variant='outline' disabled={busy} onClick={() => setCreating(false)}>{t('subscriptions.keys.cancel')}</Button>
              <Button aria-busy={busy || undefined} type='submit'>{t('subscriptions.keys.create')}</Button>
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t(issued?.kind === 'created' ? 'subscriptions.keys.created' : issued?.kind === 'rotated' ? 'subscriptions.keys.rotated' : 'subscriptions.keys.details')}
            </DialogTitle>
            <DialogDescription className={issued?.kind === 'created' ? undefined : 'sr-only'}>
              {t(issued?.kind === 'created' ? 'subscriptions.keys.createdNote' : 'subscriptions.keys.key')}
            </DialogDescription>
          </DialogHeader>
          {issued
            ? (
                <>
                  <DialogFields className='[&>span]:min-w-0 [&>span]:truncate [&>span]:leading-10'>
                    <span>{t('subscriptions.common.name')}</span>
                    <span>{issued.key.metadata.label}</span>
                    <span>{t('subscriptions.keys.expiry')}</span>
                    <span>{formatDate(issued.key.metadata.expires_at)}</span>
                    <span>{t('subscriptions.keys.quota')}</span>
                    <span className='tabular-nums'>
                      {issued.key.metadata.download_limit === undefined
                        ? t('subscriptions.keys.unlimited')
                        : `${issued.key.metadata.download_limit.toLocaleString(i18n.resolvedLanguage)} ${t('subscriptions.keys.quotaUnit')}`}
                    </span>
                    <span>{t('subscriptions.keys.key')}</span>
                    <div className='flex h-(--control-height) min-w-0 items-center gap-1 rounded-lg bg-[var(--color-surface-raised)] ps-3 pe-1'>
                      <code className='min-w-0 flex-1 truncate font-[family-name:var(--font-data)] text-compact' title={issued.key.token}>{issued.key.token}</code>
                      <IconAction
                        className={copied ? 'text-[var(--color-success-text)] hover:text-[var(--color-success-text)]' : undefined}
                        label={t(copied ? 'subscriptions.token.secret.copied' : 'subscriptions.token.secret.copy')}
                        onClick={() => void copy(issued.key.token)}
                      >
                        {copied ? <Check aria-hidden='true' /> : <Copy aria-hidden='true' />}
                      </IconAction>
                    </div>
                  </DialogFields>
                  <DialogFooter>
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

      <Dialog open={confirmation !== null} onOpenChange={open => {
        if (!open && !busy) setConfirmation(null);
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{confirmation ? t(`subscriptions.token.action.${confirmation.action}.title`) : ''}</DialogTitle>
            <DialogDescription>
              {confirmation ? t(`subscriptions.token.action.${confirmation.action}.prompt`, { name: confirmation.key.label }) : ''}
            </DialogDescription>
          </DialogHeader>
          {confirmation
            ? (
                <DialogFooter>
                  <Button variant='outline' disabled={busy} onClick={() => setConfirmation(null)}>{t('subscriptions.keys.cancel')}</Button>
                  <Button variant={confirmation.action === 'delete' ? 'destructive' : 'default'} disabled={busy} onClick={() => void run(confirmation.action, confirmation.key)}>{t(`subscriptions.token.action.${confirmation.action}.name`)}</Button>
                </DialogFooter>
              )
            : null}
        </DialogContent>
      </Dialog>
    </section>
  );
}
