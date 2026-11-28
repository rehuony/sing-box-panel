import type { ReactElement, ReactNode } from 'react';

import { encode } from 'uqr';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Check, Copy, Plus, QrCode } from 'lucide-react';
import { toast } from '@shared/components/ui/toast-manager';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { Popover, PopoverContent, PopoverTitle, PopoverTrigger } from '@shared/components/ui/popover';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';

import type { SubscriptionFormat, SubscriptionToken } from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { describeRequestError } from '@/components/error-notice';

import { buildPublicSubscriptionURL } from './public-subscription-url';

/** The tallest the panel grows, with its QR code shown; it opens on the side that fits it. */
const PANEL_HEIGHT = 320;

interface LinkChannel {
  id: string;
  name: string;
  format: SubscriptionFormat;
}

/**
 * A channel's subscription link, from the icon or strip action that opens it:
 * the key it uses, the link with copy and QR code, and a new key when no key
 * is active. It opens below its trigger, or above when only that fits, and
 * keeps that side and its corner while it grows or shrinks.
 */
export function ChannelLinkPopover({ channel, children, keyID, onKeyChange, trigger }: {
  channel: LinkChannel;
  /** The trigger's content. */
  children: ReactNode;
  /** The key last used for a link, picked again when it is still active. */
  keyID: string;
  onKeyChange: (id: string) => void;
  trigger: ReactElement;
}) {
  const [open, setOpen] = useState(false);
  const [side, setSide] = useState<'bottom' | 'top'>('bottom');
  const triggerRef = useRef<HTMLButtonElement>(null);
  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        const rect = triggerRef.current?.getBoundingClientRect();
        if (next && rect) {
          const below = window.innerHeight - rect.bottom;
          setSide(below >= PANEL_HEIGHT || below >= rect.top ? 'bottom' : 'top');
        }
        setOpen(next);
      }}
    >
      <PopoverTrigger ref={triggerRef} render={trigger}>{children}</PopoverTrigger>
      <PopoverContent
        align='end'
        className='w-[min(25rem,calc(100vw-2rem))] gap-0 p-0'
        collisionAvoidance={{ side: 'none', align: 'shift' }}
        side={side}
        sideOffset={6}
      >
        <LinkPanel channel={channel} keyID={keyID} onKeyChange={onKeyChange} side={side} />
      </PopoverContent>
    </Popover>
  );
}

function LinkPanel({ channel, keyID, onKeyChange, side }: {
  channel: LinkChannel;
  keyID: string;
  onKeyChange: (id: string) => void;
  side: 'bottom' | 'top';
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const keys = useActiveKeys();
  const key = keys.items.find(item => item.id === keyID) ?? keys.items[0];
  const [naming, setNaming] = useState(false);
  return (
    <>
      <div className='flex h-11 items-center gap-2 border-b px-4'>
        <PopoverTitle className='flex-none text-sm font-medium'>{t('channels.link')}</PopoverTitle>
        <span className='min-w-0 truncate font-[family-name:var(--font-data)] text-xs text-[var(--color-text-faint)]'>
          {`${channel.name} · ${channel.format}`}
        </span>
      </div>
      {keys.loading || keys.error != null || (!key && !naming)
        ? (
            <div className='flex h-16 items-center justify-center gap-3 px-4 text-compact text-[var(--color-text-faint)]'>
              {keys.loading
                ? t('common.loading')
                : keys.error != null
                  ? <span className='truncate text-[var(--color-danger-text)]'>{describeRequestError(keys.error)}</span>
                  : (
                      <>
                        {t('channels.noActiveKeys')}
                        <Button onClick={() => setNaming(true)} size='sm' type='button' variant='flat'>
                          <Plus aria-hidden='true' data-icon='inline-start' />
                          {t('channels.newKey')}
                        </Button>
                      </>
                    )}
            </div>
          )
        : key
          ? <KeyLink channel={channel} keyItem={key} keys={keys.items} onKeyChange={onKeyChange} side={side} />
          : (
              <NewKey
                onCancel={() => setNaming(false)}
                onCreate={async (label) => {
                  const created = await client.createSubscriptionToken({ label });
                  keys.add(created.metadata, created.token);
                  onKeyChange(created.metadata.id);
                  setNaming(false);
                  toast.add({ title: t('channels.keyCreated', { label: created.metadata.label }), type: 'success' });
                }}
              />
            )}
    </>
  );
}

/** The key picker and the link it makes, with copy and the QR code beside it. */
function KeyLink({ channel, keyItem, keys, onKeyChange, side }: {
  channel: LinkChannel;
  keyItem: SubscriptionToken;
  keys: SubscriptionToken[];
  onKeyChange: (id: string) => void;
  side: 'bottom' | 'top';
}) {
  const { t, i18n } = useTranslation();
  const client = useApiClient();
  const qrId = useId();
  const [qr, setQR] = useState(false);
  const [copied, setCopied] = useState(false);
  const secret = useQuery({
    queryKey: ['tokenSecret', keyItem.id],
    queryFn: ({ signal }) => client.getSubscriptionTokenSecret(keyItem.id, signal),
    // The secret is read for this panel only and leaves the cache with it.
    gcTime: 0, staleTime: Infinity, retry: false,
  });
  const url = secret.data ? buildPublicSubscriptionURL(secret.data.token, channel.id) : '';
  // Shown without its scheme; a long link keeps its start and its last characters.
  const shown = url.slice(url.indexOf('//') + 2);
  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(setCopied, 1500, false);
    return () => window.clearTimeout(timer);
  }, [copied]);
  function meta(token: SubscriptionToken) {
    const parts = [];
    if (token.download_limit) {
      parts.push(t('channels.keyRemaining', { count: Math.max(0, token.download_limit - token.body_response_count) }));
    }
    if (token.expires_at) {
      parts.push(t('channels.keyExpires', { date: new Date(token.expires_at).toLocaleDateString(i18n.resolvedLanguage, { dateStyle: 'medium' }) }));
    }
    return parts.length ? parts.join(' · ') : t('channels.keyUnlimited');
  }
  async function copy() {
    if (!url) {
      void secret.refetch();
      return;
    }
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      toast.add({ title: t('channels.linkCopied', { name: channel.name, key: keyItem.label }), type: 'success' });
    } catch {
      toast.add({ title: t('channels.copyFailed'), type: 'error' });
    }
  }
  // The code unfolds on the side away from the trigger, so the toggle stays where it was pressed.
  const up = side === 'top';
  const code = (
    <div
      className={cn(
        'grid transition-[grid-template-rows] duration-[var(--dur-short)] ease-[var(--ease-out)] motion-reduce:transition-none',
        qr ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]',
      )}
      id={qrId}
    >
      <div className='flex min-h-0 justify-center overflow-hidden'>
        <div
          className={cn(
            'transition-[opacity,scale,visibility] ease-[var(--ease-out)] motion-reduce:transition-none',
            up ? 'origin-bottom pb-2.5' : 'origin-top pt-2.5',
            qr ? 'visible scale-100 opacity-100 delay-60 duration-[var(--dur-short)]' : 'invisible scale-96 opacity-0 duration-[var(--dur-micro)]',
          )}
        >
          {url ? <QRCode label={t('channels.qrCode')} value={url} /> : <div className='size-37' />}
        </div>
      </div>
    </div>
  );
  return (
    <div className='flex flex-col px-4 py-3 text-compact'>
      <div className='grid grid-cols-[2.5rem_minmax(0,1fr)] items-center gap-x-3'>
        <span className='text-[var(--color-text-secondary)]' id={`${qrId}-key`}>{t('channels.linkKey')}</span>
        <Select<string>
          items={keys.map(token => ({ value: token.id, label: token.label }))}
          onValueChange={(next) => {
            if (next !== null) onKeyChange(next);
          }}
          value={keyItem.id}
        >
          <SelectTrigger aria-labelledby={`${qrId}-key`} className='h-(--control-height-sm) w-full ps-2.5 pe-2 text-compact md:text-compact'>
            <SelectValue className='flex-none' />
            <span className='ms-auto min-w-0 truncate text-xs text-[var(--color-text-faint)]'>{meta(keyItem)}</span>
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {keys.map(token => (
                <SelectItem className='text-compact' key={token.id} value={token.id}>
                  {token.label}
                  <span className='ms-auto ps-4 text-xs text-[var(--color-text-faint)]'>{meta(token)}</span>
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      {up ? code : null}
      <div className='mt-2.5 grid grid-cols-[2.5rem_minmax(0,1fr)] items-center gap-x-3'>
        <span className='text-[var(--color-text-secondary)]'>{t('channels.linkURL')}</span>
        <div className='flex h-(--control-height-sm) min-w-0 items-center gap-0.5 rounded-[var(--radius-control)] bg-[var(--color-surface-raised)] ps-2.5 pe-0.5'>
          <span
            className={cn('me-1 flex min-w-0 flex-1 font-[family-name:var(--font-data)] text-xs whitespace-nowrap', secret.isError && 'text-[var(--color-danger-text)]')}
            title={url || undefined}
          >
            {secret.isPending
              ? <span className='text-[var(--color-text-faint)]'>{t('common.loading')}</span>
              : secret.isError
                ? <span className='truncate'>{describeRequestError(secret.error)}</span>
                : (
                    <>
                      <span className='min-w-0 truncate'>{shown.slice(0, -10)}</span>
                      <span className='flex-none'>{shown.slice(-10)}</span>
                    </>
                  )}
          </span>
          <IconAction
            className={cn(copied && 'text-[var(--color-success-text)] hover:text-[var(--color-success-text)]')}
            label={t(copied ? 'channels.copied' : 'channels.copyLink')}
            onClick={() => void copy()}
            size='icon-xs'
          >
            {copied ? <Check aria-hidden='true' /> : <Copy aria-hidden='true' />}
          </IconAction>
          <IconAction
            aria-controls={qrId}
            aria-expanded={qr}
            aria-pressed={qr}
            className='aria-pressed:bg-[var(--color-hover)] aria-pressed:text-foreground'
            label={t(qr ? 'channels.hideQRCode' : 'channels.showQRCode')}
            onClick={() => setQR(!qr)}
            size='icon-xs'
          >
            <QrCode aria-hidden='true' />
          </IconAction>
        </div>
      </div>
      {up ? null : code}
    </div>
  );
}

/** The one field a new key needs here: its name. Expiry and quota stay in key management. */
function NewKey({ onCancel, onCreate }: { onCancel: () => void; onCreate: (label: string) => Promise<void> }) {
  const { t } = useTranslation();
  const id = useId();
  const [label, setLabel] = useState('');
  const [issue, setIssue] = useState('');
  const [busy, setBusy] = useState(false);
  async function create() {
    if (busy) return;
    if (!label.trim()) {
      setIssue(t('channels.keyNameRequired'));
      return;
    }
    setBusy(true);
    try {
      await onCreate(label.trim());
    } catch (reason) {
      setIssue(describeRequestError(reason));
      setBusy(false);
    }
  }
  return (
    <>
      <div className='grid grid-cols-[2.5rem_minmax(0,1fr)] items-center gap-x-3 px-4 pt-3 text-compact'>
        <label className='text-[var(--color-text-secondary)]' htmlFor={id}>{t('channels.keyName')}</label>
        <Input
          aria-invalid={issue ? true : undefined}
          autoFocus
          className='h-(--control-height-sm)'
          id={id}
          maxLength={128}
          onChange={(event) => {
            setLabel(event.target.value);
            setIssue('');
          }}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
              event.preventDefault();
              void create();
            }
          }}
          value={label}
        />
        <span className='col-start-2 h-5 truncate pt-0.5 text-xs text-[var(--color-danger-text)]' role='status'>{issue}</span>
      </div>
      <div className='flex justify-end gap-2 border-t px-3 py-2.5'>
        <Button onClick={onCancel} size='sm' type='button' variant='ghost'>{t('common.cancel')}</Button>
        <Button aria-busy={busy || undefined} onClick={() => void create()} size='sm' type='button'>{t('channels.createKey')}</Button>
      </div>
    </>
  );
}

/** The link as a QR code: dark modules on white whatever the theme, which is what scanners read. */
function QRCode({ label, value }: { label: string; value: string }) {
  const { path, size } = useMemo(() => {
    const { data, size } = encode(value, { ecc: 'M', border: 2 });
    let path = '';
    data.forEach((row, y) => {
      for (let x = 0; x < size; x++) {
        if (!row[x]) continue;
        let end = x;
        while (end + 1 < size && row[end + 1]) end++;
        path += `M${x} ${y}h${end - x + 1}v1h-${end - x + 1}z`;
        x = end;
      }
    });
    return { path, size };
  }, [value]);
  return (
    <svg
      aria-label={label}
      className='block size-37 rounded-[var(--radius-control)]'
      role='img'
      shapeRendering='crispEdges'
      viewBox={`0 0 ${size} ${size}`}
    >
      <rect fill='#fff' height={size} width={size} />
      <path d={path} fill='#000' />
    </svg>
  );
}

/** Every active key in the keys' order, read when the panel opens; a key made here joins them. */
function useActiveKeys() {
  const client = useApiClient();
  const cache = useQueryClient();
  const [state, setState] = useState<{ items: SubscriptionToken[]; loading: boolean; error?: unknown }>(
    { items: [], loading: true },
  );
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      const { items } = await client.listSubscriptionTokens(controller.signal);
      if (!controller.signal.aborted) setState({ items: items.filter(token => token.active), loading: false });
    }
    void load().catch((error: unknown) => {
      if (!controller.signal.aborted) setState({ items: [], loading: false, error });
    });
    return () => controller.abort();
  }, [client]);
  return {
    ...state,
    add(token: SubscriptionToken, secret: string) {
      cache.setQueryData(['tokenSecret', token.id], { token: secret });
      void cache.invalidateQueries({ queryKey: ['tokens'] });
      setState(current => ({ ...current, items: [...current.items, token] }));
    },
  };
}
