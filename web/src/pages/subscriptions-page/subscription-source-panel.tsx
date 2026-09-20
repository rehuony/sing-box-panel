import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, Plus, RefreshCw, Search } from 'lucide-react';

import type {
  SubscriptionNodeSummary,
  SubscriptionSource,
  SubscriptionSourceSummary,
} from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { waitForTask } from '@/lib/wait-for-task';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { SubscriptionNodeGrid } from './subscription-node-grid';
import { SubscriptionNodeEditor } from './subscription-node-editor';

interface SourceForm {
  url: string;
  name: string;
  format: string;
  interval: string;
  source?: SubscriptionSource;
}
function newSource(): SourceForm {
  return {
    name: '',
    url: '',
    format: 'auto',
    interval: '360',
  };
}

function latestStart(...values: (string | undefined)[]): string | undefined {
  return values.filter((value): value is string => value !== undefined && Number.isFinite(Date.parse(value)))
    .sort((a, b) => Date.parse(b) - Date.parse(a))[0];
}

export function SubscriptionSourcePanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t, i18n } = useTranslation();
  const client = useApiClient();
  const telemetry = useOptionalSharedTelemetry();
  const [lastStartedAt, setLastStartedAt] = useState<string>();
  const runningStartedAt = telemetry?.runtimeStatus?.running?.started_at;
  const latestStartedAt = latestStart(runningStartedAt, lastStartedAt);
  if (latestStartedAt !== lastStartedAt) setLastStartedAt(latestStartedAt);
  const addSourceRef = useRef<HTMLButtonElement>(null);
  const [sources, setSources] = useState<SubscriptionSourceSummary[]>([]);
  const [nodes, setNodes] = useState<SubscriptionNodeSummary[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [tab, setTab] = useState<'nodes' | 'settings'>('nodes');
  const [search, setSearch] = useState('');
  const [size, setSize] = useState(10);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [editor, setEditor] = useState<{ node: SubscriptionNodeSummary | null } | null>(null);
  const [form, setForm] = useState<SourceForm | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<SubscriptionSource | null>(null);
  const [formError, setFormError] = useState('');
  const lifetimeRef = useRef<AbortController | null>(null);
  const requestRef = useRef(0);
  const load = useCallback(
    async (signal?: AbortSignal) => {
      const generation = ++requestRef.current;
      void client.getRuntimeHistory({ state: 'running', limit: 1 }, signal).then((history) => {
        if (!signal?.aborted && generation === requestRef.current) {
          setLastStartedAt(current => latestStart(current, history.items[0]?.process_started_at));
        }
      }).catch(() => {
        // Keep the last confirmed start time when history is temporarily unavailable.
      });
      try {
        const all: SubscriptionSourceSummary[] = [];
        let cursor: { created_at: string; id: string } | undefined;
        do {
          const result = await client.listSubscriptionSources(
            {
              limit: 100,
              beforeID: cursor?.id,
              beforeTime: cursor?.created_at,
            },
            signal,
          );
          all.push(...result.items);
          cursor = result.next;
        } while (cursor && all.length < 10_000 && !signal?.aborted);
        const catalog = await client.getSubscriptionNodeCatalog(signal);
        if (!signal?.aborted && generation === requestRef.current) {
          setSources(all);
          setNodes(catalog.nodes);
          setError(null);
        }
      } catch (reason) {
        if (!signal?.aborted && generation === requestRef.current) setError(reason);
      }
    },
    [client],
  );
  useEffect(() => {
    const controller = new AbortController();
    lifetimeRef.current = controller;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      await load(controller.signal);
      if (!controller.signal.aborted) timer = setTimeout(() => void poll(), 15000);
    };
    void poll();
    return () => {
      clearTimeout(timer);
      controller.abort();
      requestRef.current += 1;
    };
  }, [load]);
  const signal = () => lifetimeRef.current?.signal;
  const reload = () => void load(signal());

  function openSource(id: string) {
    setSelected(id);
    setTab('nodes');
    setSearch('');
    setForm(null);
    setFormError('');
  }
  async function openSettings() {
    if (!selected || selected === 'manual') return;
    setBusy(true);
    setFormError('');
    try {
      const source = await client.getSubscriptionSource(selected, signal());
      if (signal()?.aborted) return;
      setForm({
        source,
        name: source.name,
        url: typeof source.config.url === 'string' ? source.config.url : '',
        format: typeof source.config.format === 'string' ? source.config.format : 'auto',
        interval: String(source.config.refresh_interval_minutes ?? 0),
      });
      setTab('settings');
    } catch (reason) {
      if (!signal()?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!signal()?.aborted) setBusy(false);
    }
  }
  async function refresh(sourceIDs: string[]) {
    if (busy) return;
    setBusy(true);
    try {
      const results = await Promise.allSettled(
        sourceIDs.map(async (id) => {
          const currentSignal = signal();
          if (!currentSignal) return;
          const task = await client.refreshSubscriptionSource(id, currentSignal);
          const completed = await waitForTask(client, task, currentSignal);
          if (completed.status !== 'succeeded') throw new Error(t('subscriptions.sources.refreshFailed'));
        }),
      );
      if (signal()?.aborted) return;
      const failed = results.filter((value) => value.status === 'rejected').length;
      toast.add({
        title: failed
          ? t('subscriptions.sources.refreshFailed')
          : t('subscriptions.sources.refreshed'),
        type: failed ? 'error' : 'success',
      });
      await load(signal());
    } finally {
      if (!signal()?.aborted) setBusy(false);
    }
  }
  async function saveSource() {
    if (!form || busy) return;
    setBusy(true);
    setFormError('');
    try {
      if (!form.name.trim()) throw new Error(t('subscriptions.source.validation.name'));
      const remote = !form.source || form.source.source_kind === 'remote';
      if (remote && !/^https?:\/\//i.test(form.url)) throw new Error(t('subscriptions.sources.urlRequired'));
      const config = remote
        ? {
            ...form.source?.config,
            url: form.url,
            format: form.format,
            refresh_interval_minutes: Number(form.interval),
          }
        : form.source!.config;
      const input = {
        name: form.name.trim(),
        source_kind: remote ? ('remote' as const) : ('local' as const),
        config,
        enabled: form.source?.enabled ?? true,
      };
      if (form.source) {
        const result = await client.updateSubscriptionSource(
          form.source.id,
          input,
          form.source.updated_at,
          signal(),
        );
        if (!signal()?.aborted) setForm({ ...form, source: result });
      } else {
        await client.createSubscriptionSource(input, signal());
        if (!signal()?.aborted) {
          setCreating(false);
        }
      }
      if (!signal()?.aborted) {
        toast.add({ title: t('subscriptions.sources.saved'), type: 'success' });
        await load(signal());
      }
    } catch (reason) {
      if (!signal()?.aborted) setFormError(describeRequestError(reason));
    } finally {
      if (!signal()?.aborted) setBusy(false);
    }
  }
  async function confirmSourceDeletion(id: string) {
    if (busy) return;
    setBusy(true);
    try {
      const source = await client.getSubscriptionSource(id, signal());
      if (!signal()?.aborted) setDeleting(source);
    } catch (reason) {
      if (!signal()?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!signal()?.aborted) setBusy(false);
    }
  }
  async function deleteSource() {
    if (!deleting || busy) return;
    setBusy(true);
    try {
      await client.deleteSubscriptionSource(deleting.id, deleting.updated_at, signal());
      if (signal()?.aborted) return;
      setDeleting(null);
      setSelected(null);
      setForm(null);
      setTab('nodes');
      toast.add({ title: t('subscriptions.sources.deleted'), type: 'success' });
      await load(signal());
    } catch (reason) {
      if (!signal()?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!signal()?.aborted) setBusy(false);
    }
  }
  async function toggle(node: SubscriptionNodeSummary) {
    if (busy) return;
    setBusy(true);
    try {
      const updated = await client.setSubscriptionNodeVisibility(
        node.id,
        !node.hidden,
        node.visibility_revision,
        signal(),
      );
      if (!signal()?.aborted) setNodes((values) => values.map((value) => (value.id === node.id ? updated : value)));
    } catch (reason) {
      if (!signal()?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!signal()?.aborted) setBusy(false);
    }
  }
  const fields = form && (
    <div className='subscription-settings-fields'>
      <label htmlFor='source-name'>{t('subscriptions.common.name')}</label>
      <input
        disabled={busy}
        id='source-name'
        onChange={(event) => setForm({ ...form, name: event.target.value })}
        value={form.name}
      />
      {(!form.source || form.source.source_kind === 'remote') && (
        <>
          <label htmlFor='source-url'>{t('subscriptions.sources.url')}</label>
          <input
            autoComplete='off'
            disabled={busy}
            id='source-url'
            onChange={(event) => setForm({ ...form, url: event.target.value })}
            placeholder='https://'
            type='url'
            value={form.url}
          />
          <label htmlFor='source-format'>{t('subscriptions.channel.field.format')}</label>
          <select
            disabled={busy}
            id='source-format'
            onChange={(event) => setForm({ ...form, format: event.target.value })}
            value={form.format}
          >
            <option value='auto'>{t('subscriptions.sources.auto')}</option>
            <option value='sing-box-json'>sing-box JSON</option>
            <option value='mihomo-yaml'>Mihomo YAML</option>
            <option value='uri-list'>URI</option>
          </select>
          <label htmlFor='source-interval'>{t('subscriptions.sources.interval')}</label>
          <select
            disabled={busy}
            id='source-interval'
            onChange={(event) => setForm({ ...form, interval: event.target.value })}
            value={form.interval}
          >
            {[...new Set(['0', '60', '360', '720', '1440', form.interval])].map((value) => (
              <option key={value} value={value}>
                {value === '0' ? t('subscriptions.sources.onDemand') : `${value} min`}
              </option>
            ))}
          </select>
        </>
      )}
    </div>
  );
  const localSourceIDs = new Set(
    sources.filter((source) => source.source_kind === 'local').map((source) => source.id),
  );
  const inManualCollection = (node: SubscriptionNodeSummary) =>
    node.origin !== 'source' || localSourceIDs.has(node.source_id);
  const displayedSources = [
    {
      id: 'manual',
      name: t('subscriptions.nodes.manual'),
      source_kind: 'local',
      updated_at: latestStartedAt ?? '',
      enabled: true,
    },
    ...sources.filter((source) => source.source_kind === 'remote'),
  ].filter((source) => source.name.toLowerCase().includes(search.toLowerCase()));
  const pages = Math.max(1, Math.ceil(displayedSources.length / size));
  const current = Math.min(page, pages);
  return (
    <Tabs
      className='subscription-source-workspace'
      value={tab}
      onValueChange={(value) => value === 'settings' ? void openSettings() : setTab('nodes')}
    >
      {error != null && <ErrorNotice error={error} title={t('subscriptions.source.loadFailed')} />}
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='subscription-source-toolbar workspace-toolbar-content'>
          {selected && (
            <div className='subscription-detail-tabs'>
              <Button
                disabled={busy}
                onClick={() => {
                  setSelected(null);
                  setSearch('');
                  setForm(null);
                  setFormError('');
                }}
                variant='outline'
              >
                {t('subscriptions.sources.back')}
              </Button>
              {selected !== 'manual' && (
                <TabsList className='subscriptions-tabs' aria-label={t('subscriptions.sources.settings')}>
                  <TabsTrigger value='nodes' disabled={busy}>
                    {t('subscriptions.sources.nodes')}
                  </TabsTrigger>
                  <TabsTrigger value='settings' disabled={busy}>
                    {t('subscriptions.sources.settings')}
                  </TabsTrigger>
                </TabsList>
              )}
            </div>
          )}
          {tab === 'nodes' || !selected
            ? (
                <>
                  <div className='subscription-search'>
                    <Search aria-hidden='true' />
                    <input
                      aria-label={t('subscriptions.sources.search')}
                      onChange={(event) => {
                        setSearch(event.target.value);
                        setPage(1);
                      }}
                      placeholder={t('subscriptions.sources.search')}
                      value={search}
                    />
                  </div>
                  <div className='subscription-toolbar-actions'>
                    <Button
                      aria-label={t('subscriptions.sources.refresh')}
                      disabled={busy}
                      onClick={() =>
                        selected === 'manual'
                          ? reload()
                          : void refresh(
                            selected
                              ? [selected]
                              : sources
                                  .filter((source) => source.enabled && source.source_kind === 'remote')
                                  .map((source) => source.id),
                          )
                      }
                      size='icon'
                      variant='outline'
                    >
                      <RefreshCw aria-hidden='true' className={busy ? 'animate-spin' : ''} />
                    </Button>
                    {(!selected || selected === 'manual') && (
                      <Button
                        ref={addSourceRef}
                        aria-label={t(
                          selected ? 'subscriptions.nodes.add' : 'subscriptions.source.attach',
                        )}
                        disabled={busy}
                        onClick={() =>
                          selected
                            ? setEditor({ node: null })
                            : (setForm(newSource()), setCreating(true), setFormError(''))
                        }
                        size='icon'
                        variant='outline'
                      >
                        <Plus aria-hidden='true' />
                      </Button>
                    )}
                  </div>
                </>
              )
            : null}
        </div>
      </ToolbarActions>
      {selected
        ? (
            tab === 'settings' && form
              ? (
                  <TabsContent value='settings' className='subscription-source-settings'>
                    {formError && (
                      <p role='alert' className='subscription-form-error'>
                        {formError}
                      </p>
                    )}
                    {fields}
                    <footer>
                      <Button disabled={busy} onClick={() => setDeleting(form.source!)} variant='destructive'>
                        {t('subscriptions.sources.delete')}
                      </Button>
                      <Button disabled={busy} onClick={() => void saveSource()} variant='default'>
                        {t('subscriptions.sources.save')}
                      </Button>
                    </footer>
                  </TabsContent>
                )
              : (
                  <TabsContent value='nodes' className='subscription-node-list'>
                    <SubscriptionNodeGrid
                      busy={busy}
                      nodes={nodes.filter((node) =>
                        selected === 'manual' ? inManualCollection(node) : node.source_id === selected,
                      )}
                      onOpen={(node) => setEditor({ node })}
                      onVisibility={(node) => void toggle(node)}
                      search={search}
                    />
                  </TabsContent>
                )
          )
        : (
            <>
              <div className='subscription-source-table-scroll'>
                <table className='workspace-table subscription-source-table'>
                  <thead>
                    <tr>
                      <th>{t('subscriptions.tabs.sources')}</th>
                      <th>{t('subscriptions.sources.nodes')}</th>
                      <th>{t('subscriptions.sources.updated')}</th>
                      <th>{t('subscriptions.common.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {displayedSources.slice((current - 1) * size, current * size).map((source) => (
                      <tr key={source.id}>
                        <td>
                          <span className='block truncate font-medium' title={source.name}>
                            {source.name}
                          </span>
                        </td>
                        <td>
                          {
                            nodes.filter((node) =>
                              source.id === 'manual'
                                ? inManualCollection(node)
                                : node.source_id === source.id,
                            ).length
                          }
                        </td>
                        <td>
                          {source.updated_at
                            ? new Intl.DateTimeFormat(i18n.language, {
                                dateStyle: 'short',
                                timeStyle: 'short',
                              }).format(new Date(source.updated_at))
                            : '—'}
                        </td>
                        <td>
                          <Button size='sm' disabled={busy} onClick={() => openSource(source.id)} variant='outline'>
                            {t('subscriptions.sources.edit')}
                          </Button>
                          <Button
                            size='sm'
                            disabled={
                              busy || (source.id !== 'manual' && source.source_kind !== 'remote')
                            }
                            onClick={() =>
                              source.id === 'manual' ? reload() : void refresh([source.id])
                            }
                            variant='outline'
                          >
                            {t('subscriptions.sources.refresh')}
                          </Button>
                          {source.id !== 'manual' && (
                            <Button size='sm' disabled={busy} onClick={() => void confirmSourceDeletion(source.id)} variant='destructive'>
                              {t('subscriptions.sources.delete')}
                            </Button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
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
                    disabled={current === 1}
                    onClick={() => setPage(current - 1)}
                    size='icon'
                    variant='outline'
                  >
                    <ChevronLeft />
                  </Button>
                  <span aria-current='page'>{current}</span>
                  <Button
                    aria-label={t('subscriptions.keys.next')}
                    disabled={current === pages}
                    onClick={() => setPage(current + 1)}
                    size='icon'
                    variant='outline'
                  >
                    <ChevronRight />
                  </Button>
                </div>
              </footer>
            </>
          )}
      {editor && (
        <SubscriptionNodeEditor
          candidates={nodes}
          node={editor.node}
          onClose={() => setEditor(null)}
          onSaved={reload}
        />
      )}
      <Dialog
        onOpenChange={(open) => {
          if (!open && !busy) {
            setCreating(false);
          }
        }}
        onOpenChangeComplete={(open) => {
          if (!open) setForm(current => current?.source ? current : null);
        }}
        open={creating}
      >
        <DialogContent className='subscription-source-dialog' finalFocus={addSourceRef}>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.source.attach')}</DialogTitle>
            <DialogDescription className='sr-only'>
              {t('subscriptions.sources.url')}
            </DialogDescription>
          </DialogHeader>
          {formError && (
            <p role='alert' className='subscription-form-error'>
              {formError}
            </p>
          )}
          {fields}
          <DialogFooter>
            <Button
              disabled={busy}
              onClick={() => {
                setCreating(false);
              }}
              variant='outline'
            >
              {t('common.cancel')}
            </Button>
            <Button disabled={busy} onClick={() => void saveSource()} variant='default'>
              {t('subscriptions.sources.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={(open) => !open && !busy && setDeleting(null)} open={deleting !== null}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.sources.delete')}</DialogTitle>
            <DialogDescription>
              {t('subscriptions.sources.deletePrompt', { name: deleting?.name })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button disabled={busy} onClick={() => setDeleting(null)} variant='outline'>
              {t('common.cancel')}
            </Button>
            <Button disabled={busy} onClick={() => void deleteSource()} variant='destructive'>
              {t('subscriptions.sources.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Tabs>
  );
}
