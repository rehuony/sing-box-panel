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
    <div className={cn('flex min-w-0 flex-col gap-1 rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)] p-3 @3xl:p-4', className)}>
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

/** The fleet's key figures: who is online, what runs, what flows now and what was used this period. */
export function FleetFigures({ figures }: { figures: Figures }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  return (
    <dl className='grid grid-cols-2 gap-3 @3xl:grid-cols-4 @3xl:gap-4'>
      <Figure label={t('overview.figures.nodes')} meta={t('overview.figures.nodesMeta', { offline: figures.offline, pending: figures.pending })}>
        {figures.online}
        <span className='text-muted-foreground'>{` / ${figures.total}`}</span>
      </Figure>
      <Figure label={t('overview.figures.cores')} meta={t('overview.figures.coresMeta', { count: figures.enrolled })}>
        {figures.running}
        <span className='text-muted-foreground'>{` / ${figures.enrolled}`}</span>
      </Figure>
      {/* Rates and usage take a full row on narrow views, so both values stay on one line. */}
      <Figure className='@max-3xl:col-span-2' label={t('overview.figures.rates')} meta={t('overview.figures.connections', { count: figures.connections })}>
        <UpDown
          download={formatRate(figures.rates.download, locale)}
          upload={formatRate(figures.rates.upload, locale)}
        />
      </Figure>
      <Figure
        className='@max-3xl:col-span-2'
        help={t('fleet.usageHint')}
        label={t('overview.figures.usage')}
        meta={figures.exhausted > 0
          ? <span className='text-[var(--color-danger-text)]'>{t('overview.figures.exhausted', { count: figures.exhausted })}</span>
          : t('overview.figures.usageTotal', { total: formatBytes(figures.download + figures.upload, locale) })}
      >
        <UpDown download={formatBytes(figures.download, locale)} upload={formatBytes(figures.upload, locale)} />
      </Figure>
    </dl>
  );
}
