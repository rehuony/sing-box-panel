import { useRef } from 'react';
import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import { useSortable } from '@dnd-kit/sortable';
import { useReducedMotion } from 'motion/react';

import type { SubscriptionNodeSummary } from '@/api/api-client';

import { SubscriptionNodeCardContent } from './subscription-node-card';
import { SubscriptionNodeSortContext } from './subscription-node-sort-context';

export interface ChannelNodeCard {
  id: string;
  label: string;
  selected: boolean;
  disabled?: boolean;
  unavailable?: boolean;
  builtin?: 'direct' | 'reject';
  node?: SubscriptionNodeSummary;
}

interface Props {
  items: ChannelNodeCard[];
  onToggle: (id: string) => void;
  onDraggingChange?: (dragging: boolean) => void;
  onMove: (active: string, over: string) => void;
}

function SortableCard({ item, onToggle }: { item: ChannelNodeCard; onToggle: Props['onToggle'] }) {
  const reducedMotion = useReducedMotion();
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
      aria-keyshortcuts='F2'
      tabIndex={item.disabled ? -1 : 0}
      className='subscription-node-card channel-node-card select-none'
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
      <SubscriptionNodeCardContent node={item.node} builtin={item.builtin} unavailable={item.unavailable} />
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
          <div className='subscription-node-card channel-node-card channel-node-card-overlay' data-selected={dragged.selected} aria-hidden='true'>
            <SubscriptionNodeCardContent
              node={dragged.node}
              builtin={dragged.builtin}
              unavailable={dragged.unavailable}
            />
          </div>
        );
      }}
    >
      <div className='subscription-node-grid channel-candidates'
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
