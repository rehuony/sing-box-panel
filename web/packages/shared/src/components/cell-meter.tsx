import type { CSSProperties } from 'react';

import { cn } from '@shared/lib/utils';

interface CellMeterProps {
  /** Fills its container's width with this many cells instead of ten cells of the monospace grid. */
  cells?: number;
  /** The share, from 0 to 100. */
  percent: number;
  className?: string;
  /** Colours the filled cells: success for a healthy share, warning or danger as it nears its limit. */
  tone?: 'success' | 'warning' | 'danger';
}

/**
 * A share drawn as cells filled to the nearest cell; a share under half a
 * percent fills none. By default ten cells of the monospace grid; with a cell
 * count, that many cells across its container. Its number beside it carries
 * the value, so the meter itself is hidden from assistive technology.
 */
export function CellMeter({ cells, className, percent, tone }: CellMeterProps) {
  const count = cells ?? 10;
  const filled = percent < 0.5 ? 0 : Math.max(1, Math.min(count, Math.round(percent / 100 * count)));
  return (
    <span
      aria-hidden='true'
      className={cn('cell-meter', className)}
      data-cells={cells === undefined ? undefined : ''}
      data-tone={tone}
      style={{ '--meter-cells': count, '--meter-fill': `${filled / count * 100}%` } as CSSProperties}
    />
  );
}
