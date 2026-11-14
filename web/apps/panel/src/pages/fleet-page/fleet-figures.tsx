import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { formatBytes, formatRate } from '@shared/lib/telemetry-format';

import type { FleetFigures as Figures } from '@/utils/fleet-summary';

import { UpDown } from '@/components/up-down';
import { InfoTooltip } from '@/components/info-tooltip';

interface FigureProps {
  /** What the label alone cannot say, behind a help trigger. */
  help?: string;
  label: string;
  meta: ReactNode;
  className?: string;
  children: ReactNode;
}

function Figure({ children, className, help, label, meta }: FigureProps) {
  return (
    <div className={cn('flex min-w-0 flex-col gap-1 border-border px-4 py-3', className)}>
      <dt className='flex min-w-0 items-center gap-1 text-xs text-muted-foreground'>
        <span className='truncate'>{label}</span>
        {/* The trigger overlaps the line's padding so the label row keeps its height. */}
        {help ? <span className='-my-[calc((var(--control-height-xs)-1rem)/2)] flex'><InfoTooltip label={help}>{help}</InfoTooltip></span> : null}
      </dt>
      <dd className='min-w-0 font-[family-name:var(--font-data)] text-base font-semibold tabular-nums @3xl:text-xl'>{children}</dd>
      <dd className='truncate text-xs text-muted-foreground'>{meta}</dd>
    </div>
  );
}

/**
 * The fleet's key figures in one card, divided by hairlines: who is online,
 * what runs, what flows now and what this period used, summed but never
 * against a summed quota. Two a row on narrow screens, four from 48rem.
 */
export function FleetFigures({ figures }: { figures: Figures }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  return (
    <dl
      aria-label={t('fleet.figures.label')}
      className='grid flex-none grid-cols-2 rounded-[var(--radius-card)] border bg-[var(--color-surface-dense)] @3xl:grid-cols-4'
    >
      <Figure label={t('fleet.figures.nodes')} meta={t('fleet.figures.nodesMeta', { offline: figures.offline, pending: figures.pending })}>
        {figures.online}
        <span className='text-muted-foreground'>{` / ${figures.total}`}</span>
      </Figure>
      <Figure
        className='border-l'
        label={t('fleet.figures.cores')}
        meta={t('fleet.figures.coresMeta', { stopped: figures.stopped, unknown: figures.unknown })}
      >
        {figures.running}
        <span className='text-muted-foreground'>{` / ${figures.enrolled}`}</span>
      </Figure>
      <Figure className='border-t @3xl:border-t-0 @3xl:border-l' label={t('fleet.figures.rates')} meta={t('fleet.figures.connections', { count: figures.connections })}>
        <UpDown
          download={formatRate(figures.rates.download, locale)}
          upload={formatRate(figures.rates.upload, locale)}
        />
      </Figure>
      <Figure
        className='border-t border-l @3xl:border-t-0'
        help={t('fleet.usageHint')}
        label={t('fleet.figures.usage')}
        meta={<UpDown download={formatBytes(figures.download, locale)} upload={formatBytes(figures.upload, locale)} />}
      >
        {formatBytes(figures.download + figures.upload, locale)}
      </Figure>
    </dl>
  );
}
