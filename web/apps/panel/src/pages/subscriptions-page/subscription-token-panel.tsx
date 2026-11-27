import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { EM_DASH } from '@shared/lib/telemetry-format';
import { toast } from '@shared/components/ui/toast-manager';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@shared/components/ui/table';
import { InputGroup, InputGroupAddon, InputGroupInput, InputGroupText } from '@shared/components/ui/input-group';
import { Check, CirclePlus, Copy, KeyRound, MoreHorizontal, Power, PowerOff, RefreshCw, Trash2 } from 'lucide-react';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';
import { Dialog, DialogContent, DialogDescription, DialogFields, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@shared/components/ui/dropdown-menu';

import type { CreatedSubscriptionToken, SubscriptionToken } from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { DateTimeField } from '@/components/date-time-field';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError, ErrorNotice, toastFailure } from '@/components/error-notice';

import type { useSubscriptionTokenPage } from './use-subscription-token-page';

import { SubscriptionContentState } from './subscription-content-state';

type KeyAction = 'rotate' | 'delete';
/** What a key action failed to do. */
type KeyFailure = KeyAction | 'enable' | 'disable' | 'reveal' | 'copy';

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

  /** A failed key action, titled by the action. */
  const report = useCallback((failure: KeyFailure, reason: unknown) => toastFailure(t(`subscriptions.keys.failed.${failure}`), reason), [t]);

  /** A key's state and the colour its dot takes: usable, used up or expired, paused, or revoked. */
  function keyState(key: SubscriptionToken): { label: string; tone: string } {
    if (key.revoked_at) return { label: t('subscriptions.token.state.revoked'), tone: 'text-[var(--color-danger-text)]' };
    if (!key.enabled) return { label: t('subscriptions.token.state.disabled'), tone: 'text-muted-foreground' };
    if (key.download_limit !== undefined && key.body_response_count >= key.download_limit) {
      return { label: t('subscriptions.keys.exhausted'), tone: 'text-[var(--color-warning-text)]' };
    }
    return key.active
      ? { label: t('subscriptions.token.state.active'), tone: 'text-[var(--color-success-text)]' }
      : { label: t('subscriptions.token.state.expired'), tone: 'text-[var(--color-warning-text)]' };
  }
  function formatTime(value: string) {
    return new Date(value).toLocaleString(i18n.resolvedLanguage, { dateStyle: 'medium', timeStyle: 'short' });
  }
  function formatDate(value?: string) {
    return value ? formatTime(value) : t('subscriptions.keys.unlimited');
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
    // What the action does to this key, which its toasts name.
    const outcome = action === 'toggle' ? (key.enabled ? 'disable' : 'enable') : action;
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
      // A rotated key shows its new secret instead.
      if (outcome !== 'rotate') toast.add({ title: t(`subscriptions.keys.changed.${outcome}`, { name: key.label }), type: 'success' });
    } catch (reason) {
      report(outcome, reason);
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
      if (generation === operationRef.current) report('reveal', reason);
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
      if (generation === operationRef.current) report('copy', reason);
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
      {/*
        One short reading a cell: a row opens the key's details, its state and dates read from the start, its
        downloads end the readings and its actions end the row; a phone keeps the name, state and actions.
      */}
      <div className='relative flex min-h-0 flex-1 flex-col overflow-auto' aria-busy={loading}>
        <Table className='sm:min-w-236'>
          <TableHeader>
            <TableRow>
              <TableHead className='sm:w-64'>{t('subscriptions.common.name')}</TableHead>
              <TableHead className='w-24 sm:w-28'>{t('subscriptions.common.state')}</TableHead>
              <TableHead className='w-48 max-sm:hidden'>{t('subscriptions.keys.lastUsed')}</TableHead>
              <TableHead className='w-48 max-sm:hidden'>{t('subscriptions.keys.expiry')}</TableHead>
              <TableHead className='w-28 text-end max-sm:hidden'>{t('subscriptions.keys.downloads')}</TableHead>
              <TableHead className='max-sm:w-24'><span className='sr-only'>{t('subscriptions.common.actions')}</span></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((key) => {
              const state = keyState(key);
              const lastUsed = key.last_used_at ? formatTime(key.last_used_at) : EM_DASH;
              const expires = formatDate(key.expires_at);
              const revoked = Boolean(key.revoked_at);
              return (
                <TableRow
                  interactive
                  key={key.id}
                  onClick={(event) => {
                    // The row opens the key's details; its buttons and their floating layers do their own work.
                    const target = event.target as Element;
                    if (event.currentTarget.contains(target) && !target.closest('button')) void reveal(key);
                  }}
                >
                  <TableCell>
                    <button
                      aria-haspopup='dialog'
                      className='block w-full cursor-pointer truncate text-start font-medium outline-none'
                      data-row-title=''
                      onClick={() => void reveal(key)}
                      title={key.label}
                      type='button'
                    >
                      {key.label}
                    </button>
                  </TableCell>
                  <TableCell>
                    <span className={cn('inline-flex items-center gap-1.5 text-xs font-medium whitespace-nowrap', state.tone)}>
                      <span aria-hidden='true' className='size-1.5 flex-none rounded-full bg-current' />
                      {state.label}
                    </span>
                  </TableCell>
                  <TableCell className='max-sm:hidden' kind='data' title={lastUsed}>{lastUsed}</TableCell>
                  <TableCell
                    className={cn('max-sm:hidden', !key.expires_at && 'font-[family-name:var(--font-body)] text-sm')}
                    kind='data'
                    title={expires}
                  >
                    {expires}
                  </TableCell>
                  <TableCell className='max-sm:hidden' kind='number'>{usage(key)}</TableCell>
                  <TableCell kind='actions'>
                    {/* A revoked key can only be deleted. */}
                    {revoked
                      ? null
                      : (
                          <IconAction
                            disabled={busy || loading}
                            label={t(key.enabled ? 'subscriptions.common.disable' : 'subscriptions.common.enable')}
                            onClick={() => void run('toggle', key)}
                          >
                            {key.enabled ? <PowerOff aria-hidden='true' /> : <Power aria-hidden='true' />}
                          </IconAction>
                        )}
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={(
                          <IconAction
                            className='data-popup-open:bg-[var(--color-hover)] data-popup-open:text-foreground'
                            label={t('subscriptions.keys.more', { name: key.label })}
                          />
                        )}
                      >
                        <MoreHorizontal aria-hidden='true' />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align='end' className='w-auto min-w-36'>
                        <DropdownMenuGroup>
                          {revoked
                            ? null
                            : (
                                <DropdownMenuItem onClick={() => setConfirmation({ action: 'rotate', key })}>
                                  <RefreshCw aria-hidden='true' />
                                  {t('subscriptions.token.action.rotate.name')}
                                </DropdownMenuItem>
                              )}
                          <DropdownMenuItem onClick={() => setConfirmation({ action: 'delete', key })} variant='destructive'>
                            <Trash2 aria-hidden='true' />
                            {t('subscriptions.token.action.delete.name')}
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
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
          {issued
            ? (
                <IssuedKey
                  copied={copied}
                  formatDate={formatDate}
                  formatTime={formatTime}
                  issued={issued}
                  onCopy={() => void copy(issued.key.token)}
                  onDone={() => {
                    setIssued(null);
                    operationRef.current++;
                  }}
                  state={keyState(issued.key.metadata)}
                  usage={usage}
                />
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

/** A fact of the key: its label above its value, the value in the data face when it is data. */
function KeyFact({ label, value, data = false }: { label: string; value: string; data?: boolean }) {
  return (
    <div className='min-w-0'>
      <dt className='text-xs text-[var(--color-text-faint)]'>{label}</dt>
      <dd className={cn('truncate text-compact text-[var(--color-text-secondary)]', data && 'font-[family-name:var(--font-data)] tabular-nums')} title={value}>
        {value}
      </dd>
    </div>
  );
}

/**
 * A key's secret and what governs it. Viewed, the key's name heads it with its
 * state, the secret follows on one line with its copy action, then when it was
 * made and last used, its expiry and downloads. Created or rotated, a mark and
 * the outcome head it over the key's name, the secret follows, and Done
 * acknowledges it.
 */
function IssuedKey({ issued, state, copied, onCopy, onDone, formatDate, formatTime, usage }: {
  issued: { key: CreatedSubscriptionToken; kind: 'created' | 'rotated' | 'viewed' };
  state: { label: string; tone: string };
  copied: boolean;
  onCopy: () => void;
  onDone: () => void;
  formatDate: (value?: string) => string;
  formatTime: (value: string) => string;
  usage: (key: SubscriptionToken) => string;
}) {
  const { t } = useTranslation();
  const key = issued.key.metadata;
  const secret = (
    // One line, cut at its end; the full key reads on hover and Copy takes all of it. Without
    // min-w-0 the dialog's grid would widen to the whole key instead.
    <div className='flex h-11 min-w-0 items-center gap-3 rounded-lg border bg-[var(--color-surface-raised)] ps-3.5 pe-1.5'>
      <code className='min-w-0 flex-1 truncate font-[family-name:var(--font-data)] text-compact' title={issued.key.token}>{issued.key.token}</code>
      <Button
        className={cn('flex-none', copied && 'text-[var(--color-success-text)] hover:text-[var(--color-success-text)]')}
        onClick={onCopy}
        size='sm'
        type='button'
        variant='outline'
      >
        {copied ? <Check aria-hidden='true' data-icon='inline-start' /> : <Copy aria-hidden='true' data-icon='inline-start' />}
        {t(copied ? 'subscriptions.token.secret.copied' : 'subscriptions.token.secret.copy')}
      </Button>
    </div>
  );
  if (issued.kind === 'viewed') {
    return (
      <>
        <DialogHeader>
          <DialogTitle className='flex min-w-0 items-center gap-2.5'>
            <span className='min-w-0 truncate'>{key.label}</span>
            <span className={cn('inline-flex flex-none items-center gap-1.5 text-xs font-medium whitespace-nowrap', state.tone)}>
              <span aria-hidden='true' className='size-1.5 rounded-full bg-current' />
              {state.label}
            </span>
          </DialogTitle>
          <DialogDescription className='sr-only'>{t('subscriptions.keys.key')}</DialogDescription>
        </DialogHeader>
        {secret}
        <dl className='grid grid-cols-2 gap-x-4 gap-y-3.5 max-[520px]:grid-cols-1'>
          <KeyFact data label={t('subscriptions.keys.createdAt')} value={formatTime(key.created_at)} />
          <KeyFact data label={t('subscriptions.keys.lastUsed')} value={key.last_used_at ? formatTime(key.last_used_at) : EM_DASH} />
          <KeyFact data={Boolean(key.expires_at)} label={t('subscriptions.keys.expiry')} value={formatDate(key.expires_at)} />
          <KeyFact data label={t('subscriptions.keys.downloads')} value={usage(key)} />
        </dl>
      </>
    );
  }
  return (
    <>
      <DialogHeader className='flex-row items-center gap-3.5 pe-9'>
        <span aria-hidden='true' className='flex size-10 flex-none items-center justify-center rounded-lg border border-[var(--color-success-border)] bg-[var(--color-success-soft)] text-[var(--color-success-text)]'>
          <KeyRound className='size-5' />
        </span>
        <div className='grid min-w-0 gap-0.5'>
          <DialogTitle className='pr-0'>{t(issued.kind === 'created' ? 'subscriptions.keys.created' : 'subscriptions.keys.rotated')}</DialogTitle>
          <DialogDescription className='truncate text-compact'>
            {issued.kind === 'created' ? key.label : `${key.label} · ${t('subscriptions.keys.rotatedNote')}`}
          </DialogDescription>
        </div>
      </DialogHeader>
      {secret}
      <DialogFooter>
        <Button onClick={onDone}>{t('subscriptions.keys.done')}</Button>
      </DialogFooter>
    </>
  );
}
