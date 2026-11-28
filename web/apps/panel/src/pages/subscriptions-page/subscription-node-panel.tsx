import type { ComponentProps } from 'react';

import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { useDeferredValue, useMemo, useRef, useState } from 'react';
import { CirclePlus, Layers3, Server, TriangleAlert } from 'lucide-react';
import { useMutation, useMutationState, useQuery, useQueryClient } from '@tanstack/react-query';
import { Popover, PopoverContent, PopoverTitle, PopoverTrigger } from '@shared/components/ui/popover';

import type { NodePublication, SubscriptionDiagnostic, SubscriptionNode } from '@/api/api-client';

import { queries } from '@/api/queries';
import { nodePath } from '@/utils/node-path';
import { ApiRequestError } from '@/api/api-client';
import { compareText } from '@/utils/compare-text';
import { useApiClient } from '@/api/api-client-context';
import { RegionError } from '@/components/region-state';
import { SelectField } from '@/components/select-field';
import { usePageVisible } from '@/hooks/use-page-visible';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { describeRequestError } from '@/components/error-notice';
import { Rail, RailCount, RailHeading, RailItem } from '@/components/rail';
import { SortContext, useDraggedId, useSortWindow } from '@/components/sortable';
import { StripAction, StripDivider, StripSearch } from '@/components/title-strip';

import type { IssueItem } from './issue-list';
import type { SourceDialogMode } from './subscription-source-dialog';

import { IssueList } from './issue-list';
import { nodeCardGrid } from './node-card-classes';
import { useSourceRefresh } from './use-source-refresh';
import { SourceDialogs, SourceRailItem } from './source-rail';
import { reorderVisibleNodes } from './subscription-node-order';
import { SubscriptionNodeDialog } from './subscription-node-dialog';
import { SubscriptionContentState } from './subscription-content-state';
import { PoolNodeCard, PoolNodeCardOverlay } from './subscription-node-card';
import { ALL_ORIGINS, ALL_PROTOCOLS, FLEET_ORIGIN, matchesNode, originFilter } from './subscription-node-pool';

const orderMutationKey = ['subscription-node-order'];
/** The card the pool's nodes sit on, 16px in and 12px apart, which also holds its loading, failed and empty states. */
const poolCard = 'flex min-h-0 min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]';

/**
 * The fleet nodes that publish nothing or leave out some of their inbounds,
 * as a warning in the strip whose popover groups them by cause, each node
 * opening its configuration.
 */
function PoolDiagnostics({ diagnostics, publications }: {
  diagnostics: SubscriptionDiagnostic[];
  publications: NodePublication[];
}) {
  const { t, i18n } = useTranslation();
  const client = useApiClient();
  const fleet = useQuery(queries.fleetNodes(client));
  const names = new Map<string, string>([
    ...publications.map(item => [item.node_id, item.node_name] as const),
    ...(fleet.data?.items ?? []).map(item => [item.id, item.name] as const),
  ]);
  const byCode = new Map<string, IssueItem[]>();
  diagnostics.forEach((item, index) => {
    byCode.set(item.code, [...byCode.get(item.code) ?? [], {
      key: `${index}`,
      name: item.node_id ? names.get(item.node_id) ?? item.node_id : t('subscriptions.pool.diagnostics.unknownNode'),
      detail: item.item_index >= 0 ? `${item.collection}[${item.item_index}]` : undefined,
      to: item.node_id ? nodePath(item.node_id, 'configuration') : undefined,
    }]);
  });
  const groups = [...byCode]
    .map(([code, items]) => ({
      key: code,
      title: i18n.exists(`subscriptions.issues.${code}`) ? t(`subscriptions.issues.${code}`) : t('subscriptions.issues.unknown', { code }),
      items,
    }))
    .sort((left, right) => right.items.length - left.items.length);
  const label = t('subscriptions.pool.diagnostics.label', { count: diagnostics.length });
  return (
    <Popover>
      <PopoverTrigger render={<Button aria-label={label} className='flex-none' size='sm' variant='flat' />}>
        <TriangleAlert aria-hidden='true' className='text-[var(--color-warning)]' data-icon='inline-start' />
        <span className='font-[family-name:var(--font-data)] tabular-nums'>{diagnostics.length}</span>
      </PopoverTrigger>
      <PopoverContent align='end' className='max-h-[min(22.5rem,var(--available-height))] w-[min(22.5rem,calc(100vw-2rem),var(--available-width))] gap-0 overflow-hidden p-0'>
        <PopoverTitle className='sr-only'>{t('subscriptions.pool.diagnostics.title')}</PopoverTitle>
        <ScrollArea><IssueList groups={groups} /></ScrollArea>
      </PopoverContent>
    </Popover>
  );
}

/**
 * The pool's cards in their grid: every card of a small pool, and of a large
 * one the cards near the view and the one being dragged.
 */
function PoolGrid({ nodes, onOpen, saving, ...handlers }: {
  nodes: readonly SubscriptionNode[];
  saving: boolean;
  onOpen: (node: SubscriptionNode) => void;
} & Pick<ComponentProps<'ul'>, 'onClickCapture' | 'onKeyDownCapture' | 'onPointerDownCapture'>) {
  const { t } = useTranslation();
  const ids = useMemo(() => nodes.map(node => node.id), [nodes]);
  const { ref, slots, style } = useSortWindow(ids, useDraggedId());
  return (
    <ul
      aria-busy={saving || undefined}
      aria-label={t('subscriptions.tabs.nodes')}
      className={nodeCardGrid}
      data-slot='pool-node-grid'
      ref={ref}
      style={style}
      {...handlers}
    >
      {slots.map(slot => (typeof slot === 'number'
        ? <PoolNodeCard index={slot} key={nodes[slot]!.id} node={nodes[slot]!} onOpen={onOpen} />
        : <li key='gap' {...slot} />))}
    </ul>
  );
}

/**
 * The subscription node pool beside the rail of its origins: every node, the
 * fleet's inbounds, then each source, which the rail adds, refreshes and
 * edits. The pool shows as a grid of panes in its saved order, unpaged and
 * drawn near the view, filtered by the rail's origin and the strip's
 * protocol and search. A drop saves the whole pool's new order; while it
 * saves the pending order shows, here or after coming back, no card drags,
 * and a pool that changed meanwhile is read again. A click opens a node's
 * detail.
 */
export function SubscriptionNodePanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const visible = usePageVisible();
  const enabled = active && visible;
  const list = useQuery({ ...queries.nodes(client), enabled, refetchInterval: enabled ? 15_000 : false });
  const sourceList = useQuery({ ...queries.sources(client), enabled, refetchInterval: enabled ? 15_000 : false });
  const { refresh, refreshing } = useSourceRefresh();
  const [sourceDialog, setSourceDialog] = useState<SourceDialogMode | null>(null);
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
  const sources = useMemo(
    () => [...sourceList.data ?? []].sort((left, right) => compareText(left.name, right.name)),
    [sourceList.data],
  );
  // How many pool nodes each origin adds.
  const originCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const node of items ?? []) counts.set(originFilter(node), (counts.get(originFilter(node)) ?? 0) + 1);
    return counts;
  }, [items]);
  const protocolChoices = useMemo(() => [
    { label: t('subscriptions.pool.allProtocols'), value: ALL_PROTOCOLS },
    ...[...new Set((items ?? []).map(node => node.protocol))].sort(compareText).map(value => ({ label: value, value })),
  ], [items, t]);
  // A filter whose choice left the pool shows everything again.
  const originValue = origin === FLEET_ORIGIN || sources.some(source => `source:${source.id}` === origin) ? origin : ALL_ORIGINS;
  const protocolValue = protocolChoices.some(choice => choice.value === protocol) ? protocol : ALL_PROTOCOLS;
  const shown = useMemo(() => ordered.filter(node => (originValue === ALL_ORIGINS || originFilter(node) === originValue)
    && (protocolValue === ALL_PROTOCOLS || node.protocol === protocolValue)
    && matchesNode(node, query)), [ordered, originValue, protocolValue, query]);
  const byID = useMemo(() => new Map(ordered.map(node => [node.id, node])), [ordered]);
  const sortItems = useMemo(() => shown.map(node => ({ id: node.id, label: node.display_name })), [shown]);

  function move(activeID: string, overID: string) {
    if (saving) return;
    const order = ordered.map(node => node.id);
    const next = reorderVisibleNodes(order, shown.map(node => node.id), activeID, overID);
    if (next !== order) saveOrder.mutate(next);
  }

  const diagnostics = list.data?.diagnostics ?? [];
  let content;
  if (list.isPending) {
    content = <SubscriptionContentState loading />;
  } else if (list.data === undefined) {
    content = <RegionError className='flex-1' error={list.error} reload={() => void list.refetch()} title={t('subscriptions.pool.loadFailed')} />;
  } else if (ordered.length === 0) {
    content = <SubscriptionContentState kind='nodes' title={t('subscriptions.pool.empty')} />;
  } else if (shown.length === 0) {
    content = <SubscriptionContentState kind='search' title={t('subscriptions.pool.noMatch')} />;
  } else {
    content = (
      <ScrollArea className='min-h-0 flex-1' contentClassName='p-4'>
        <SortContext
          ended={t('channels.nodeDragEnd')}
          instructions={t('subscriptions.pool.dragInstructions')}
          items={sortItems}
          locked={saving}
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
          <PoolGrid
            nodes={shown}
            onClickCapture={(event) => {
              if (!suppressClickRef.current) return;
              event.preventDefault();
              event.stopPropagation();
            }}
            onKeyDownCapture={(event) => {
              if (!draggingRef.current && event.key !== 'F2') suppressClickRef.current = false;
            }}
            onOpen={setOpened}
            onPointerDownCapture={() => {
              if (!draggingRef.current) suppressClickRef.current = false;
            }}
            saving={saving}
          />
        </SortContext>
      </ScrollArea>
    );
  }

  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col'>
      <ToolbarActions active={active} target={toolbarTarget}>
        {/* On a phone the search takes a line of its own below the filters. */}
        <div className='flex w-full min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1'>
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
          <StripDivider />
          <StripAction
            icon={<CirclePlus aria-hidden='true' data-icon='inline-start' />}
            label={t('subscriptions.sources.add')}
            onClick={() => setSourceDialog({ kind: 'create' })}
          />
        </div>
      </ToolbarActions>
      {/* On a phone the rail takes a capped band above the grid. */}
      <div className='grid min-h-0 flex-1 grid-cols-[16rem_minmax(0,1fr)] gap-3 max-md:grid-cols-1 max-md:grid-rows-[minmax(0,10.5rem)_minmax(0,1fr)]'>
        <Rail label={t('subscriptions.pool.originFilter')}>
          <RailItem
            current={originValue === ALL_ORIGINS}
            end={<RailCount>{ordered.length}</RailCount>}
            icon={<Layers3 />}
            name={t('subscriptions.pool.allOrigins')}
            onClick={() => setOrigin(ALL_ORIGINS)}
          />
          <RailItem
            current={originValue === FLEET_ORIGIN}
            end={<RailCount>{originCounts.get(FLEET_ORIGIN) ?? 0}</RailCount>}
            icon={<Server />}
            name={t('subscriptions.pool.fleetOrigin')}
            onClick={() => setOrigin(FLEET_ORIGIN)}
          />
          <RailHeading end={<RailCount>{sources.length}</RailCount>}>{t('subscriptions.pool.sources')}</RailHeading>
          {sources.map(source => (
            <SourceRailItem
              count={originCounts.get(`source:${source.id}`) ?? 0}
              current={originValue === `source:${source.id}`}
              key={source.id}
              onEdit={() => setSourceDialog({ kind: 'edit', source })}
              onRefresh={() => void refresh(source)}
              onSelect={() => setOrigin(`source:${source.id}`)}
              refreshing={refreshing.has(source.id)}
              source={source}
            />
          ))}
        </Rail>
        <div className={poolCard}>{content}</div>
      </div>
      <SourceDialogs mode={sourceDialog} onModeChange={setSourceDialog} />
      {opened ? <SubscriptionNodeDialog key={opened.id} node={opened} onClose={() => setOpened(null)} /> : null}
    </div>
  );
}
