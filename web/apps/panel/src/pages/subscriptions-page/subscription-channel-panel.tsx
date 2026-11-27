import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useLocation, useNavigate } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useDeferredValue, useEffect, useRef, useState } from 'react';
import { ArrowUpRight, CirclePlus, CopyPlus, Link2, MoreHorizontal, Trash2 } from 'lucide-react';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@shared/components/ui/table';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFields,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@shared/components/ui/dropdown-menu';

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
import { useDialogProblem } from '@/hooks/use-dialog-problem';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';
import { StripAction, StripDivider, StripSearch } from '@/components/title-strip';
import { describeRequestError, ErrorNotice, toastFailure } from '@/components/error-notice';

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

/** The channels tab's address; an open channel's ID follows it after a slash. */
const CHANNELS_HASH = '#subscription-channels';

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
  const location = useLocation();
  const navigate = useNavigate();
  const { skipGuard } = useUnsavedChangesContext();
  // The address names the open channel, so reloading, or going back and forward, shows it again.
  const openID = location.hash.startsWith(`${CHANNELS_HASH}/`) ? decodeURIComponent(location.hash.slice(CHANNELS_HASH.length + 1)) : '';
  const [channel, setChannel] = useState<SubscriptionChannel | null>(null);
  const shown = openID !== '' && channel?.id === openID ? channel : null;
  // Whether the open channel was opened from the list here, so leaving it goes back to the list's entry.
  const fromListRef = useRef(false);
  const wasOpenRef = useRef(openID !== '');
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search);
  const error = list.error ?? catalog.error;
  // Only the dialogs wait on a request; a row's actions stay ready and ignore repeats.
  const [busy, setBusy] = useState(false);
  const pendingRef = useRef(new Set<string>());
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const [format, setFormat] = useState<SubscriptionFormat>('sing-box');
  const createProblem = useDialogProblem<'name'>(creating);
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
  const { pathname, search: query, state } = location;
  /** Shows a channel, or the list without one, keeping the rest of the address and its state. */
  const show = useCallback((id: string, replace = false) => navigate(
    { pathname, search: query, hash: id ? `${CHANNELS_HASH}/${encodeURIComponent(id)}` : CHANNELS_HASH },
    { replace, state },
  ), [navigate, pathname, query, state]);
  // The channel the address names is read when it is not the one at hand; one that cannot be read leaves for the list.
  const channelID = channel?.id;
  useEffect(() => {
    if (openID === '' || channelID === openID) return;
    const controller = new AbortController();
    client.getSubscriptionChannel(openID, controller.signal).then(setChannel, (reason) => {
      if (controller.signal.aborted) return;
      toastFailure(t('channels.openFailed'), reason);
      void show('', true);
    });
    return () => controller.abort();
  }, [channelID, client, openID, show, t]);
  // Leaving a channel reads the list again, which its edits may have changed.
  useEffect(() => {
    if (openID === '' && wasOpenRef.current) void load(lifetimeRef.current?.signal);
    wasOpenRef.current = openID !== '';
  }, [load, openID]);
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
      if (!signal?.aborted) toastFailure(t('channels.duplicateFailed'), reason);
    } finally {
      pendingRef.current.delete(key);
    }
  }
  function open(id: string) {
    fromListRef.current = true;
    void show(id);
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
        actionProps: { children: t('channels.openChannel'), onClick: () => open(created.id) },
      });
    });
  }
  async function create() {
    if (busy) return;
    if (!name.trim()) {
      createProblem.report(t('channels.nameRequired'), { field: 'name' });
      document.getElementById('channel-name')?.focus();
      return;
    }
    setBusy(true);
    createProblem.clear();
    try {
      const value = await client.createSubscriptionChannel(
        { name: name.trim(), format, enabled: true, config: emptyChannelConfig() },
        lifetimeRef.current?.signal,
      );
      if (lifetimeRef.current?.signal.aborted) return;
      setCreating(false);
      setChannel(value);
      fromListRef.current = true;
      // The form just saved is still dirty until it closes; it must not ask about opening its channel.
      skipGuard(() => void show(value.id));
      await load(lifetimeRef.current?.signal);
      toast.add({ title: t('channels.saved'), type: 'success' });
    } catch (reason) {
      createProblem.report(t('channels.createFailed'), { description: describeRequestError(reason) });
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
      // Deleting from its settings leaves the channel's workspace, whose edits went with it.
      if (shown?.id === fresh.id) skipGuard(() => void show('', true));
      await load(lifetimeRef.current?.signal);
      toast.add({ title: t('channels.removed'), type: 'success' });
    } catch (reason) {
      toastFailure(t('channels.removeFailed'), reason);
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
      {shown
        ? (
            <ChannelWorkspace
              key={shown.id}
              active={active}
              toolbarTarget={toolbarTarget}
              channel={shown}
              linkKey={linkKey}
              nodes={nodes}
              // Back returns to the list's entry when the channel was opened from it, and otherwise replaces this one.
              onBack={() => void (fromListRef.current ? navigate(-1) : show('', true))}
              onDelete={() => setDeleting(shown)}
              onLinkKeyChange={setLinkKey}
              onSaved={setChannel}
            />
          )
        : openID
          ? <SubscriptionContentState loading />
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
                    <StripAction
                      icon={<CirclePlus aria-hidden='true' data-icon='inline-start' />}
                      label={t('channels.add')}
                      onClick={() => {
                        setCreating(true);
                        setName('');
                        setFormat('sing-box');
                        createProblem.clear();
                      }}
                    />
                  </div>
                </ToolbarActions>
                {/* The list scrolls inside the card under its header; a narrow card scrolls it sideways. */}
                <div className='flex min-h-0 flex-1 flex-col overflow-auto' aria-busy={list.isPending} ref={listRef}>
                  <Table className='min-w-160'>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('channels.name')}</TableHead>
                        <TableHead className='w-28'>{t('channels.client')}</TableHead>
                        <TableHead className='w-20 text-end'>{t('channels.nodes')}</TableHead>
                        <TableHead className='w-24 text-end'>{t('channels.routes')}</TableHead>
                        <TableHead className='w-40'>{t('channels.finalExit')}</TableHead>
                        <TableHead className='w-24'><span className='sr-only'>{t('channels.actions')}</span></TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filtered.map((value) => (
                        <TableRow
                          className={cn(flash === value.id && 'bg-accent hover:bg-accent')}
                          data-channel-id={value.id}
                          interactive
                          key={value.id}
                          onClick={(event) => {
                          // The row opens its channel; its buttons and their floating layers do their own work.
                            const target = event.target as Element;
                            if (!event.currentTarget.contains(target) || target.closest('button')) return;
                            open(value.id);
                          }}
                        >
                          <TableCell>
                            <button
                              className='block w-full cursor-pointer truncate text-start font-medium outline-none'
                              data-row-title=''
                              onClick={() => open(value.id)}
                              type='button'
                            >
                              {value.name}
                            </button>
                          </TableCell>
                          <TableCell><ChannelClientTag format={value.format} /></TableCell>
                          <TableCell className={cn(value.node_count === 0 && 'text-[var(--color-warning-text)]')} kind='number'>
                            {value.node_count}
                          </TableCell>
                          <TableCell kind='number'>{value.route_count}</TableCell>
                          <TableCell className='text-compact'><FinalExit exit={value.final_exit} /></TableCell>
                          <TableCell kind='actions'>
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
                                  <DropdownMenuItem onClick={() => open(value.id)}>
                                    <ArrowUpRight aria-hidden='true' />
                                    {t('channels.openChannel')}
                                  </DropdownMenuItem>
                                  <DropdownMenuItem onClick={() => void duplicate(value.id)}>
                                    <CopyPlus aria-hidden='true' />
                                    {t('channels.duplicate')}
                                  </DropdownMenuItem>
                                </DropdownMenuGroup>
                                <DropdownMenuSeparator />
                                <DropdownMenuGroup>
                                  <DropdownMenuItem onClick={() => setDeleting(value)} variant='destructive'>
                                    <Trash2 aria-hidden='true' />
                                    {t('channels.remove')}
                                  </DropdownMenuItem>
                                </DropdownMenuGroup>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  {list.isPending && !filtered.length
                    ? <SubscriptionContentState loading />
                    : !list.isPending && !filtered.length && !error && <SubscriptionContentState kind={search ? 'search' : 'channels'} title={t(search ? 'channels.noMatchingChannels' : 'channels.empty')} />}
                </div>
              </>
            )}
      <Dialog open={creating} onOpenChange={(open) => !busy && setCreating(open)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('channels.add')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
          </DialogHeader>
          <form className='contents' noValidate onSubmit={(event) => {
            event.preventDefault();
            void create();
          }}>
            <DialogFields>
              <label htmlFor='channel-name'>{t('channels.name')}</label>
              <Input
                aria-invalid={createProblem.invalid('name')}
                id='channel-name'
                value={name}
                onChange={(event) => {
                  setName(event.target.value);
                  createProblem.clear();
                }}
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
            </DialogFields>
            <DialogFooter>
              <Button disabled={busy} type='button' variant='outline' onClick={() => setCreating(false)}>
                {t('common.cancel')}
              </Button>
              <Button aria-busy={busy || undefined} type='submit' variant='default'>
                {t('channels.add')}
              </Button>
            </DialogFooter>
          </form>
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
