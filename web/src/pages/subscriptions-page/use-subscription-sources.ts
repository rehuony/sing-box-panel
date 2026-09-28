import { useTranslation } from 'react-i18next';
import { useCallback, useDeferredValue, useEffect, useRef, useState } from 'react';
import { useIsMutating, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import type { SubscriptionNodeCatalog, SubscriptionNodeSummary, SubscriptionSource } from '@/api/api-client';

import { queries } from '@/api/queries';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { usePageVisible } from '@/hooks/use-page-visible';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';

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

function sourceForm(source: SubscriptionSource): SourceForm {
  return {
    source,
    name: source.name,
    url: typeof source.config.url === 'string' ? source.config.url : '',
    format: typeof source.config.format === 'string' ? source.config.format : 'auto',
    interval: String(source.config.refresh_interval_minutes ?? 0),
  };
}

function latestStart(...values: (string | undefined)[]): string | undefined {
  return values.filter((value): value is string => value !== undefined && Number.isFinite(Date.parse(value)))
    .sort((a, b) => Date.parse(b) - Date.parse(a))[0];
}

const nodeOrderMutationKey = ['subscription-node-order'];

export function useSubscriptionSources(active = true) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const savingOrder = useIsMutating({ mutationKey: nodeOrderMutationKey }) > 0;
  const visible = usePageVisible();
  const enabled = active && visible;
  const sourceQuery = useQuery({ ...queries.sources(client), enabled, refetchInterval: enabled ? 15_000 : false });
  const nodeQuery = useQuery({ ...queries.nodes(client), enabled, refetchInterval: enabled ? 15_000 : false });
  const sources = sourceQuery.data ?? [];
  const nodes = nodeQuery.data?.nodes ?? [];
  const runningStartedAt = useOptionalSharedTelemetry(s => s.runtimeStatus?.running?.started_at);
  const [lastStartedAt, setLastStartedAt] = useState<string>();
  const latestStartedAt = latestStart(runningStartedAt ?? undefined, lastStartedAt);
  if (latestStartedAt !== lastStartedAt) setLastStartedAt(latestStartedAt);
  const [selected, setSelected] = useState<string | null>(null);
  const [tab, setTab] = useState<'nodes' | 'settings'>('nodes');
  const [search, setSearch] = useState('');
  const [size, setSize] = useState(10);
  const [page, setPage] = useState(1);
  const error = sourceQuery.error ?? nodeQuery.error;
  const [busy, setBusy] = useState(false);
  const [editor, setEditor] = useState<{ node: SubscriptionNodeSummary | null } | null>(null);
  const [form, setForm] = useState<SourceForm | null>(null);
  const [creating, setCreating] = useState(false);
  const [formError, setFormError] = useState('');
  const baseline = form?.source ? sourceForm(form.source) : newSource();
  const dirty = form !== null && (creating || (selected !== null && tab === 'settings'))
    && (['name', 'url', 'format', 'interval'] as const).some(key => form[key] !== baseline[key]);
  const confirmNavigation = useUnsavedChanges(dirty, () => {
    setForm(form?.source ? sourceForm(form.source) : null);
    setCreating(false);
    setFormError('');
  }, busy);
  const lifetimeRef = useRef<AbortController | null>(null);
  const load = useCallback(async () => {
    await cache.invalidateQueries({ queryKey: ['sources'], refetchType: 'none' });
    await cache.invalidateQueries({ queryKey: ['nodes'], refetchType: 'none' });
    await Promise.all([cache.fetchQuery(queries.sources(client)), cache.fetchQuery(queries.nodes(client))]);
  }, [cache, client]);
  useEffect(() => {
    const controller = new AbortController();
    lifetimeRef.current = controller;
    return () => controller.abort();
  }, []);
  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    void client.getRuntimeHistory({ state: 'running', limit: 1 }, controller.signal).then(history => {
      if (!controller.signal.aborted) {
        setLastStartedAt(current => latestStart(current, history.items[0]?.process_started_at));
      }
    }).catch(() => undefined);
    return () => controller.abort();
  }, [client, enabled]);
  const signal = () => lifetimeRef.current?.signal;
  const reload = () => {
    void load().catch(reason => toast.add({ title: describeRequestError(reason), type: 'error' }));
  };
  const refreshingRef = useRef(false);
  const [refreshingSources, setRefreshingSources] = useState<Record<string, 'pending' | 'success' | 'error'>>({});

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
      setForm(sourceForm(source));
      setTab('settings');
    } catch (reason) {
      if (!signal()?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!signal()?.aborted) setBusy(false);
    }
  }
  async function refresh(sourceIDs: string[]) {
    if (refreshingRef.current) return;
    refreshingRef.current = true;
    try {
      let failed = 0;
      const pending = [...new Set(sourceIDs)];
      setRefreshingSources(Object.fromEntries(pending.map(id => [id, 'pending' as const])));
      await Promise.all(Array.from({ length: Math.min(3, pending.length) }, async () => {
        while (pending.length && !signal()?.aborted) {
          const id = pending.shift()!;
          try {
            await client.refreshSubscriptionSource(id, signal());
            if (!signal()?.aborted) setRefreshingSources(current => ({ ...current, [id]: 'success' }));
          } catch {
            failed++;
            if (!signal()?.aborted) setRefreshingSources(current => ({ ...current, [id]: 'error' }));
          }
        }
      }));
      if (signal()?.aborted) return;
      toast.add({
        title: failed
          ? t('subscriptions.sources.refreshFailed')
          : t('subscriptions.sources.refreshed'),
        type: failed ? 'error' : 'success',
      });
      // Refresh must reach the server even when there are no remote sources.
      await load();
    } catch (reason) {
      if (!signal()?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      refreshingRef.current = false;
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
        if (!signal()?.aborted) setForm(sourceForm(result));
      } else {
        await client.createSubscriptionSource(input, signal());
        if (!signal()?.aborted) {
          setCreating(false);
        }
      }
      if (!signal()?.aborted) {
        toast.add({ title: t('subscriptions.sources.saved'), type: 'success' });
        await load();
      }
    } catch (reason) {
      if (!signal()?.aborted) setFormError(describeRequestError(reason));
    } finally {
      if (!signal()?.aborted) setBusy(false);
    }
  }
  const [busyNodes, setBusyNodes] = useState<Set<string>>(() => new Set());
  const busyNodesRef = useRef(new Set<string>());
  const toggle = useCallback(async (node: SubscriptionNodeSummary) => {
    if (busyNodesRef.current.has(node.id)) return;
    busyNodesRef.current.add(node.id);
    setBusyNodes(new Set(busyNodesRef.current));
    try {
      await client.setSubscriptionNodeVisibility(
        node.id,
        !node.hidden,
        node.visibility_revision,
        signal(),
      );
    } catch (reason) {
      if (!signal()?.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      busyNodesRef.current.delete(node.id);
      if (!signal()?.aborted) setBusyNodes(new Set(busyNodesRef.current));
    }
  }, [client]);
  async function reorderNodes(collectionID: string, ids: string[]) {
    let reconcile = true;
    try {
      const revision = cache.getQueryData<SubscriptionNodeCatalog>(['nodes'])?.node_orders[collectionID]?.revision ?? 0;
      // A completed drag must survive panel navigation. The API boundary still
      // rejects responses from an expired session before they reach the cache.
      const order = await client.setSubscriptionNodeOrder(collectionID, ids, revision);
      cache.setQueryData<SubscriptionNodeCatalog>(['nodes'], current => {
        if (!current) return current;
        const byID = new Map(current.nodes.map(node => [node.id, node]));
        const members = new Set(ids);
        const reordered = ids.flatMap(id => byID.has(id) ? [byID.get(id)!] : []);
        let index = 0;
        return {
          ...current,
          node_orders: { ...current.node_orders, [collectionID]: order },
          nodes: current.nodes.map(node => members.has(node.id) ? reordered[index++] : node),
        };
      });
    } catch (error) {
      reconcile = !(error instanceof DOMException && error.name === 'AbortError');
      throw error;
    } finally {
      // Reconcile on success and on conflicts before the grid drops its preview.
      if (reconcile) await cache.invalidateQueries({ queryKey: ['nodes'] });
    }
  }
  const orderMutation = useMutation({
    mutationKey: nodeOrderMutationKey,
    gcTime: 0,
    mutationFn: ({ collectionID, ids }: { collectionID: string; ids: string[] }) => reorderNodes(collectionID, ids),
  });
  const localSourceIDs = new Set(
    sources.filter((source) => source.source_kind === 'local').map((source) => source.id),
  );
  const inManualCollection = (node: SubscriptionNodeSummary) =>
    node.origin !== 'source' || localSourceIDs.has(node.source_id);
  const deferredSearch = useDeferredValue(search);
  const displayedSources = [
    {
      id: 'manual',
      name: t('subscriptions.nodes.manual'),
      source_kind: 'local',
      updated_at: latestStartedAt ?? '',
      enabled: true,
    },
    ...sources.filter((source) => source.source_kind === 'remote'),
  ].filter((source) => source.name.toLowerCase().includes(deferredSearch.toLowerCase()));
  const pages = Math.max(1, Math.ceil(displayedSources.length / size));
  const current = Math.min(page, pages);
  if (page !== current) setPage(current);
  return {
    sources,
    refreshingSources,
    busyNodes,
    loading: sourceQuery.isPending || nodeQuery.isPending,
    sourceLoading: sourceQuery.isPending,
    nodeLoading: nodeQuery.isPending,
    refreshing: sourceQuery.isFetching || nodeQuery.isFetching,
    nodes,
    selected,
    setSelected,
    tab,
    setTab,
    search,
    setSearch,
    size,
    setSize,
    page,
    setPage,
    error,
    busy: busy || savingOrder,
    editor,
    setEditor,
    form,
    setForm,
    creating,
    setCreating,
    formError,
    setFormError,
    dirty,
    confirmNavigation,
    reload,
    openSource,
    openSettings,
    refresh,
    saveSource,
    toggle,
    reorderNodes: (collectionID: string, ids: string[]) => orderMutation.mutateAsync({ collectionID, ids }),
    displayedSources,
    inManualCollection,
    current,
    pages,
    newSource,
  };
}
