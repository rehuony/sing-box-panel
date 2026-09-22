import { useTranslation } from 'react-i18next';
import { CirclePlus, Search } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';

import type {
  SubscriptionChannel,
  SubscriptionChannelSummary,
  SubscriptionFormat,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { ChannelWorkspace } from './channel-workspace';
import { initialChannelPolicy } from './channel-policy';
import { ChannelLinkDialog } from './channel-token-links';

export function SubscriptionChannelPanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [channels, setChannels] = useState<SubscriptionChannelSummary[]>([]);
  const [nodes, setNodes] = useState<SubscriptionNodeSummary[]>([]);
  const [channel, setChannel] = useState<SubscriptionChannel | null>(null);
  const [search, setSearch] = useState('');
  const [size, setSize] = useState(10);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [creating, setCreating] = useState(false);
  const [linkChannel, setLinkChannel] = useState<SubscriptionChannel | null>(null);
  const [name, setName] = useState('');
  const [format, setFormat] = useState<SubscriptionFormat>('sing-box');
  const [formError, setFormError] = useState('');
  const [deleting, setDeleting] = useState<SubscriptionChannelSummary | null>(null);
  useUnsavedChanges(creating && (name !== '' || format !== 'sing-box'), () => {
    setCreating(false);
    setName('');
    setFormat('sing-box');
  }, busy);
  const lifetimeRef = useRef<AbortController | null>(null);
  const generationRef = useRef(0);
  const load = useCallback(
    async (signal?: AbortSignal) => {
      const request = ++generationRef.current;
      try {
        const all: SubscriptionChannelSummary[] = [];
        let cursor: { created_at: string; id: string } | undefined;
        do {
          const result = await client.listSubscriptionChannels(
            { limit: 100, beforeID: cursor?.id, beforeTime: cursor?.created_at },
            signal,
          );
          all.push(...result.items);
          cursor = result.next;
        } while (cursor && all.length < 10000 && !signal?.aborted);
        const catalog = await client.getSubscriptionNodeCatalog(signal);
        if (signal?.aborted || request !== generationRef.current) return;
        setChannels(all);
        setNodes(catalog.nodes);
        setError(null);
      } catch (reason) {
        if (!signal?.aborted && request === generationRef.current) setError(reason);
      }
    },
    [client],
  );
  useEffect(() => {
    const controller = new AbortController();
    lifetimeRef.current = controller;
    return () => {
      controller.abort();
      generationRef.current += 1;
    };
  }, []);
  useEffect(() => {
    if (active) void load(lifetimeRef.current?.signal);
  }, [active, load]);
  async function open(id: string) {
    if (busy) return;
    setBusy(true);
    try {
      const value = await client.getSubscriptionChannel(id, lifetimeRef.current?.signal);
      if (!lifetimeRef.current?.signal.aborted) setChannel(value);
    } catch (reason) {
      toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      setBusy(false);
    }
  }
  async function channelAction(id: string, action: 'duplicate' | 'link') {
    if (busy) return;
    setBusy(true);
    const signal = lifetimeRef.current?.signal;
    try {
      const fresh = await client.getSubscriptionChannel(id, signal);
      if (signal?.aborted) return;
      if (action === 'link') {
        setLinkChannel(fresh);
      } else {
        function nameForCopy(number: number) {
          const suffix = ` ${t('channels.duplicateSuffix')}${number > 1 ? ` ${number}` : ''}`;
          const characters = Array.from(fresh.name);
          while (new TextEncoder().encode(characters.join('') + suffix).length > 128) characters.pop();
          return characters.join('') + suffix;
        }
        let number = 1;
        let copyName = nameForCopy(number);
        while (channels.some((item) => item.name === copyName)) copyName = nameForCopy(++number);
        await client.createSubscriptionChannel({
          name: copyName, format: fresh.format, enabled: fresh.enabled,
          public_host: fresh.public_host, config: fresh.config,
        }, signal);
        if (signal?.aborted) return;
        await load(signal);
        if (!signal?.aborted) toast.add({ title: t('channels.duplicated'), type: 'success' });
      }
    } catch (reason) {
      if (!signal?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!signal?.aborted) setBusy(false);
    }
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
      const policy = initialChannelPolicy({ config: {} }, nodes);
      policy.selection.new_node_policy = 'exclude';
      const value = await client.createSubscriptionChannel(
        { name: name.trim(), format, enabled: true, public_host: '', config: { policy } },
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
      await load(lifetimeRef.current?.signal);
      toast.add({ title: t('channels.remove'), type: 'success' });
    } catch (reason) {
      toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      setBusy(false);
    }
  }
  const filtered = channels.filter((value) =>
    value.name.toLowerCase().includes(search.toLowerCase()),
  );
  const pages = Math.max(1, Math.ceil(filtered.length / size));
  const current = Math.min(page, pages);
  if (page !== current) setPage(current);
  return (
    <section className='subscription-source-workspace'>
      {linkChannel && (
        <ChannelLinkDialog
          channelID={linkChannel.id} tokenIDs={linkChannel.config.export_token_ids ?? []}
          onClose={() => setLinkChannel(null)}
        />
      )}
      {error != null && <ErrorNotice error={error} title={t('channels.title')} />}
      {channel
        ? (
            <ChannelWorkspace
              key={channel.id}
              active={active}
              toolbarTarget={toolbarTarget}
              channel={channel}
              nodes={nodes}
              onBack={() => {
                setChannel(null);
                void load(lifetimeRef.current?.signal);
              }}
              onSaved={setChannel}
            />
          )
        : (
            <>
              <ToolbarActions active={active} target={toolbarTarget}>
                <div className='subscription-source-toolbar workspace-toolbar-content'>
                  <div className='subscription-toolbar-actions'>
                    <Button
                      aria-label={t('channels.add')}
                      title={t('channels.add')}
                      disabled={busy}
                      size='icon-sm'
                      variant='ghost'
                      onClick={() => {
                        setCreating(true);
                        setName('');
                        setFormat('sing-box');
                        setFormError('');
                      }}
                    >
                      <CirclePlus aria-hidden='true' />
                    </Button>
                  </div>
                  <div className='subscription-search'>
                    <Search />
                    <input
                      aria-label={t('channels.search')}
                      placeholder={t('channels.search')}
                      value={search}
                      onChange={(event) => {
                        setSearch(event.target.value);
                        setPage(1);
                      }}
                    />
                  </div>
                </div>
              </ToolbarActions>
              <div className='subscription-source-table-scroll'>
                <table className='workspace-table subscription-source-table channel-list-table'>
                  <thead>
                    <tr>
                      <th>{t('channels.name')}</th>
                      <th>{t('channels.client')}</th>
                      <th>{t('channels.state')}</th>
                      <th>{t('channels.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.slice((current - 1) * size, current * size).map((value) => (
                      <tr key={value.id}>
                        <td>
                          <span className='block truncate' title={value.name}>{value.name}</span>
                        </td>
                        <td>
                          {value.format === 'sing-box'
                            ? t('subscriptions.channel.format.singBox')
                            : value.format === 'mihomo'
                              ? t('subscriptions.channel.format.mihomo')
                              : t('subscriptions.channel.format.loon')}
                        </td>
                        <td>{t(value.enabled ? 'channels.enabled' : 'channels.disabled')}</td>
                        <td>
                          <Button size='sm' disabled={busy} variant='outline' onClick={() => void open(value.id)}>
                            {t('channels.edit')}
                          </Button>
                          <Button size='sm' disabled={busy} variant='outline' onClick={() => void channelAction(value.id, 'duplicate')}>{t('channels.copy')}</Button>
                          <Button size='sm' disabled={busy || !value.enabled} variant='outline' onClick={() => void channelAction(value.id, 'link')}>{t('channels.link')}</Button>
                          <Button size='sm' disabled={busy} variant='destructive' onClick={() => setDeleting(value)}>
                            {t('channels.remove')}
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {!filtered.length && <p className='subscription-empty'>{t('channels.empty')}</p>}
              </div>
              <ListPagination
                page={current}
                pages={pages}
                pageSize={size}
                disabled={filtered.length === 0}
                onPageChange={setPage}
                onPageSizeChange={value => {
                  setSize(value);
                  setPage(1);
                }}
              />
            </>
          )}
      <Dialog open={creating} onOpenChange={(open) => !busy && setCreating(open)}>
        <DialogContent className='subscription-source-dialog'>
          <DialogHeader>
            <DialogTitle>{t('channels.add')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('channels.title')}</DialogDescription>
          </DialogHeader>
          <div className='subscription-settings-fields'>
            <label htmlFor='channel-name'>{t('channels.name')}</label>
            <input
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
