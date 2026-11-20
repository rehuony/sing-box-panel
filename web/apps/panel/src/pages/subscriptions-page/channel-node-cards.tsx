import { cn } from '@shared/lib/utils';
import { CSS } from '@dnd-kit/utilities';
import { memo, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useSortable } from '@dnd-kit/sortable';
import { Badge } from '@shared/components/ui/badge';

import type { ChannelRuleGroup, SubscriptionNode } from '@/api/api-client';

import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';

import { SubscriptionNodeSortContext } from './subscription-node-sort-context';
import { nodeAddress, nodeOrigin, nodeSecurity } from './subscription-node-pool';

export interface ChannelNodeCard {
  id: string;
  label: string;
  selected: boolean;
  disabled?: boolean;
  unavailable?: boolean;
  disabledReason?: string;
  node?: SubscriptionNode;
  group?: ChannelRuleGroup;
  builtin?: 'direct' | 'reject';
}

interface Props {
  items: ChannelNodeCard[];
  onToggle: (id: string) => void;
  onDraggingChange?: (dragging: boolean) => void;
  onMove: (active: string, over: string) => void;
}

const badge = 'h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal';
const card = 'relative isolate flex min-h-46 min-w-0 flex-col gap-2 overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] p-3 select-none';

/**
 * A candidate's name, then badges for what it is: a pool node's origin,
 * protocol, address and security, a built-in exit or a group.
 */
const CandidateContent = memo(({ item }: { item: ChannelNodeCard }) => {
  const { t } = useTranslation();
  const { builtin, disabledReason, group, node, unavailable } = item;
  const name = node?.display_name ?? group?.name ?? builtin?.toUpperCase() ?? t('channels.missingNode');
  const address = node ? nodeAddress(node) : '';
  return (
    <>
      <header className='relative z-1 flex items-center gap-1.5'>
        <span aria-hidden='true' className={cn('mx-[3px] size-1.5 flex-none rounded-full bg-[var(--color-success)] shadow-[0_0_0_3px_var(--color-success-soft)]', unavailable && 'bg-[var(--color-text-faint)] shadow-[0_0_0_3px_var(--color-surface-raised)]')} />
        <span className='flex min-h-(--control-height-xs) min-w-0 flex-1 items-center text-left text-sm font-normal' title={name}><span className='truncate'>{name}</span></span>
      </header>
      <div className='flex min-h-0 w-full min-w-0 flex-1 flex-wrap content-start items-start gap-1 text-left'>
        {node && (
          <>
            <Badge className={badge} title={nodeOrigin(node)} variant='secondary'><span className='truncate'>{nodeOrigin(node)}</span></Badge>
            <Badge className={badge} variant='success'>{node.protocol}</Badge>
            {address ? <Badge className={badge} title={address} variant='info'><span className='truncate'>{address}</span></Badge> : null}
            {[...nodeSecurity(node), node.sni ?? ''].filter(Boolean).map(part => <Badge className={badge} key={part} title={part} variant='success'><span className='truncate'>{part}</span></Badge>)}
          </>
        )}
        {builtin && (
          <>
            <Badge className={badge} variant='secondary'>{t('channels.builtinNode')}</Badge>
            <Badge className={badge} variant={builtin === 'direct' ? 'success' : 'warning'}>{t(`channels.${builtin}`)}</Badge>
          </>
        )}
        {group && (
          <>
            <Badge className={badge} variant='secondary'>{t('channels.strategyGroup')}</Badge>
            <Badge className={badge} variant='info'>{t(`channels.groupTypes.${group.type}`)}</Badge>
          </>
        )}
        {disabledReason && <Badge className={badge} variant='secondary'>{disabledReason}</Badge>}
        {unavailable && !disabledReason && <Badge className={badge} variant='warning'>{t('channels.unavailable')}</Badge>}
      </div>
    </>
  );
});
CandidateContent.displayName = 'CandidateContent';

function SortableCard({ item, onToggle }: { item: ChannelNodeCard; onToggle: Props['onToggle'] }) {
  const reasonId = useId();
  const reducedMotion = usePrefersReducedMotion();
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: item.id, disabled: item.disabled,
  });
  return (
    <div
      ref={setNodeRef}
      {...attributes}
      {...listeners}
      role='checkbox'
      aria-checked={item.selected}
      aria-pressed={undefined}
      aria-disabled={item.disabled || undefined}
      aria-roledescription={undefined}
      aria-label={item.label}
      aria-describedby={item.disabledReason ? reasonId : attributes['aria-describedby']}
      aria-keyshortcuts='F2'
      tabIndex={item.disabled ? -1 : 0}
      className={cn(card, 'cursor-pointer touch-pan-y outline-none hover:not-data-[disabled=true]:border-[var(--color-border-strong)] focus-visible:ring-2 focus-visible:ring-ring data-[disabled=true]:cursor-default data-[disabled=true]:opacity-50 data-[dragging=true]:opacity-30 data-[selected=true]:border-primary/50 data-[selected=true]:bg-accent')}
      data-selected={item.selected}
      data-disabled={item.disabled || undefined}
      data-dragging={isDragging || undefined}
      style={{ transform: CSS.Transform.toString(transform), transition: reducedMotion ? 'none' : transition }}
      onClick={() => {
        if (!item.disabled && !isDragging) onToggle(item.id);
      }}
      onKeyDown={(event) => {
        listeners?.onKeyDown?.(event);
        if (!item.disabled && !isDragging && !event.defaultPrevented && (event.key === ' ' || event.key === 'Enter')) {
          event.preventDefault();
          onToggle(item.id);
        }
      }}
    >
      {item.disabledReason && <span id={reasonId} className='sr-only'>{item.disabledReason}</span>}
      <CandidateContent item={item} />
    </div>
  );
}

export function ChannelNodeCards({ items, onToggle, onMove, onDraggingChange }: Props) {
  const { t } = useTranslation();
  const suppressClickRef = useRef(false);
  const draggingRef = useRef(false);
  return (
    <SubscriptionNodeSortContext
      items={items}
      instructions={t('channels.nodeDragInstructions')}
      onMove={onMove}
      onDraggingChange={value => {
        draggingRef.current = value;
        if (value) suppressClickRef.current = true;
        onDraggingChange?.(value);
      }}
      renderOverlay={(id) => {
        const dragged = items.find((item) => item.id === id);
        return dragged && (
          <div className={cn(card, 'h-full cursor-grabbing shadow-[var(--shadow-panel)] data-[selected=true]:border-primary/50 data-[selected=true]:bg-accent')} data-selected={dragged.selected} aria-hidden='true'>
            <CandidateContent item={dragged} />
          </div>
        );
      }}
    >
      <div data-slot='channel-candidates' className='grid grid-cols-[repeat(auto-fill,min(100%,18rem))] content-start gap-3 p-0.5'
        onPointerDownCapture={() => {
          if (!draggingRef.current) suppressClickRef.current = false;
        }}
        onKeyDownCapture={event => {
          if (!draggingRef.current && event.key !== 'F2') suppressClickRef.current = false;
        }}
        onClickCapture={event => {
          if (suppressClickRef.current) {
            event.preventDefault();
            event.stopPropagation();
          }
        }}>
        {items.map((item) => <SortableCard key={item.id} item={item} onToggle={onToggle} />)}
      </div>
    </SubscriptionNodeSortContext>
  );
}
