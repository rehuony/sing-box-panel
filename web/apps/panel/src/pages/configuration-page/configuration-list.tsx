import type { ComponentProps, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Button } from '@shared/components/ui/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@shared/components/ui/table';

// Tag, type, summary and actions; every collection row shares these columns.
const columns = 'grid grid-cols-[minmax(100px,1fr)_minmax(80px,0.65fr)_minmax(80px,1.2fr)_200px] items-center gap-4 px-3 py-2';

/** A standalone collection: a header row, one row per entry and an add row. */
export function ConfigurationList({ className, labels, children, ...props }: ComponentProps<typeof Table> & {
  labels: string[];
}) {
  return (
    <Table className={cn('mt-4 block min-w-160', className)} {...props}>
      <TableHeader className='block'>
        <TableRow className={cn(columns, 'min-h-9 rounded-[var(--radius-control)] border-0 bg-[var(--color-surface-raised)] hover:bg-[var(--color-surface-raised)]')}>
          {labels.map(label => (
            <TableHead className='h-auto min-w-0 p-0 text-center text-xs font-medium text-muted-foreground' key={label} scope='col'>
              {label}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody className='block'>{children}</TableBody>
    </Table>
  );
}

export function ConfigurationListRow({ children }: { children: ReactNode }) {
  return <TableRow className={cn(columns, 'min-h-15 border-b [&:has(+[data-add-row])]:border-b-0')}>{children}</TableRow>;
}

export function ConfigurationListCell({ children, secondary = false }: { children: ReactNode; secondary?: boolean }) {
  return <TableCell className={cn('min-w-0 p-0 text-center font-normal', secondary && 'text-muted-foreground')}>{children}</TableCell>;
}

export function ConfigurationListText({ children, title }: { children: ReactNode; title?: string }) {
  return <span className='block truncate' title={title}>{children}</span>;
}

export function ConfigurationListSummary({ children }: { children: ReactNode }) {
  return <div className='flex flex-wrap items-center justify-center gap-1 *:max-w-full'>{children}</div>;
}

export function ConfigurationListActions({ children }: { children: ReactNode }) {
  return <div className='flex items-center justify-center gap-0.5 *:data-[slot=button]:font-normal'>{children}</div>;
}

/** The last row: one wide dashed action that adds an entry. */
export function ConfigurationListAddRow({ children }: { children: ReactNode }) {
  return (
    <TableRow className='block border-0 bg-transparent px-0 pt-2 pb-0 hover:bg-transparent' data-add-row>
      <TableCell className='block p-0' colSpan={4}>{children}</TableCell>
    </TableRow>
  );
}

/** The add action inside ConfigurationListAddRow. */
export function ConfigurationListAddButton({ className, ...props }: ComponentProps<typeof Button>) {
  return (
    <Button
      className={cn('min-h-15 w-full rounded-[var(--radius-control)] border-dashed', className)}
      size='content'
      type='button'
      variant='outline'
      {...props}
    />
  );
}
