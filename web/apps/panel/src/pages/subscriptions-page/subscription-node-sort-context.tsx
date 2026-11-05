import type { ReactNode } from 'react';

import { useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { rectSortingStrategy, SortableContext, sortableKeyboardCoordinates } from '@dnd-kit/sortable';
import { closestCenter, DndContext, DragOverlay, KeyboardSensor, MouseSensor, TouchSensor, useSensor, useSensors } from '@dnd-kit/core';

interface Props {
  children: ReactNode;
  instructions: string;
  items: { id: string; label: string }[];
  renderOverlay: (id: string) => ReactNode;
  onDraggingChange?: (dragging: boolean) => void;
  onMove: (active: string, over: string) => void;
}

export function SubscriptionNodeSortContext({
  items, children, instructions, renderOverlay, onMove, onDraggingChange,
}: Props) {
  const { t } = useTranslation();
  const [active, setActive] = useState<string | null>(null);
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 180, tolerance: 5 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
      keyboardCodes: { start: ['F2'], end: ['F2'], cancel: ['Escape'] },
    }),
  );
  const name = (id: string | number) => items.find((item) => item.id === id)?.label ?? '';
  function finish() {
    setActive(null);
    onDraggingChange?.(false);
  }
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      accessibility={{
        screenReaderInstructions: { draggable: instructions },
        announcements: {
          onDragStart: ({ active }) => t('channels.nodeDragStart', { name: name(active.id) }),
          onDragOver: ({ over }) => over ? t('channels.nodeDragPosition', { position: items.findIndex((item) => item.id === over.id) + 1, count: items.length }) : undefined,
          onDragEnd: () => t('channels.nodeDragEnd'),
          onDragCancel: () => t('channels.nodeDragCancel'),
        },
      }}
      onDragStart={({ active }) => {
        setActive(String(active.id));
        onDraggingChange?.(true);
      }}
      onDragCancel={finish}
      onDragEnd={({ active, over }) => {
        if (over) onMove(String(active.id), String(over.id));
        finish();
      }}
    >
      <SortableContext items={items.map((item) => item.id)} strategy={rectSortingStrategy}>
        {children}
      </SortableContext>
      {createPortal(
        <DragOverlay dropAnimation={null} style={{ zIndex: 'calc(var(--z-modal) + 2)' }}>
          {active && renderOverlay(active)}
        </DragOverlay>,
        document.body,
      )}
    </DndContext>
  );
}
