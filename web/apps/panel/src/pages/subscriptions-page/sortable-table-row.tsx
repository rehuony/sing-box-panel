import type { ComponentProps } from 'react';

import { cn } from '@shared/lib/utils';
import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import { useSortable } from '@dnd-kit/sortable';
import { TableCell, TableHead, TableRow } from '@shared/components/ui/table';

import { dropSlotRow, SortGrip } from '@/components/sortable';
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';

/** The header cell above the grips of a sortable table's rows. */
export function SortableTableHead() {
  const { t } = useTranslation();
  return <TableHead className='w-10 pe-0 max-sm:pe-0'><span className='sr-only'>{t('common.order')}</span></TableHead>;
}

/**
 * A data table's row that starts with the grip that drags it into another
 * place, with the mouse, a held touch or F2 from the keyboard; its slot shows
 * where it lands while a copy follows the pointer, without moving the
 * columns. The cell after the grip
 * starts close to it.
 */
export function SortableTableRow({
  children, className, id, label, sortable, style, ...props
}: ComponentProps<typeof TableRow> & {
  id: string;
  /** What the row is, which names its grip. */
  label: string;
  /** Whether the row can be dragged, as while the list is not filtered. */
  sortable: boolean;
}) {
  const { t } = useTranslation();
  const reducedMotion = usePrefersReducedMotion();
  const { attributes, isDragging, listeners, setActivatorNodeRef, setNodeRef, transform, transition }
    = useSortable({ id, disabled: !sortable });
  return (
    <TableRow
      {...props}
      className={cn('[&>td:nth-child(2)]:ps-1', className, isDragging && dropSlotRow.row)}
      ref={setNodeRef}
      style={{
        ...style, transform: CSS.Translate.toString(transform), transition: reducedMotion ? undefined : transition,
      }}
    >
      <TableCell className='w-10 ps-3 pe-0 max-sm:ps-2 max-sm:pe-0'>
        {isDragging ? <span aria-hidden='true' className={dropSlotRow.slot} /> : null}
        <SortGrip
          activatorRef={setActivatorNodeRef}
          attributes={attributes}
          className={isDragging ? 'opacity-40' : undefined}
          disabled={!sortable}
          label={t('common.reorderItem', { name: label })}
          listeners={listeners}
        />
      </TableCell>
      {children}
    </TableRow>
  );
}
