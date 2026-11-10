import type { ReactNode } from 'react';

import { useId } from 'react';
import { cn } from '@shared/lib/utils';
import { Link } from 'react-router-dom';
import { ArrowRight } from 'lucide-react';

interface OverviewRegionProps {
  title: string;
  /** A short status at the end of the header, before the link. */
  meta?: ReactNode;
  className?: string;
  children: ReactNode;
  /** Where the region's full view lives, with the link's label. */
  more?: { label: string; to: string };
}

/** One functional area of the overview: a header row, then its content on the card surface. */
export function OverviewRegion({ children, className, meta, more, title }: OverviewRegionProps) {
  const id = useId();
  return (
    <section aria-labelledby={id} className={cn('@container flex min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]', className)}>
      <header className='flex h-11 shrink-0 items-center gap-3 border-b px-4'>
        <h2 className='truncate text-sm font-medium' id={id}>{title}</h2>
        <div className='ms-auto flex min-w-0 items-center gap-3'>
          {meta}
          {more
            ? (
                <Link className='flex shrink-0 items-center gap-1 rounded-[var(--radius-control)] text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring' to={more.to}>
                  {more.label}
                  <ArrowRight aria-hidden='true' className='size-3.5' />
                </Link>
              )
            : null}
        </div>
      </header>
      {children}
    </section>
  );
}
