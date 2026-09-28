import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import { useSortable } from '@dnd-kit/sortable';
import { useReducedMotion } from 'motion/react';
import { Eye, EyeOff, MoreHorizontal } from 'lucide-react';
import { memo, useDeferredValue, useMemo, useRef, useState } from 'react';

import type { SubscriptionNodeSummary } from '@/api/api-client';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { ListPagination } from '@/components/list-pagination';

import { SubscriptionNodeCardContent } from './subscription-node-card';
import { SubscriptionContentState } from './subscription-content-state';
import { SubscriptionNodeSortContext } from './subscription-node-sort-context';
import { filterSourceNodes, orderSourceNodes, reorderVisibleNodes } from './subscription-node-order';

interface NodeGridProps {
  busy?: boolean;
  search: string;
  error?: boolean;
  loading?: boolean;
  readOnly?: boolean;
  selected?: Set<string>;
  busyNodes?: Set<string>;
  onSelect?: (id: string) => void;
  nodes: SubscriptionNodeSummary[];
  onReorder?: (ids: string[]) => Promise<void>;
  onOpen: (node: SubscriptionNodeSummary) => void;
  onVisibility?: (node: SubscriptionNodeSummary) => void;
}

const SourceNodeCard = memo(({ node, busy, readOnly, selected, onSelect, onOpen, onVisibility }: Pick<NodeGridProps,
  'busy' | 'readOnly' | 'selected' | 'onSelect' | 'onOpen' | 'onVisibility'> & { node: SubscriptionNodeSummary }) => {
  const { t } = useTranslation();
  const reducedMotion = useReducedMotion();
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: node.id, disabled: busy || readOnly,
  });
  function canDrag(target: EventTarget) {
    if (!(target instanceof Element)) return false;
    const control = target.closest('button, input, select, textarea, a, [role="button"]');
    // The hidden overlay is itself the card surface: click restores, drag reorders.
    return !control || control.classList.contains('subscription-node-hidden');
  }
  return (
    <article
      ref={setNodeRef}
      {...attributes}
      {...listeners}
      role='article'
      aria-label={node.name}
      aria-roledescription={t('subscriptions.nodes.sortableCard')}
      aria-pressed={undefined}
      aria-keyshortcuts='F2'
      tabIndex={busy || readOnly ? -1 : 0}
      className={cn('subscription-node-card subscription-node-sortable', node.hidden && 'is-hidden')}
      data-dragging={isDragging || undefined}
      style={{ transform: CSS.Transform.toString(transform), transition: reducedMotion ? 'none' : transition }}
      onMouseDown={(event) => {
        if (canDrag(event.target)) listeners?.onMouseDown?.(event);
      }}
      onTouchStart={(event) => {
        if (canDrag(event.target)) listeners?.onTouchStart?.(event);
      }}
      onKeyDown={(event) => {
        if (event.target === event.currentTarget) listeners?.onKeyDown?.(event);
      }}
    >
      <SubscriptionNodeCardContent node={node} actions={(
        <>
          {onSelect
            ? (
                <input
                  aria-label={t('subscriptions.nodes.select', { name: node.name })}
                  checked={selected?.has(node.id) ?? false}
                  disabled={busy || readOnly}
                  onChange={() => onSelect(node.id)}
                  type='checkbox'
                />
              )
            : !node.hidden && (
                <Button
                  aria-label={t('subscriptions.nodes.hide', { name: node.name })}
                  disabled={busy}
                  onClick={() => onVisibility?.(node)}
                  size='icon-xs'
                  variant='outline'
                >
                  <Eye aria-hidden='true' />
                </Button>
              )}
          <Button
            aria-label={t('subscriptions.nodes.details', { name: node.name })}
            onClick={() => onOpen(node)}
            size='icon-xs'
            variant='outline'
          >
            <MoreHorizontal aria-hidden='true' />
          </Button>
        </>
      )} />
      {node.hidden && (
        <Button
          aria-label={t('subscriptions.nodes.show', { name: node.name })}
          className='subscription-node-hidden'
          disabled={busy}
          onClick={() => onVisibility?.(node)}
          size='content'
          title={t('subscriptions.nodes.show', { name: node.name })}
          variant='ghost'
        >
          <EyeOff aria-hidden='true' />
          <span>{t('subscriptions.nodes.hidden')}</span>
        </Button>
      )}
    </article>
  );
});

export function SubscriptionNodeGrid({
  loading = false,
  error = false,
  onReorder,
  nodes,
  search,
  busy: externalBusy,
  busyNodes,
  readOnly,
  selected,
  onSelect,
  onOpen,
  onVisibility,
}: NodeGridProps) {
  const { t } = useTranslation();
  const [pendingOrder, setPendingOrder] = useState<string[] | null>(null);
  const savingRef = useRef(false);
  const busy = externalBusy || pendingOrder !== null;
  const deferredSearch = useDeferredValue(search);
  const ordered = useMemo(() => pendingOrder ? orderSourceNodes(nodes, pendingOrder) : nodes, [nodes, pendingOrder]);
  const byID = new Map(ordered.map(node => [node.id, node]));
  const order = ordered.map(node => node.id);
  const [size, setSize] = useState(10);
  const [pagination, setPagination] = useState({ search, size, page: 1 });
  const page = pagination.search === search && pagination.size === size ? pagination.page : 1;
  const setPage = (value: number) => setPagination({ search, size, page: value });
  const filtered = useMemo(
    () => filterSourceNodes(ordered, deferredSearch, Boolean(selected)),
    [ordered, deferredSearch, selected],
  );
  const pages = Math.max(1, Math.ceil(filtered.length / size));
  const current = Math.min(page, pages);
  if (pagination.search !== search || pagination.size !== size || pagination.page !== current) setPage(current);
  const pageNodes = filtered.slice((current - 1) * size, current * size);
  async function move(active: string, over: string) {
    const next = reorderVisibleNodes(order, pageNodes.map((node) => node.id), active, over);
    if (next === order || busy || readOnly || savingRef.current || !onReorder) return;
    savingRef.current = true;
    setPendingOrder(next);
    try {
      await onReorder(next);
    } catch (error) {
      if (!(error instanceof DOMException && error.name === 'AbortError')) {
        toast.add({ title: t('subscriptions.nodes.orderSaveFailed'), type: 'error' });
      }
    } finally {
      savingRef.current = false;
      setPendingOrder(null);
    }
  }
  return (
    <div className='subscription-node-list'>
      <div className='subscription-node-list__scroll subscription-state-region' aria-busy={loading}>
        {filtered.length === 0
          ? (
              loading ? <SubscriptionContentState loading /> : !error && <SubscriptionContentState kind={search ? 'search' : 'nodes'} title={t(search ? 'channels.noMatchingNodes' : 'subscriptions.nodes.empty')} />
            )
          : (
              <SubscriptionNodeSortContext
                items={pageNodes.map((node) => ({ id: node.id, label: node.name }))}
                instructions={t('subscriptions.nodes.dragInstructions')}
                onMove={move}
                renderOverlay={(id) => {
                  const node = byID.get(id);
                  return node && (
                    <article className={cn('subscription-node-card subscription-node-drag-overlay', node.hidden && 'is-hidden')} aria-hidden='true'>
                      <SubscriptionNodeCardContent node={node} />
                      {node.hidden && (
                        <div className='subscription-node-hidden'>
                          <EyeOff />
                          <span>{t('subscriptions.nodes.hidden')}</span>
                        </div>
                      )}
                    </article>
                  );
                }}
              >
                <div className='subscription-node-grid'>
                  {pageNodes.map((node) => (
                    <SourceNodeCard
                      key={node.id}
                      node={node}
                      busy={busy || busyNodes?.has(node.id)}
                      readOnly={readOnly}
                      selected={selected}
                      onSelect={onSelect}
                      onOpen={onOpen}
                      onVisibility={onVisibility}
                    />
                  ))}
                </div>
              </SubscriptionNodeSortContext>
            )}
      </div>
      <ListPagination
        page={current}
        pages={pages}
        pageSize={size}
        disabled={filtered.length === 0}
        onPageChange={setPage}
        onPageSizeChange={setSize}
      />
    </div>
  );
}
