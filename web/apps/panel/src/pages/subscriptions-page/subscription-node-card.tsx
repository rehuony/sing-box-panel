import type { KeyboardEvent, ReactNode } from 'react';

import { memo } from 'react';
import { cn } from '@shared/lib/utils';
import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import { useSortable } from '@dnd-kit/sortable';
import { EM_DASH } from '@shared/lib/telemetry-format';
import { ReadoutRow } from '@shared/components/terminal';

import type { SubscriptionNode } from '@/api/api-client';

import { dropSlotCard } from '@/components/sortable';
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';

import { nodeCardFace } from './node-card-classes';
import { nodeAddress, nodeOrigin, nodeSecurity } from './subscription-node-pool';

/** Cards that always paint: more than a large window shows at once. */
const EAGER_CARDS = 48;

/** Parts of a value split by faint dots. */
function Parts({ parts }: { parts: string[] }) {
  return parts.map((part, index) => (
    // A value's parts keep their order; their position is their identity.
    // eslint-disable-next-line react/no-array-index-key
    <span key={index}>
      {index > 0 ? <span aria-hidden='true' className='text-[var(--color-text-faint)]'>{' · '}</span> : null}
      {part}
    </span>
  ));
}

/** One labelled row of a node card. */
export interface NodeCardRow {
  label: string;
  value: ReactNode;
}

interface NodeCardContentProps {
  name: string;
  /** What ends the title bar: a node's protocol, a group's strategy or a built-in exit's mark. */
  kind?: string;
  /** Dims all but the note, for a card that cannot be chosen. */
  dimClassName?: string;
  /** A pool node, whose rows are where it connects, its transport and security, its SNI and its origin. */
  node?: SubscriptionNode;
  /** Without a node, the rows, such as a group's type and members or a built-in exit's effect. */
  rows?: readonly NodeCardRow[];
  /** What ends the title bar in the kind's place, such as why the card cannot be chosen. */
  note?: { text: string; warning?: boolean };
}

/** A pool node's rows, each value "—" when the node has none. */
function useNodeRows(node: SubscriptionNode | undefined): NodeCardRow[] {
  const { t } = useTranslation();
  if (!node) return [];
  const layers = [node.transport, ...nodeSecurity(node)].filter((part): part is string => Boolean(part));
  return [
    { label: t('subscriptions.pool.card.server'), value: nodeAddress(node) },
    { label: t('subscriptions.pool.card.transport'), value: layers.length ? <Parts parts={layers} /> : null },
    { label: t('subscriptions.pool.card.sni'), value: node.sni },
    { label: t('subscriptions.pool.card.origin'), value: nodeOrigin(node) },
  ];
}

/**
 * A node card's content, the same wherever a pool node, a group or a
 * built-in exit shows as a card: the name with its kind or a note at the end
 * of the title bar, then labelled rows in the data face, a value it lacks
 * reading "—".
 */
export function NodeCardContent({ dimClassName, kind, name, node, note, rows = [] }: NodeCardContentProps) {
  const nodeRows = useNodeRows(node);
  let end: ReactNode = kind ? <span className={cn('flex-none text-[var(--color-text-faint)]', dimClassName)}>{kind}</span> : null;
  if (note) {
    end = (
      <span className={cn('flex-none font-[family-name:var(--font-body)] text-xs', note.warning ? 'text-[var(--color-warning-text)]' : 'text-muted-foreground')}>
        {note.text}
      </span>
    );
  }
  return (
    <>
      <header className='flex h-8 flex-none items-center gap-2 border-b px-3'>
        <h2 className={cn('min-w-0 flex-1 truncate font-[family-name:var(--font-body)] text-sm font-medium', dimClassName)}>{name}</h2>
        {end}
      </header>
      {/* Each row keeps to one line, so every card holds one height. */}
      <dl className={cn('grid min-h-0 grid-cols-[max-content_minmax(0,1fr)] content-start gap-x-3 px-3 py-2.5 leading-5 whitespace-nowrap', dimClassName)}>
        {(node ? nodeRows : rows).map(row => (
          <ReadoutRow key={row.label} label={row.label}>
            {row.value ? <span className='min-w-0 truncate'>{row.value}</span> : <span className='text-[var(--color-text-faint)]'>{EM_DASH}</span>}
          </ReadoutRow>
        ))}
      </dl>
    </>
  );
}

/** A pool node's card content: its name and protocol over its rows. */
export function PoolNodeCardContent({ node }: { node: SubscriptionNode }) {
  return <NodeCardContent kind={node.protocol} name={node.display_name} node={node} />;
}

/** The cell as the pointer carries it: a card lifted with the panel shadow above everything. */
export function PoolNodeCardOverlay({ node }: { node: SubscriptionNode }) {
  return (
    <div aria-hidden='true' className={cn(nodeCardFace, 'cursor-grabbing bg-popover shadow-[var(--shadow-panel)]')}>
      <PoolNodeCardContent node={node} />
    </div>
  );
}

interface PoolNodeCardProps {
  /** Where the card sits in the grid; cards past the first screenful paint only near the view. */
  index: number;
  node: SubscriptionNode;
  onOpen: (node: SubscriptionNode) => void;
}

/**
 * One pool node, a card of its own in the pool's grid. The
 * whole card drags with the mouse, a held touch or F2, unless the pool's sort
 * context is locked, and its slot stays while it moves; a click, Enter or
 * Space opens its detail.
 */
export const PoolNodeCard = memo(({ index, node, onOpen }: PoolNodeCardProps) => {
  const { t } = useTranslation();
  const reducedMotion = usePrefersReducedMotion();
  const { attributes, isDragging, listeners, setNodeRef, transform, transition } = useSortable({ id: node.id });

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget) return;
    if (!isDragging && (event.key === 'Enter' || event.key === ' ')) {
      event.preventDefault();
      onOpen(node);
      return;
    }
    listeners?.onKeyDown?.(event);
  }

  return (
    // The list item moves as a whole, so a card past the first screenful is not clipped by its own containment.
    <li
      className={cn('min-w-0', index >= EAGER_CARDS && '[contain-intrinsic-size:auto_var(--node-card-height)] [content-visibility:auto]')}
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition: reducedMotion ? undefined : transition }}
    >
      <div
        {...attributes}
        {...listeners}
        aria-disabled={undefined}
        aria-keyshortcuts='F2'
        aria-label={node.display_name}
        aria-roledescription={t('subscriptions.pool.sortableCard')}
        className={cn(
          nodeCardFace,
          'cursor-pointer touch-pan-y bg-[var(--color-surface-translucent)] outline-none select-none transition-colors duration-[var(--dur-short)] ease-out hover:border-[var(--color-border-strong)] focus-visible:border-primary',
          isDragging && dropSlotCard,
        )}
        data-dragging={isDragging || undefined}
        onClick={() => onOpen(node)}
        onKeyDown={onKeyDown}
      >
        <PoolNodeCardContent node={node} />
      </div>
    </li>
  );
});
PoolNodeCard.displayName = 'PoolNodeCard';
