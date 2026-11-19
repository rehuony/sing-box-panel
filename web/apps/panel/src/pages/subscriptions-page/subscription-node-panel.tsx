import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { CirclePlus, TriangleAlert } from 'lucide-react';
import { toast } from '@shared/components/ui/toast-manager';
import { useDeferredValue, useMemo, useRef, useState } from 'react';
import { useMutation, useMutationState, useQuery, useQueryClient } from '@tanstack/react-query';
import { Popover, PopoverContent, PopoverHeader, PopoverTitle, PopoverTrigger } from '@shared/components/ui/popover';

import type { NodePublication, SubscriptionDiagnostic, SubscriptionNode } from '@/api/api-client';

import { queries } from '@/api/queries';
import { nodePath } from '@/utils/node-path';
import { ApiRequestError } from '@/api/api-client';
import { compareText } from '@/utils/compare-text';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { usePageVisible } from '@/hooks/use-page-visible';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { describeRequestError } from '@/components/error-notice';
import { StripDivider, StripSearch } from '@/components/title-strip';

import { reorderVisibleNodes } from './subscription-node-order';
import { SubscriptionNodeDialog } from './subscription-node-dialog';
import { SubscriptionContentState } from './subscription-content-state';
import { PoolNodeCard, PoolNodeCardOverlay } from './subscription-node-card';
import { SubscriptionNodeSortContext } from './subscription-node-sort-context';
import { ALL_ORIGINS, ALL_PROTOCOLS, FLEET_ORIGIN, matchesNode, originFilter } from './subscription-node-pool';

const orderMutationKey = ['subscription-node-order'];

/**
 * The fleet nodes that publish nothing or leave out some of their inbounds,
 * as a warning in the strip whose popover names each node and why.
 */
function PoolDiagnostics({ diagnostics, publications }: {
  diagnostics: SubscriptionDiagnostic[];
  publications: NodePublication[];
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const fleet = useQuery(queries.fleetNodes(client));
  const names = new Map<string, string>([
    ...publications.map(item => [item.node_id, item.node_name] as const),
    ...(fleet.data?.items ?? []).map(item => [item.id, item.name] as const),
  ]);
  const label = t('subscriptions.pool.diagnostics.label', { count: diagnostics.length });
  return (
    <Popover>
      <PopoverTrigger render={<Button aria-label={label} className='flex-none' size='sm' title={label} variant='flat' />}>
        <TriangleAlert aria-hidden='true' className='text-[var(--color-warning)]' data-icon='inline-start' />
        <span className='font-[family-name:var(--font-data)] tabular-nums'>{diagnostics.length}</span>
      </PopoverTrigger>
      <PopoverContent align='end' className='max-h-[min(24rem,var(--available-height))] w-80 overflow-y-auto'>
        <PopoverHeader>
          <PopoverTitle>{t('subscriptions.pool.diagnostics.title')}</PopoverTitle>
        </PopoverHeader>
        <ul className='grid gap-2'>
          {diagnostics.map((item) => {
            const name = item.node_id ? names.get(item.node_id) ?? item.node_id : t('subscriptions.pool.diagnostics.unknownNode');
            const place = item.item_index >= 0 ? `${item.collection}[${item.item_index}]` : '';
            return (
              <li className='min-w-0' key={`${item.node_id ?? ''}-${item.collection}-${item.item_index}-${item.code}`}>
                <p className='flex min-w-0 items-baseline gap-2'>
                  {item.node_id
                    ? <Link className='min-w-0 truncate font-medium hover:underline' title={name} to={nodePath(item.node_id, 'configuration')}>{name}</Link>
                    : <span className='min-w-0 truncate font-medium'>{name}</span>}
                  {place ? <span className='flex-none font-[family-name:var(--font-data)] text-xs text-muted-foreground'>{place}</span> : null}
                </p>
                <p className='text-xs text-muted-foreground'>
                  {t(`subscriptions.pool.diagnostics.reasons.${item.code}`, {
                    defaultValue: t('subscriptions.pool.diagnostics.reasons.unknown', { code: item.code }),
                  })}
                </p>
              </li>
            );
          })}
        </ul>
      </PopoverContent>
    </Popover>
  );
}

/**
 * The subscription node pool: every node as a card in the pool's saved order,
 * unpaged, filtered by origin and protocol and searched from the strip. A
 * drop saves the whole pool's new order; while it saves the pending order
 * shows, here or after coming back, and a pool that changed meanwhile is read
 * again. A click opens a node's detail.
 */
export function SubscriptionNodePanel({ active = true, onAddSource, toolbarTarget }: {
  active?: boolean;
  /** Leads to adding a source, the next step of an empty pool. */
  onAddSource?: () => void;
  toolbarTarget?: HTMLElement | null;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const visible = usePageVisible();
  const enabled = active && visible;
  const list = useQuery({ ...queries.nodes(client), enabled, refetchInterval: enabled ? 15_000 : false });
  const [origin, setOrigin] = useState(ALL_ORIGINS);
  const [protocol, setProtocol] = useState(ALL_PROTOCOLS);
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search.trim().toLowerCase());
  const [opened, setOpened] = useState<SubscriptionNode | null>(null);
  // A drag completes a click on the card it ends over; that click is not the user's.
  const draggingRef = useRef(false);
  const suppressClickRef = useRef(false);

  // The order a drop is saving, kept with the mutation so it survives leaving the tab.
  const pendingOrders = useMutationState({
    filters: { mutationKey: orderMutationKey, status: 'pending' },
    select: mutation => mutation.state.variables as string[] | undefined,
  });
  const pendingOrder = pendingOrders.at(-1);
  const saving = pendingOrders.length > 0;
  const saveOrder = useMutation({
    mutationKey: orderMutationKey,
    gcTime: 0,
    mutationFn: async (ids: string[]) => {
      try {
        cache.setQueryData(['nodes'], await client.setSubscriptionNodeOrder(ids));
      } catch (error) {
        if (error instanceof DOMException && error.name === 'AbortError') return;
        if (error instanceof ApiRequestError && error.status === 409) {
          toast.add({ title: t('subscriptions.pool.orderConflict'), type: 'warning' });
        } else {
          toast.add({ title: t('subscriptions.pool.orderSaveFailed'), description: describeRequestError(error), type: 'error' });
        }
        // The confirmed order replaces the one that did not save.
        await cache.invalidateQueries({ queryKey: ['nodes'], exact: true });
      }
    },
  });

  const items = list.data?.items;
  const ordered = useMemo(() => {
    if (!items || !pendingOrder) return items ?? [];
    const byID = new Map(items.map(node => [node.id, node]));
    const placed = new Set(pendingOrder);
    return [...pendingOrder.flatMap(id => byID.get(id) ?? []), ...items.filter(node => !placed.has(node.id))];
  }, [items, pendingOrder]);
  const originChoices = useMemo(() => {
    const sources = new Map<string, string>();
    for (const node of items ?? []) {
      if (node.origin.kind === 'source') sources.set(originFilter(node), node.origin.source_name ?? node.origin.source_id ?? '');
    }
    return [
      { label: t('subscriptions.pool.allOrigins'), value: ALL_ORIGINS },
      { label: t('subscriptions.pool.fleetOrigin'), value: FLEET_ORIGIN },
      ...[...sources]
        .sort(([, left], [, right]) => compareText(left, right))
        .map(([value, label]) => ({ label, value })),
    ];
  }, [items, t]);
  const protocolChoices = useMemo(() => [
    { label: t('subscriptions.pool.allProtocols'), value: ALL_PROTOCOLS },
    ...[...new Set((items ?? []).map(node => node.protocol))].sort(compareText).map(value => ({ label: value, value })),
  ], [items, t]);
  // A filter whose choice left the pool shows everything again.
  const originValue = originChoices.some(choice => choice.value === origin) ? origin : ALL_ORIGINS;
  const protocolValue = protocolChoices.some(choice => choice.value === protocol) ? protocol : ALL_PROTOCOLS;
  const shown = useMemo(() => ordered.filter(node => (originValue === ALL_ORIGINS || originFilter(node) === originValue)
    && (protocolValue === ALL_PROTOCOLS || node.protocol === protocolValue)
    && matchesNode(node, query)), [ordered, originValue, protocolValue, query]);
  const byID = useMemo(() => new Map(ordered.map(node => [node.id, node])), [ordered]);

  function move(activeID: string, overID: string) {
    if (saving) return;
    const order = ordered.map(node => node.id);
    const next = reorderVisibleNodes(order, shown.map(node => node.id), activeID, overID);
    if (next !== order) saveOrder.mutate(next);
  }

  function clearFilters() {
    setOrigin(ALL_ORIGINS);
    setProtocol(ALL_PROTOCOLS);
    setSearch('');
  }

  const diagnostics = list.data?.diagnostics ?? [];
  let content;
  if (list.isPending) {
    content = <SubscriptionContentState loading />;
  } else if (list.data === undefined) {
    content = (
      <SubscriptionContentState kind='preview' title={t('subscriptions.pool.loadFailed')}>
        <Button onClick={() => void list.refetch()} variant='outline'>{t('shell.error.retry')}</Button>
      </SubscriptionContentState>
    );
  } else if (ordered.length === 0) {
    content = (
      <SubscriptionContentState kind='nodes' title={t('subscriptions.pool.empty')}>
        {onAddSource
          ? (
              <Button onClick={onAddSource} variant='outline'>
                <CirclePlus aria-hidden='true' data-icon='inline-start' />
                {t('subscriptions.sources.add')}
              </Button>
            )
          : null}
      </SubscriptionContentState>
    );
  } else if (shown.length === 0) {
    content = (
      <SubscriptionContentState kind='search' title={t('subscriptions.pool.noMatch')}>
        <Button onClick={clearFilters} variant='outline'>{t('subscriptions.pool.clearFilters')}</Button>
      </SubscriptionContentState>
    );
  } else {
    content = (
      <SubscriptionNodeSortContext
        instructions={t('subscriptions.pool.dragInstructions')}
        items={shown.map(node => ({ id: node.id, label: node.display_name }))}
        onDraggingChange={(dragging) => {
          draggingRef.current = dragging;
          if (dragging) suppressClickRef.current = true;
        }}
        onMove={move}
        renderOverlay={(id) => {
          const node = byID.get(id);
          return node ? <PoolNodeCardOverlay node={node} /> : null;
        }}
      >
        <ul
          aria-busy={saving || undefined}
          aria-label={t('subscriptions.tabs.nodes')}
          className='grid grid-cols-[repeat(auto-fill,minmax(min(100%,var(--pane-min-width)),1fr))] content-start gap-3'
          data-slot='pool-node-grid'
          onClickCapture={(event) => {
            if (!suppressClickRef.current) return;
            event.preventDefault();
            event.stopPropagation();
          }}
          onKeyDownCapture={(event) => {
            if (!draggingRef.current && event.key !== 'F2') suppressClickRef.current = false;
          }}
          onPointerDownCapture={() => {
            if (!draggingRef.current) suppressClickRef.current = false;
          }}
        >
          {shown.map((node, index) => (
            <PoolNodeCard index={index} key={node.id} node={node} onOpen={setOpened} sortable={!saving} />
          ))}
        </ul>
      </SubscriptionNodeSortContext>
    );
  }

  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col'>
      <ToolbarActions active={active} target={toolbarTarget}>
        {/* On a phone the search takes a line of its own below the filters. */}
        <div className='flex w-full min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1'>
          <SelectField
            active={originValue !== ALL_ORIGINS}
            aria-label={t('subscriptions.pool.originFilter')}
            className='max-w-48 min-w-0 flex-none'
            items={originChoices}
            onValueChange={setOrigin}
            value={originValue}
            variant='flat'
          />
          <SelectField
            active={protocolValue !== ALL_PROTOCOLS}
            aria-label={t('subscriptions.pool.protocolFilter')}
            className='max-w-40 min-w-0 flex-none'
            items={protocolChoices}
            onValueChange={setProtocol}
            value={protocolValue}
            variant='flat'
          />
          <StripDivider className='max-sm:hidden' />
          <StripSearch className='max-sm:order-last max-sm:basis-full' label={t('subscriptions.pool.search')} onValueChange={setSearch} value={search} />
          {diagnostics.length > 0
            ? (
                <>
                  <StripDivider className='max-sm:hidden' />
                  <PoolDiagnostics diagnostics={diagnostics} publications={list.data?.node_publications ?? []} />
                </>
              )
            : null}
        </div>
      </ToolbarActions>
      <div className='flex min-h-0 flex-1 flex-col overflow-auto p-4'>{content}</div>
      {opened ? <SubscriptionNodeDialog key={opened.id} node={opened} onClose={() => setOpened(null)} /> : null}
    </div>
  );
}
