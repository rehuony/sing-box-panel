import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useRef, useState } from 'react';

import type { SubscriptionNodeSummary, SubscriptionSource, SubscriptionSourceSummary } from '@/api/api-client';

import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
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

export function useSubscriptionSources() {
  const { t } = useTranslation();
  const client = useApiClient();
  const telemetry = useOptionalSharedTelemetry();
  const [lastStartedAt, setLastStartedAt] = useState<string>();
  const runningStartedAt = telemetry?.runtimeStatus?.running?.started_at;
  const latestStartedAt = latestStart(runningStartedAt, lastStartedAt);
  if (latestStartedAt !== lastStartedAt) setLastStartedAt(latestStartedAt);
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
        const [all, catalog] = await Promise.all([
          (async () => {
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
            return all;
          })(),
          client.getSubscriptionNodeCatalog(signal),
        ]);
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
  const reload = () => {
    client.invalidateReadCache();
    void load(signal());
  };

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
    if (busy) return;
    setBusy(true);
    try {
      const results = await Promise.allSettled(
        sourceIDs.map(async (id) => {
          const currentSignal = signal();
          if (!currentSignal) return;
          await client.refreshSubscriptionSource(id, currentSignal);
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
      // Refresh must reach the server even when there are no remote sources.
      client.invalidateReadCache();
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
        if (!signal()?.aborted) setForm(sourceForm(result));
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
  if (page !== current) setPage(current);
  return {
    sources,
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
    busy,
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
    displayedSources,
    inManualCollection,
    current,
    pages,
    newSource,
  };
}
