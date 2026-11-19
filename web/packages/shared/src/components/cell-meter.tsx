import type { CSSProperties } from 'react';
import type { MeterLimits } from '@shared/lib/telemetry-format';

import { cn } from '@shared/lib/utils';
import { SHARE_LIMITS } from '@shared/lib/telemetry-format';

interface CellMeterProps {
  /** Fills the room its row leaves with this many cells instead of ten cells of the monospace grid. */
  cells?: number;
  className?: string;
  /** Where the scale reaches warning and danger; a load's limits by default. */
  limits?: MeterLimits;
  /** The share, from 0 to 100; null draws the unknown hatch in every cell. */
  percent: number | null;
}

/**
 * A share drawn as cells filled to the nearest cell; a share under half a
 * percent fills none, and an unknown share hatches every cell. The fill
 * reveals one scale laid along the whole meter, from success at empty through
 * warning to danger at its limits, so its colour says where the share sits.
 * By default ten cells of the monospace grid; with a cell count, that many
 * cells across the room its row leaves. Its number beside it carries the
 * value, so the meter itself is hidden from assistive technology.
 */
export function CellMeter({ cells, className, limits = SHARE_LIMITS, percent }: CellMeterProps) {
  const count = cells ?? 10;
  let filled = 0;
  if (percent !== null && percent >= 0.5) filled = Math.max(1, Math.min(count, Math.round(percent / 100 * count)));
  return (
    <span
      aria-hidden='true'
      className={cn('cell-meter', className)}
      data-cells={cells === undefined ? undefined : ''}
      data-unknown={percent === null ? '' : undefined}
      style={{
        '--meter-cells': count,
        '--meter-danger': `${limits.danger}%`,
        '--meter-fill': `${filled / count * 100}%`,
        '--meter-warning': `${limits.warning}%`,
      } as CSSProperties}
    />
  );
}
