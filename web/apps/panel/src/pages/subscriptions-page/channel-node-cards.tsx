import { cn } from '@shared/lib/utils';
import { CSS } from '@dnd-kit/utilities';
import { memo, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useSortable } from '@dnd-kit/sortable';

import type { ChannelRuleGroup, SubscriptionFormat, SubscriptionNode } from '@/api/api-client';

import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';

import { dropSlotCard } from './drag-slot';
import { groupTypeName } from './channel-policy';
import { BUILTIN_EXIT_NAMES } from './channel-exits';
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
  /** The channel's client, which names group types. */
  format: SubscriptionFormat;
  onToggle: (id: string) => void;
  onDraggingChange?: (dragging: boolean) => void;
  onMove: (active: string, over: string) => void;
}

/**
 * A candidate's face: a compact node card, its title bar over two lines in
 * the data face, so a picker shows three of them a row.
 */
const candidateFace = 'group/candidate relative flex h-[var(--candidate-height)] min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border font-[family-name:var(--font-data)] text-compact';
/** Candidates as cards of their own in one grid with the panes' gaps. */
const candidateGrid = 'grid grid-cols-[repeat(auto-fill,minmax(min(100%,var(--candidate-min-width)),1fr))] content-start gap-3';

/**
 * A candidate: its name, then why it cannot be chosen or that it is
 * unavailable, or else its protocol or strategy, ending the title bar; below,
 * a pool node's address over its transport, security and origin, a group
 * over its member count, a built-in exit as one, or the ID of a node no
 * longer in the pool. A candidate that cannot be chosen dims all but why.
 */
const CandidateContent = memo(({ format, item }: { item: ChannelNodeCard; format: SubscriptionFormat }) => {
  const { t } = useTranslation();
  const { builtin, disabledReason, group, node, unavailable } = item;
  const note = disabledReason ?? (unavailable ? t('channels.unavailable') : '');
  let name = t('channels.missingNode');
  let kind = '';
  let lines = [item.id.replace(/^node:/, ''), ''];
  if (node) {
    name = node.display_name;
    kind = node.protocol;
    lines = [nodeAddress(node), [node.transport, ...nodeSecurity(node), nodeOrigin(node)].filter(Boolean).join(' · ')];
  } else if (group) {
    name = group.name;
    kind = groupTypeName(group.type, format);
    lines = [t('channels.strategyGroup'), t('channels.memberCount', { count: group.members.length })];
  } else if (builtin) {
    name = BUILTIN_EXIT_NAMES[builtin];
    lines = [t('channels.builtinNode'), ''];
  }
  const dim = 'group-data-[disabled=true]/candidate:opacity-50';
  let end = kind ? <span className={cn('flex-none text-[var(--color-text-faint)]', dim)}>{kind}</span> : null;
  if (note) {
    end = (
      <span className={cn('flex-none font-[family-name:var(--font-body)] text-xs', unavailable ? 'text-[var(--color-warning-text)]' : 'text-muted-foreground')}>
        {note}
      </span>
    );
  }
  return (
    <>
      <header className='flex h-8 flex-none items-center gap-2 border-b px-3'>
        <h2 className={cn('min-w-0 flex-1 truncate font-[family-name:var(--font-body)] text-sm font-medium', dim)}>{name}</h2>
        {end}
      </header>
      <div className={cn('flex min-h-0 flex-col px-3 pt-1.5 leading-5 whitespace-nowrap', dim)}>
        <span className='truncate'>{lines[0]}</span>
        {lines[1] ? <span className='truncate text-[var(--color-text-secondary)]'>{lines[1]}</span> : null}
      </div>
    </>
  );
});
CandidateContent.displayName = 'CandidateContent';

function SortableCard({ format, item, onToggle }: { item: ChannelNodeCard; format: SubscriptionFormat; onToggle: Props['onToggle'] }) {
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
}

/** A group's candidates as cards in their order: a click or Space selects one, F2 moves it. */
export function ChannelNodeCards({ format, items, onToggle, onMove, onDraggingChange }: Props) {
  const { t } = useTranslation();
  const suppressClickRef = useRef(false);
  const draggingRef = useRef(false);
  if (!items.length) return null;
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
          <div className={cn(candidateFace, 'cursor-grabbing bg-popover shadow-[var(--shadow-panel)] data-[selected=true]:bg-accent')} data-selected={dragged.selected} aria-hidden='true'>
            <CandidateContent format={format} item={dragged} />
          </div>
        );
      }}
    >
      <div data-slot='channel-candidates' className={candidateGrid}
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
        {items.map((item) => <SortableCard format={format} key={item.id} item={item} onToggle={onToggle} />)}
      </div>
    </SubscriptionNodeSortContext>
  );
}

/**
 * The pool nodes a group's match takes, after its members under a hairline
 * heading with their count: cards like the members', neither selectable
 * nor sortable.
 */
export function ChannelMatchedCards({ format, nodes }: {
  /** The channel's client. */
  format: SubscriptionFormat;
  nodes: readonly SubscriptionNode[];
}) {
  const { t } = useTranslation();
  if (!nodes.length) return null;
  const heading = t('channels.autoMatched', { count: nodes.length });
  return (
    <section aria-label={heading} className='flex flex-col gap-2'>
      <h3 className='flex items-center gap-2 text-xs text-[var(--color-text-secondary)] after:h-px after:flex-1 after:bg-border'>
        {heading}
      </h3>
      <ul className={candidateGrid}>
        {nodes.map(node => (
          <li aria-label={node.display_name} className={cn(candidateFace, 'bg-[var(--color-surface-translucent)] select-none')} key={node.id}>
            <CandidateContent
              format={format}
              item={{ id: `node:${node.id}`, label: node.display_name, node, selected: false }}
            />
          </li>
        ))}
      </ul>
    </section>
  );
}
