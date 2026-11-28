import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { CellMeter } from '@shared/components/cell-meter';

import { InfoTooltip } from '@/components/info-tooltip';

type Tone = 'danger' | 'warning';

const toneText: Record<Tone, string> = {
  danger: 'text-[var(--color-danger-text)]',
  warning: 'text-[var(--color-warning-text)]',
};

interface KeyFigureProps {
  /** Colours the value and its line when the figure needs attention. */
  tone?: Tone;
  /** What the label alone cannot say, behind a help trigger. */
  help?: string;
  label: string;
  /** A share drawn as cells beside the value. */
  meter?: number;
  /** One line under the value. */
  meta: ReactNode;
  /** The value in the data face. */
  children: ReactNode;
}

/** One key figure: its label, its value in the data face and one line of meta. */
export function KeyFigure({ children, help, label, meta, meter, tone }: KeyFigureProps) {
  return (
    <div className='flex min-w-0 flex-col gap-1 px-3 py-3 @3xl/figures:px-4'>
      <dt className='flex min-w-0 items-center gap-1 text-xs text-muted-foreground'>
        <span className='truncate'>{label}</span>
        {/* The trigger overlaps the line's padding so the label row keeps its height. */}
        {help ? <span className='-my-[calc((var(--control-height-xs)-1rem)/2)] flex'><InfoTooltip label={help}>{help}</InfoTooltip></span> : null}
      </dt>
      <dd className={cn('flex min-w-0 flex-wrap items-center gap-x-3 font-[family-name:var(--font-data)] text-base font-semibold tabular-nums @3xl/figures:text-xl', tone && toneText[tone])}>
        <span className='min-w-0'>{children}</span>
        {meter !== undefined
          ? (
              <span className='flex text-xs'>
                <CellMeter percent={meter} />
              </span>
            )
          : null}
      </dd>
      <dd className={cn('truncate text-xs', tone ? toneText[tone] : 'text-muted-foreground')}>{meta}</dd>
    </div>
  );
}

/**
 * Key figures in one card, divided by hairlines: four a row from 36rem of
 * their own width, so the card keeps to one row above a phone and leaves the
 * view below it its height, two a row below that; values take 20px and the
 * cells their full inset from 48rem.
 */
export function KeyFigures({ children, label }: { children: ReactNode; label: string }) {
  return (
    <div className='@container/figures flex-none'>
      <dl
        aria-label={label}
        className='grid grid-cols-2 rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)] *:border-border @xl/figures:grid-cols-4 [&>*:nth-child(even)]:border-l [&>*:nth-child(n+3)]:border-t @xl/figures:[&>*:nth-child(n+2)]:border-l @xl/figures:[&>*:nth-child(n+3)]:border-t-0'
      >
        {children}
      </dl>
    </div>
  );
}
