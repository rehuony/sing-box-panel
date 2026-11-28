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

import { nodeAddress, nodeOrigin, nodeSecurity } from './subscription-node-pool';

/** Cards that always paint: more than a large window shows at once. */
const EAGER_CARDS = 48;

/** A pane's face: a card in the pool's grid, or the card the pointer carries. */
const face = 'relative flex h-[var(--pane-height)] min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border font-[family-name:var(--font-data)] text-compact';

/** Parts of a value split by faint dots. */
function Parts({ parts }: { parts: string[] }) {
  return parts.map((part, index) => (
    <span key={part}>
      {index > 0 ? <span aria-hidden='true' className='text-muted-foreground'>{' · '}</span> : null}
      {part}
    </span>
  ));
}

/** One labelled row on one line, or "—" when the node has none; the detail shows a cut value in full. */
function Row({ children, className, label, value }: {
  label: string;
  value: string | undefined;
  className?: string;
  /** How the value shows when not as plain text. */
  children?: ReactNode;
}) {
  return (
    <ReadoutRow label={label}>
      {value ? <span className={cn('min-w-0 truncate', className)}>{children ?? value}</span> : <span className='text-muted-foreground'>{EM_DASH}</span>}
    </ReadoutRow>
  );
}

/**
 * A pool node as a pane: its title bar holds the display name alone, then in
 * the data face labelled rows for its protocol, where it connects, its
 * transport, its security layer and SNI, and where it comes from.
 */
export function PoolNodeCardContent({ node }: { node: SubscriptionNode }) {
  const { t } = useTranslation();
  const security = nodeSecurity(node);
  return (
    <>
      <header className='flex h-8 flex-none items-center border-b px-3'>
        <h2 className='min-w-0 truncate font-[family-name:var(--font-body)] text-sm font-medium'>{node.display_name}</h2>
      </header>
      {/* Each row keeps to one line, so the card holds the panes' height. */}
      <dl className='grid min-h-0 flex-1 grid-cols-[max-content_minmax(0,1fr)] content-start gap-x-3 px-3 py-3 whitespace-nowrap'>
        <Row label={t('subscriptions.pool.card.type')} value={node.protocol} />
        <Row label={t('subscriptions.pool.card.server')} value={nodeAddress(node)} />
        <Row label={t('subscriptions.pool.card.transport')} value={node.transport} />
        <Row label={t('subscriptions.pool.card.security')} value={security.join(' · ')}><Parts parts={security} /></Row>
        <Row label={t('subscriptions.pool.card.sni')} value={node.sni} />
        <Row className='text-[var(--color-text-secondary)]' label={t('subscriptions.pool.card.origin')} value={nodeOrigin(node)} />
      </dl>
    </>
  );
}

/** The cell as the pointer carries it: a card lifted with the panel shadow above everything. */
export function PoolNodeCardOverlay({ node }: { node: SubscriptionNode }) {
  return (
    <div aria-hidden='true' className={cn(face, 'cursor-grabbing bg-popover shadow-[var(--shadow-panel)]')}>
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
 * One pool node, a card of its own in the pool's grid like a fleet pane. The
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
      className={cn('min-w-0', index >= EAGER_CARDS && '[contain-intrinsic-size:auto_var(--pane-height)] [content-visibility:auto]')}
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
          face,
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
