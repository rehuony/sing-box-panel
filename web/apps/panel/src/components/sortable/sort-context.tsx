import type { ReactNode } from 'react';
import type { SortingStrategy } from '@dnd-kit/sortable';

import { createPortal } from 'react-dom';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { rectSortingStrategy, SortableContext, sortableKeyboardCoordinates } from '@dnd-kit/sortable';
import { closestCenter, DndContext, DragOverlay, KeyboardSensor, MouseSensor, TouchSensor, useSensor, useSensors } from '@dnd-kit/core';

import { showGrabbing } from './drag-slot';

interface Props {
  /** What a drop announces, such as that the order changed. */
  ended: string;
  children: ReactNode;
  instructions: string;
  /** How the items make room while one drags; by default a grid. */
  strategy?: SortingStrategy;
  items: { id: string; label: string }[];
  renderOverlay: (id: string) => ReactNode;
  onDraggingChange?: (dragging: boolean) => void;
  onMove: (active: string, over: string) => void;
}

/**
 * Sorts a list or grid by dragging with the mouse, a touch or the keyboard:
 * F2 picks an item up and drops it, the arrow keys move it and Escape cancels;
 * a copy follows the pointer above everything while the item leaves its drop
 * slot, and the page shows the grabbing hand until the drag ends.
 */
export function SortContext({
  items, children, ended, instructions, renderOverlay, onMove, onDraggingChange, strategy = rectSortingStrategy,
}: Props) {
  const { t } = useTranslation();
  const [active, setActive] = useState<string | null>(null);
  // A drag cut short by leaving the view must not leave the hand closed.
  useEffect(() => () => showGrabbing(false), []);
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
    showGrabbing(false);
    onDraggingChange?.(false);
  }
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      accessibility={{
        // The live region sits in the body, so a dialog around the list still hides behind a nested one,
        // and no announcement lands inside a table's markup.
        container: document.body,
        screenReaderInstructions: { draggable: instructions },
        announcements: {
          onDragStart: ({ active }) => t('common.dragStart', { name: name(active.id) }),
          onDragOver: ({ over }) => over ? t('common.dragPosition', { position: items.findIndex((item) => item.id === over.id) + 1, count: items.length }) : undefined,
          onDragEnd: () => ended,
          onDragCancel: () => t('common.dragCancel'),
        },
      }}
      onDragStart={({ active }) => {
        setActive(String(active.id));
        showGrabbing(true);
        onDraggingChange?.(true);
      }}
      onDragCancel={finish}
      onDragEnd={({ active, over }) => {
        if (over) onMove(String(active.id), String(over.id));
        finish();
      }}
    >
      <SortableContext items={items.map((item) => item.id)} strategy={strategy}>
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
