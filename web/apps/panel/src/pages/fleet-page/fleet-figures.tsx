import { useTranslation } from 'react-i18next';
import { formatBytes, formatRate } from '@shared/lib/telemetry-format';

import type { FleetFigures as Figures } from '@/utils/fleet-summary';

import { UpDown } from '@/components/up-down';
import { KeyFigure, KeyFigures } from '@/components/key-figures';

/**
 * The fleet's key figures: what runs, what flows now, how many connections
 * the online nodes hold, and what this period used, summed but never against
 * a summed quota.
 */
export function FleetFigures({ figures }: { figures: Figures }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const { download, upload } = figures.rates;
  const combined = download === null && upload === null ? null : (download ?? 0) + (upload ?? 0);
  return (
    <KeyFigures label={t('fleet.figures.label')}>
      <KeyFigure label={t('fleet.figures.cores')} meta={t('fleet.figures.coresMeta', { stopped: figures.stopped, unknown: figures.unknown })}>
        {figures.running}
        <span className='text-muted-foreground'>{` / ${figures.total}`}</span>
      </KeyFigure>
      <KeyFigure label={t('fleet.figures.rates')} meta={t('fleet.figures.ratesMeta', { value: formatRate(combined, locale) })}>
        <UpDown download={formatRate(download, locale)} upload={formatRate(upload, locale)} />
      </KeyFigure>
      <KeyFigure
        label={t('fleet.figures.connections')}
        meta={t('fleet.figures.connectionsMeta', { online: figures.online, total: figures.total })}
      >
        {new Intl.NumberFormat(locale).format(figures.connections)}
      </KeyFigure>
      <KeyFigure
        help={t('fleet.usageHint')}
        label={t('fleet.figures.usage')}
        meta={<UpDown download={formatBytes(figures.download, locale)} upload={formatBytes(figures.upload, locale)} />}
      >
        {formatBytes(figures.download + figures.upload, locale)}
      </KeyFigure>
    </KeyFigures>
  );
}
