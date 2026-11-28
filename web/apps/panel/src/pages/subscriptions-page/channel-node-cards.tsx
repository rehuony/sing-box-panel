import type { ComponentProps, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import { useSortable } from '@dnd-kit/sortable';
import { memo, useId, useMemo, useRef } from 'react';

import type { ChannelRuleGroup, SubscriptionFormat, SubscriptionNode } from '@/api/api-client';

import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';
import { dropSlotCard, SortContext, useDraggedId, useSortWindow } from '@/components/sortable';

import type { NodeCardRow } from './subscription-node-card';

import { groupTypeName } from './channel-policy';
import { BUILTIN_EXIT_NAMES } from './channel-exits';
import { NodeCardContent } from './subscription-node-card';
import { nodeCardFace, nodeCardGrid } from './node-card-classes';

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
  /** The channel's client, which names group types. */
  format: SubscriptionFormat;
  onToggle: (id: string) => void;
  onDraggingChange?: (dragging: boolean) => void;
  onMove: (active: string, over: string) => void;
}

/**
 * A candidate as a node card: its name, then why it cannot be chosen or that
 * it is unavailable, or else its protocol, strategy or built-in mark, ending
 * the title bar; below, a pool node's rows, a group's type and members, a
 * built-in exit's effect, or the ID of a node no longer in the pool. A
 * candidate that cannot be chosen dims all but why.
 */
const CandidateContent = memo(({ format, item }: { item: ChannelNodeCard; format: SubscriptionFormat }) => {
  const { t } = useTranslation();
  const { builtin, disabledReason, group, node, unavailable } = item;
  const noteText = disabledReason ?? (unavailable ? t('channels.unavailable') : '');
  const note = noteText ? { text: noteText, warning: unavailable } : undefined;
  let name = t('channels.missingNode');
  let kind: string | undefined;
  let rows: NodeCardRow[] = [{ label: t('subscriptions.pool.card.id'), value: item.id.replace(/^node:/, '') }];
  if (node) {
    name = node.display_name;
    kind = node.protocol;
  } else if (group) {
    name = group.name;
    kind = groupTypeName(group.type, format);
    rows = [
      { label: t('subscriptions.pool.card.kind'), value: t('channels.strategyGroup') },
      { label: t('subscriptions.pool.card.members'), value: t('subscriptions.pool.card.memberCount', { count: group.members.length }) },
    ];
  } else if (builtin) {
    name = BUILTIN_EXIT_NAMES[builtin];
    kind = t('channels.builtin');
    rows = [{ label: t('subscriptions.pool.card.effect'), value: t(`channels.builtinEffect.${builtin}`) }];
  }
  return <NodeCardContent dimClassName='group-data-[disabled=true]/candidate:opacity-50' kind={kind} name={name} node={node} note={note} rows={rows} />;
});
CandidateContent.displayName = 'CandidateContent';

/** A candidate's face: the node card, which a disabled candidate dims through its group. */
const candidateFace = cn('group/candidate', nodeCardFace);

/** A titled part of a list of node cards: a hairline heading with its count over its cards. */
export function NodeCardSection({ children, heading }: { heading: string; children: ReactNode }) {
  return (
    <section aria-label={heading} className='flex flex-col gap-2'>
      <h3 className='flex items-center gap-2 text-xs text-[var(--color-text-secondary)] after:h-px after:flex-1 after:bg-border'>
        {heading}
      </h3>
      {children}
    </section>
  );
}

const SortableCard = memo(({ format, item, onToggle }: {
  item: ChannelNodeCard;
  format: SubscriptionFormat;
  onToggle: Props['onToggle'];
}) => {
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
      className={cn(
        candidateFace,
        'cursor-pointer touch-pan-y outline-none select-none transition-colors duration-[var(--dur-short)] ease-out focus-visible:border-primary data-[disabled=true]:cursor-default',
        // A chosen candidate takes the accent's soft fill, a 50% accent hairline and a 25% accent
        // divider, as a selected pane does.
        item.selected
          ? 'border-primary/50 bg-accent [&_header]:border-primary/25'
          : 'bg-[var(--color-surface-translucent)] hover:not-data-[disabled=true]:border-[var(--color-border-strong)]',
        isDragging && dropSlotCard,
      )}
      data-selected={item.selected}
      data-disabled={item.disabled || undefined}
      data-dragging={isDragging || undefined}
      style={{ transform: CSS.Translate.toString(transform), transition: reducedMotion ? 'none' : transition }}
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
      <CandidateContent format={format} item={item} />
    </div>
  );
});
SortableCard.displayName = 'SortableCard';

/** The candidates' grid: every card of a short list, and of a long one the cards near the view and the one dragged. */
function CandidateGrid({ format, items, onToggle, ...handlers }: {
  items: ChannelNodeCard[];
  format: SubscriptionFormat;
  onToggle: Props['onToggle'];
} & Pick<ComponentProps<'div'>, 'onClickCapture' | 'onKeyDownCapture' | 'onPointerDownCapture'>) {
  const ids = useMemo(() => items.map(item => item.id), [items]);
  const { ref, slots, style } = useSortWindow(ids, useDraggedId());
  return (
    <div className={nodeCardGrid} data-slot='channel-candidates' ref={ref} style={style} {...handlers}>
      {slots.map(slot => (typeof slot === 'number'
        ? <SortableCard format={format} item={items[slot]!} key={items[slot]!.id} onToggle={onToggle} />
        : <div key='gap' {...slot} />))}
    </div>
  );
}

/** A group's candidates as cards in their order: a click or Space selects one, F2 moves it. */
export function ChannelNodeCards({ format, items, onToggle, onMove, onDraggingChange }: Props) {
  const { t } = useTranslation();
  const suppressClickRef = useRef(false);
  const draggingRef = useRef(false);
  if (!items.length) return null;
  return (
    <SortContext
      ended={t('channels.nodeDragEnd')}
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
          <div className={cn(candidateFace, 'cursor-grabbing bg-popover shadow-[var(--shadow-panel)] data-[selected=true]:bg-accent')} data-selected={dragged.selected} aria-hidden='true'>
            <CandidateContent format={format} item={dragged} />
          </div>
        );
      }}
    >
      <CandidateGrid
        format={format}
        items={items}
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
        }}
        onToggle={onToggle}
      />
    </SortContext>
  );
}

/** A node a group's match takes, as a candidate card that is neither selectable nor sortable. */
const MatchedCard = memo(({ format, node }: { node: SubscriptionNode; format: SubscriptionFormat }) => {
  const item = useMemo(() => ({ id: `node:${node.id}`, label: node.display_name, node, selected: false }), [node]);
  return (
    <li aria-label={node.display_name} className={cn(candidateFace, 'bg-[var(--color-surface-translucent)] select-none')}>
      <CandidateContent format={format} item={item} />
    </li>
  );
});
MatchedCard.displayName = 'MatchedCard';

/**
 * The pool nodes a group's match takes, after its members under a hairline
 * heading with their count: cards like the members', neither selectable
 * nor sortable, those of a long list drawn near the view.
 */
export function ChannelMatchedCards({ format, nodes }: {
  /** The channel's client. */
  format: SubscriptionFormat;
  nodes: readonly SubscriptionNode[];
}) {
  const { t } = useTranslation();
  const ids = useMemo(() => nodes.map(node => node.id), [nodes]);
  const { ref, slots, style } = useSortWindow(ids);
  if (!nodes.length) return null;
  return (
    <NodeCardSection heading={t('channels.autoMatched', { count: nodes.length })}>
      <ul className={nodeCardGrid} ref={ref} style={style}>
        {slots.map(slot => (typeof slot === 'number'
          ? <MatchedCard format={format} key={nodes[slot]!.id} node={nodes[slot]!} />
          : <li key='gap' {...slot} />))}
      </ul>
    </NodeCardSection>
  );
}
