import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useDeferredValue, useEffect, useRef, useState } from 'react';
import { ArrowUpRight, CirclePlus, CopyPlus, Link2, MoreHorizontal, Trash2 } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@shared/components/ui/dropdown-menu';

import type {
  SubscriptionChannel,
  SubscriptionChannelSummary,
  SubscriptionFormat,
} from '@/api/api-client';

import { queries } from '@/api/queries';
import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { usePageVisible } from '@/hooks/use-page-visible';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { StripDivider, StripSearch } from '@/components/title-strip';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

import { BUILTIN_EXIT_NAMES } from './channel-exits';
import { emptyChannelConfig } from './channel-policy';
import { ChannelWorkspace } from './channel-workspace';
import { ChannelClientTag } from './channel-client-tag';
import { ChannelLinkPopover } from './channel-link-popover';
import { SubscriptionContentState } from './subscription-content-state';

/** Where a channel sends what no exit rule matches: DIRECT or REJECT as clients write them, else a group or node. */
function FinalExit({ exit }: { exit: SubscriptionChannelSummary['final_exit'] }) {
  if (exit.kind === 'direct' || exit.kind === 'reject') {
    return (
      <span className='font-[family-name:var(--font-data)] text-xs text-[var(--color-text-secondary)]'>
        {BUILTIN_EXIT_NAMES[exit.kind]}
      </span>
    );
  }
  return <span className='block truncate'>{exit.name ?? '—'}</span>;
}

export function SubscriptionChannelPanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const visible = usePageVisible();
  const list = useQuery({ ...queries.channels(client), enabled: active && visible });
  const catalog = useQuery({ ...queries.nodes(client), enabled: active && visible });
  const channels = list.data ?? [];
  const nodes = catalog.data?.items ?? [];
  const [channel, setChannel] = useState<SubscriptionChannel | null>(null);
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search);
  const error = list.error ?? catalog.error;
  // Only the dialogs wait on a request; a row's actions stay ready and ignore repeats.
  const [busy, setBusy] = useState(false);
  const pendingRef = useRef(new Set<string>());
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const [format, setFormat] = useState<SubscriptionFormat>('sing-box');
  const [formError, setFormError] = useState('');
  const [deleting, setDeleting] = useState<Pick<SubscriptionChannel, 'id' | 'name'> | null>(null);
  // The key the last link used, offered first by every channel's link.
  const [linkKey, setLinkKey] = useState('');
  const [flash, setFlash] = useState<string | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  useUnsavedChanges(creating && (name !== '' || format !== 'sing-box'), () => {
    setCreating(false);
    setName('');
    setFormat('sing-box');
  }, busy);
  const lifetimeRef = useRef<AbortController | null>(null);
  const load = useCallback(async (_signal?: AbortSignal) => {
    await cache.invalidateQueries({ queryKey: ['channels'], refetchType: 'none' });
    await cache.invalidateQueries({ queryKey: ['nodes'], refetchType: 'none' });
    await Promise.all([cache.fetchQuery(queries.channels(client)), cache.fetchQuery(queries.nodes(client))]);
  }, [client, cache]);
  useEffect(() => {
    const controller = new AbortController();
    lifetimeRef.current = controller;
    return () => controller.abort();
  }, []);
  // A new copy is marked where the list puts it, then the mark fades.
  useEffect(() => {
    if (!flash) return;
    listRef.current?.querySelector(`[data-channel-id="${CSS.escape(flash)}"]`)?.scrollIntoView({ block: 'nearest' });
    const timer = window.setTimeout(setFlash, 1200, null);
    return () => window.clearTimeout(timer);
  }, [flash]);
  async function once(key: string, action: (signal?: AbortSignal) => Promise<void>) {
    if (pendingRef.current.has(key)) return;
    pendingRef.current.add(key);
    const signal = lifetimeRef.current?.signal;
    try {
      await action(signal);
    } catch (reason) {
      if (!signal?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      pendingRef.current.delete(key);
    }
  }
  function open(id: string) {
    return once('open', async (signal) => {
      const value = await client.getSubscriptionChannel(id, signal);
      if (!signal?.aborted) setChannel(value);
    });
  }
  function duplicate(id: string) {
    return once(`duplicate:${id}`, async (signal) => {
      const fresh = await client.getSubscriptionChannel(id, signal);
      if (signal?.aborted) return;
      function nameForCopy(number: number) {
        const suffix = ` ${t('channels.duplicateSuffix')}${number > 1 ? ` ${number}` : ''}`;
        const characters = Array.from(fresh.name);
        while (new TextEncoder().encode(characters.join('') + suffix).length > 128) characters.pop();
        return characters.join('') + suffix;
      }
      let number = 1;
      let copyName = nameForCopy(number);
      while (channels.some((item) => item.name === copyName)) copyName = nameForCopy(++number);
      const created = await client.createSubscriptionChannel({
        name: copyName, format: fresh.format, enabled: fresh.enabled, config: fresh.config,
      }, signal);
      if (signal?.aborted) return;
      await load(signal);
      if (signal?.aborted) return;
      setFlash(created.id);
      toast.add({
        title: t('channels.duplicated', { name: created.name }),
        type: 'success',
        actionProps: { children: t('channels.openChannel'), onClick: () => setChannel(created) },
      });
    });
  }
  async function create() {
    if (busy) return;
    if (!name.trim()) {
      setFormError(t('channels.nameRequired'));
      return;
    }
    setBusy(true);
    setFormError('');
    try {
      const value = await client.createSubscriptionChannel(
        { name: name.trim(), format, enabled: true, config: emptyChannelConfig() },
        lifetimeRef.current?.signal,
      );
      if (lifetimeRef.current?.signal.aborted) return;
      setCreating(false);
      setChannel(value);
      await load(lifetimeRef.current?.signal);
      toast.add({ title: t('channels.saved'), type: 'success' });
    } catch (reason) {
      setFormError(describeRequestError(reason));
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!deleting || busy) return;
    setBusy(true);
    try {
      const fresh = await client.getSubscriptionChannel(deleting.id, lifetimeRef.current?.signal);
      await client.deleteSubscriptionChannel(
        fresh.id,
        fresh.updated_at,
        lifetimeRef.current?.signal,
      );
      if (lifetimeRef.current?.signal.aborted) return;
      setDeleting(null);
      // Deleting from its settings leaves the channel's workspace.
      if (channel?.id === fresh.id) setChannel(null);
      await load(lifetimeRef.current?.signal);
      toast.add({ title: t('channels.removed'), type: 'success' });
    } catch (reason) {
      toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      setBusy(false);
    }
  }
  const filtered = channels.filter((value) =>
    value.name.toLowerCase().includes(deferredSearch.toLowerCase()),
  );
  return (
    <section className='flex h-full min-h-0 min-w-0 flex-1 flex-col'>
      {error != null && <ErrorNotice error={error} title={t('channels.title')} />}
      {channel
        ? (
            <ChannelWorkspace
              key={channel.id}
              active={active}
              toolbarTarget={toolbarTarget}
              channel={channel}
              linkKey={linkKey}
              nodes={nodes}
              onBack={() => {
                setChannel(null);
                void load(lifetimeRef.current?.signal);
              }}
              onDelete={() => setDeleting(channel)}
              onLinkKeyChange={setLinkKey}
              onSaved={setChannel}
            />
          )
        : (
            <>
              <ToolbarActions active={active} target={toolbarTarget}>
                <div className='flex w-full min-w-0 flex-1 items-center gap-2'>
                  <StripSearch
                    label={t('channels.search')}
                    value={search}
                    onValueChange={setSearch}
                  />
                  <StripDivider />
                  <Button
                    aria-label={t('channels.add')}
                    className='max-sm:w-(--control-height-sm) max-sm:px-0'
                    size='sm'
                    variant='flat'
                    onClick={() => {
                      setCreating(true);
                      setName('');
                      setFormat('sing-box');
                      setFormError('');
                    }}
                  >
                    <CirclePlus aria-hidden='true' data-icon='inline-start' />
                    <span className='max-sm:sr-only'>{t('channels.add')}</span>
                  </Button>
                </div>
              </ToolbarActions>
              {/* The list scrolls inside the card under its header; a narrow card scrolls it sideways. */}
              <div className='flex min-h-0 flex-1 flex-col overflow-auto' aria-busy={list.isPending} ref={listRef}>
                <table className='w-full min-w-160 flex-none table-fixed border-collapse text-sm'>
                  <thead className='sticky top-0 z-1 bg-[var(--color-surface-raised)] text-muted-foreground'>
                    <tr className='h-11 *:px-4 *:text-start *:font-medium'>
                      <th>{t('channels.name')}</th>
                      <th className='w-28'>{t('channels.client')}</th>
                      <th className='w-20 text-end!'>{t('channels.nodes')}</th>
                      <th className='w-24 text-end!'>{t('channels.routes')}</th>
                      <th className='w-40'>{t('channels.finalExit')}</th>
                      <th className='w-24'><span className='sr-only'>{t('channels.actions')}</span></th>
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.map((value) => (
                      <tr
                        className={cn(
                          'h-11 cursor-pointer border-b transition-colors duration-[var(--dur-short)] ease-out *:px-4 hover:bg-[var(--color-hover)]',
                          flash === value.id && 'bg-accent hover:bg-accent',
                        )}
                        data-channel-id={value.id}
                        key={value.id}
                        onClick={(event) => {
                          // The row opens its channel; its buttons and their floating layers do their own work.
                          const target = event.target as Element;
                          if (!event.currentTarget.contains(target) || target.closest('button')) return;
                          void open(value.id);
                        }}
                      >
                        <td>
                          <button
                            className='block w-full cursor-pointer truncate rounded-[var(--radius-control)] text-start font-medium outline-none focus-visible:ring-1 focus-visible:ring-primary'
                            onClick={() => void open(value.id)}
                            type='button'
                          >
                            {value.name}
                          </button>
                        </td>
                        <td><ChannelClientTag format={value.format} /></td>
                        <td className={cn('text-end font-[family-name:var(--font-data)] text-compact tabular-nums', value.node_count === 0 && 'text-[var(--color-warning-text)]')}>
                          {value.node_count}
                        </td>
                        <td className='text-end font-[family-name:var(--font-data)] text-compact tabular-nums'>{value.route_count}</td>
                        <td className='text-compact'><FinalExit exit={value.final_exit} /></td>
                        <td>
                          <div className='flex items-center justify-end gap-0.5'>
                            <ChannelLinkPopover
                              channel={value}
                              keyID={linkKey}
                              onKeyChange={setLinkKey}
                              trigger={<IconAction className='data-popup-open:bg-[var(--color-hover)] data-popup-open:text-foreground' label={t('channels.link')} />}
                            >
                              <Link2 aria-hidden='true' />
                            </ChannelLinkPopover>
                            <DropdownMenu>
                              <DropdownMenuTrigger render={<IconAction className='data-popup-open:bg-[var(--color-hover)] data-popup-open:text-foreground' label={t('channels.more', { name: value.name })} />}>
                                <MoreHorizontal aria-hidden='true' />
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align='end' className='w-auto min-w-40'>
                                <DropdownMenuGroup>
                                  <DropdownMenuItem onClick={() => void open(value.id)}>
                                    <ArrowUpRight aria-hidden='true' />
                                    {t('channels.openChannel')}
                                  </DropdownMenuItem>
                                  <DropdownMenuItem onClick={() => void duplicate(value.id)}>
                                    <CopyPlus aria-hidden='true' />
                                    {t('channels.duplicate')}
                                  </DropdownMenuItem>
                                  <DropdownMenuItem onClick={() => setDeleting(value)} variant='destructive'>
                                    <Trash2 aria-hidden='true' />
                                    {t('channels.remove')}
                                  </DropdownMenuItem>
                                </DropdownMenuGroup>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {list.isPending && !filtered.length
                  ? <SubscriptionContentState loading />
                  : !list.isPending && !filtered.length && !error && <SubscriptionContentState kind={search ? 'search' : 'channels'} title={t(search ? 'channels.noMatchingChannels' : 'channels.empty')} />}
              </div>
            </>
          )}
      <Dialog open={creating} onOpenChange={(open) => !busy && setCreating(open)}>
        <DialogContent className='w-160 max-w-[calc(100vw-32px)] sm:max-w-160'>
          <DialogHeader>
            <DialogTitle>{t('channels.add')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
          </DialogHeader>
          <div className='grid grid-cols-[minmax(120px,1fr)_minmax(200px,2fr)] items-center gap-x-6 gap-y-(--form-field-gap) pt-2 pb-(--form-field-gap) max-[720px]:grid-cols-[minmax(100px,1fr)_minmax(0,2fr)] max-[720px]:gap-3 max-[480px]:grid-cols-1 max-[480px]:gap-2 [&_[aria-invalid=true]]:outline-1 [&_[aria-invalid=true]]:outline-[var(--color-danger)] [&_[data-slot=select-trigger]]:w-full [&_input:not([type=checkbox])]:w-full [&_input:not([type=checkbox])]:min-w-0 [&_input[type=checkbox]]:justify-self-start [&_textarea]:max-h-45 [&_textarea]:min-h-[90px] [&_textarea]:resize-y [&_textarea]:rounded-[var(--radius-control)] [&_textarea]:border-0 [&_textarea]:bg-[var(--color-surface-raised)] [&_textarea]:p-3'>
            <label htmlFor='channel-name'>{t('channels.name')}</label>
            <Input
              id='channel-name'
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
            <label htmlFor='channel-format'>{t('channels.client')}</label>
            <SelectField<SubscriptionFormat>
              id='channel-format'
              value={format}
              onValueChange={setFormat}
              items={[
                { value: 'sing-box', label: t('subscriptions.channel.format.singBox') },
                { value: 'mihomo', label: t('subscriptions.channel.format.mihomo') },
                { value: 'loon', label: t('subscriptions.channel.format.loon') },
              ]}
            />
          </div>
          {formError && (
            <ErrorNotice error={formError} />
          )}
          <DialogFooter>
            <Button disabled={busy} variant='outline' onClick={() => setCreating(false)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={busy} variant='default' onClick={() => void create()}>
              {t('channels.add')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={deleting != null} onOpenChange={(open) => !open && !busy && setDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('channels.remove')}</DialogTitle>
            <DialogDescription>
              {t('channels.deletePrompt', { name: deleting?.name })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button disabled={busy} variant='outline' onClick={() => setDeleting(null)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={busy} variant='destructive' onClick={() => void remove()}>
              {t('channels.remove')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
