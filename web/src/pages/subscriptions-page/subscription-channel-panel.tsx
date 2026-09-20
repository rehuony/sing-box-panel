import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, Plus, Search } from 'lucide-react';

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
import { ToolbarActions } from '@/components/workspace-toolbar';
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
  const [name, setName] = useState('');
  const [format, setFormat] = useState<SubscriptionFormat>('sing-box');
  const [formError, setFormError] = useState('');
  const [deleting, setDeleting] = useState<SubscriptionChannelSummary | null>(null);
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
  return (
    <section className='subscription-source-workspace'>
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
              onRefresh={() => load(lifetimeRef.current?.signal)}
            />
          )
        : (
            <>
              <ToolbarActions active={active} target={toolbarTarget}>
                <div className='subscription-source-toolbar workspace-toolbar-content'>
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
                  <div className='subscription-toolbar-actions'>
                    <Button
                      aria-label={t('channels.add')}
                      disabled={busy}
                      size='icon'
                      variant='ghost'
                      onClick={() => {
                        setCreating(true);
                        setName('');
                        setFormat('sing-box');
                        setFormError('');
                      }}
                    >
                      <Plus />
                    </Button>
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
                          <button onClick={() => void open(value.id)} title={value.name}>
                            {value.name}
                          </button>
                        </td>
                        <td>
                          {value.format === 'sing-box'
                            ? 'sing-box JSON'
                            : value.format === 'mihomo'
                              ? 'Mihomo YAML'
                              : 'Loon'}
                        </td>
                        <td>{t(value.enabled ? 'channels.enabled' : 'channels.disabled')}</td>
                        <td>
                          <Button disabled={busy} variant='ghost' onClick={() => void open(value.id)}>
                            {t('channels.edit')}
                          </Button>
                          <Button disabled={busy} variant='ghost' onClick={() => setDeleting(value)}>
                            {t('channels.remove')}
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {!filtered.length && <p className='subscription-empty'>{t('channels.empty')}</p>}
              </div>
              <footer className='subscription-pagination'>
                <SelectField
                  aria-label={t('subscriptions.keys.pageSize')}
                  value={size}
                  onValueChange={(value) => {
                    setSize(value);
                    setPage(1);
                  }}
                  items={[5, 10, 50].map((value) => ({ value, label: t('subscriptions.keys.perPage', { count: value }) }))}
                />
                <div>
                  <Button
                    aria-label={t('subscriptions.keys.previous')}
                    variant='ghost'
                    size='icon'
                    disabled={current === 1}
                    onClick={() => setPage(current - 1)}
                  >
                    <ChevronLeft />
                  </Button>
                  <span aria-current='page'>{current}</span>
                  <Button
                    aria-label={t('subscriptions.keys.next')}
                    variant='ghost'
                    size='icon'
                    disabled={current === pages}
                    onClick={() => setPage(current + 1)}
                  >
                    <ChevronRight />
                  </Button>
                </div>
              </footer>
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
            <select
              id='channel-format'
              value={format}
              onChange={(event) => setFormat(event.target.value as SubscriptionFormat)}
            >
              <option value='sing-box'>sing-box JSON</option>
              <option value='mihomo'>Mihomo YAML</option>
            </select>
          </div>
          {formError && (
            <p role='alert' className='subscription-form-error'>
              {formError}
            </p>
          )}
          <DialogFooter>
            <Button disabled={busy} variant='secondary' onClick={() => setCreating(false)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={busy} variant='secondary' onClick={() => void create()}>
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
            <Button disabled={busy} variant='secondary' onClick={() => setDeleting(null)}>
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
