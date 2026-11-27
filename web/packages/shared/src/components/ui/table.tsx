import type { VariantProps } from 'class-variance-authority';

import * as React from 'react';
import { cn } from '@shared/lib/utils';
import { cva } from 'class-variance-authority';

/**
 * The data table: a header kept in view on the raised surface without a
 * divider, then rows divided by hairlines, 44px for one short reading a cell
 * or, comfortable, the workspace row height; cells inset 16px, 12px on a
 * phone. It scrolls in the region that holds it, edge to edge on its card, so
 * its header stays in view.
 */
function Table({ className, density = 'compact', ...props }: React.ComponentProps<'table'> & {
  density?: 'compact' | 'comfortable';
}) {
  return (
    <table
      data-density={density}
      data-slot='table'
      className={cn('w-full flex-none table-fixed border-collapse text-sm', className)}
      {...props}
    />
  );
}

function TableHeader({ className, ...props }: React.ComponentProps<'thead'>) {
  return (
    <thead
      data-slot='table-header'
      className={cn('sticky top-0 z-1 bg-[var(--color-surface-raised)] text-muted-foreground [&_tr]:h-11 [&_tr]:border-0', className)}
      {...props}
    />
  );
}

function TableBody({ className, ...props }: React.ComponentProps<'tbody'>) {
  return <tbody data-slot='table-body' className={className} {...props} />;
}

/**
 * A row. An interactive one opens its record from anywhere on it and takes
 * the hover wash, which also marks its title, `data-row-title`, while that
 * has keyboard focus; a row that opens nothing takes no wash.
 */
function TableRow({ className, interactive = false, ...props }: React.ComponentProps<'tr'> & { interactive?: boolean }) {
  return (
    <tr
      data-slot='table-row'
      className={cn(
        'h-11 border-b in-data-[density=comfortable]:h-[var(--workspace-row-height)] data-[state=selected]:bg-accent',
        interactive && 'cursor-pointer transition-colors duration-[var(--dur-micro)] ease-out hover:bg-[var(--color-hover)] has-[[data-row-title]:focus-visible]:bg-[var(--color-hover)] motion-reduce:transition-none',
        className,
      )}
      {...props}
    />
  );
}

function TableHead({ className, scope = 'col', ...props }: React.ComponentProps<'th'>) {
  return (
    <th
      data-slot='table-head'
      scope={scope}
      className={cn('px-4 text-start align-middle font-medium whitespace-nowrap max-sm:px-3', className)}
      {...props}
    />
  );
}

const tableCellVariants = cva('px-4 align-middle max-sm:px-3 in-data-[density=comfortable]:py-3', {
  variants: {
    kind: {
      /** One line, cut with an ellipsis. */
      text: 'truncate',
      /** Text a comfortable row may wrap, anywhere in a long identifier. */
      wrap: 'wrap-anywhere',
      /** A reading in the data face, secondary and cut with an ellipsis. */
      data: 'truncate font-[family-name:var(--font-data)] text-compact text-[var(--color-text-secondary)] tabular-nums',
      /** A count or measure in the data face, at the end of its column. */
      number: 'text-end font-[family-name:var(--font-data)] text-compact whitespace-nowrap tabular-nums',
      /** The row's actions together at its end. */
      actions: '',
    },
  },
  defaultVariants: { kind: 'text' },
});

function TableCell({ children, className, kind, ...props }: React.ComponentProps<'td'> & VariantProps<typeof tableCellVariants>) {
  return (
    <td data-slot='table-cell' className={cn(tableCellVariants({ kind }), className)} {...props}>
      {kind === 'actions' ? <div className='flex items-center justify-end gap-0.5'>{children}</div> : children}
    </td>
  );
}

export {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
};
