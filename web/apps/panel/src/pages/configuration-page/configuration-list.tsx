import type { ComponentProps, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Button } from '@shared/components/ui/button';

// Tag, type, summary and actions; every collection row shares these columns.
const columns = 'grid grid-cols-[minmax(100px,1fr)_minmax(80px,0.65fr)_minmax(80px,1.2fr)_200px] items-center gap-4 px-3 py-2';

/**
 * A standalone collection: a header row, one row per entry and an add row.
 * It is an editor's list of entries laid out on a grid, not a data table, so
 * it keeps table semantics with markup of its own and scrolls sideways in its
 * own box when narrow.
 */
export function ConfigurationList({ className, labels, children, ...props }: ComponentProps<'table'> & {
  labels: string[];
}) {
  return (
    <div className='relative w-full overflow-x-auto'>
      <table className={cn('mt-4 block w-full min-w-160 text-sm', className)} {...props}>
        <thead className='block'>
          <tr className={cn(columns, 'min-h-9 rounded-[var(--radius-control)] bg-[var(--color-surface-raised)]')}>
            {labels.map(label => (
              <th className='min-w-0 text-center align-middle text-xs font-medium whitespace-nowrap text-muted-foreground' key={label} scope='col'>
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className='block'>{children}</tbody>
      </table>
    </div>
  );
}

export function ConfigurationListRow({ children }: { children: ReactNode }) {
  return (
    <tr className={cn(columns, 'min-h-15 border-b transition-colors hover:bg-[var(--color-hover)] has-aria-expanded:bg-[var(--color-hover)] [&:has(+[data-add-row])]:border-b-0')}>
      {children}
    </tr>
  );
}

export function ConfigurationListCell({ children, secondary = false }: { children: ReactNode; secondary?: boolean }) {
  return <td className={cn('min-w-0 text-center align-middle whitespace-nowrap', secondary && 'text-muted-foreground')}>{children}</td>;
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
    <tr className='block pt-2' data-add-row>
      <td className='block' colSpan={4}>{children}</td>
    </tr>
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
