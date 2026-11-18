import { useId, useRef } from 'react';
import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import { useSortable } from '@dnd-kit/sortable';
import { useReducedMotion } from 'motion/react';

import type { ChannelRuleGroup, SubscriptionNodeSummary } from '@/api/api-client';

import { SubscriptionNodeCardContent } from './subscription-node-card';
import { SubscriptionNodeSortContext } from './subscription-node-sort-context';

export interface ChannelNodeCard {
  id: string;
  label: string;
  selected: boolean;
  disabled?: boolean;
  unavailable?: boolean;
  disabledReason?: string;
  group?: ChannelRuleGroup;
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
  const reasonId = useId();
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
      aria-describedby={item.disabledReason ? reasonId : attributes['aria-describedby']}
      aria-keyshortcuts='F2'
      tabIndex={item.disabled ? -1 : 0}
      className='relative isolate flex min-h-46 min-w-0 flex-col gap-2 overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] p-3 [&>header]:relative [&>header]:z-1 [&>header]:flex [&>header]:items-center [&>header]:gap-1.5 cursor-pointer touch-pan-y outline-none select-none hover:not-data-[disabled=true]:border-[var(--color-border-strong)] focus-visible:ring-2 focus-visible:ring-ring data-[disabled=true]:cursor-default data-[disabled=true]:opacity-50 data-[dragging=true]:opacity-30 data-[selected=true]:border-primary/50 data-[selected=true]:bg-accent select-none'
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
      <SubscriptionNodeCardContent
        node={item.node} builtin={item.builtin} group={item.group}
        unavailable={item.unavailable} disabledReason={item.disabledReason}
      />
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
          <div className='relative isolate flex min-h-46 min-w-0 flex-col gap-2 overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] p-3 [&>header]:relative [&>header]:z-1 [&>header]:flex [&>header]:items-center [&>header]:gap-1.5 cursor-pointer touch-pan-y outline-none select-none hover:not-data-[disabled=true]:border-[var(--color-border-strong)] focus-visible:ring-2 focus-visible:ring-ring data-[disabled=true]:cursor-default data-[disabled=true]:opacity-50 data-[dragging=true]:opacity-30 data-[selected=true]:border-primary/50 data-[selected=true]:bg-accent h-full cursor-grabbing shadow-[var(--shadow-panel)]' data-selected={dragged.selected} aria-hidden='true'>
            <SubscriptionNodeCardContent
              node={dragged.node}
              group={dragged.group}
              disabledReason={dragged.disabledReason}
              builtin={dragged.builtin}
              unavailable={dragged.unavailable}
            />
          </div>
        );
      }}
    >
      <div data-slot='channel-candidates' className='grid grid-cols-[repeat(auto-fill,min(100%,18rem))] content-start gap-3 p-0.5 content-start'
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
