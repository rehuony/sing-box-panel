import { useTranslation } from 'react-i18next';
import { EM_DASH, formatBytes, formatRate } from '@shared/lib/telemetry-format';

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
  // The cores that do not run, by why, naming only the reasons some node has.
  const idle = ([['coresStopped', figures.stopped], ['coresNotEnabled', figures.notEnabled], ['coresUnknown', figures.unknown]] as const)
    .filter(([, count]) => count > 0)
    .map(([key, count]) => t(`fleet.figures.${key}`, { count }));
  let coresMeta = idle.join(' · ');
  if (idle.length === 0) coresMeta = figures.total > 0 ? t('fleet.figures.coresAllRunning') : EM_DASH;
  return (
    <KeyFigures label={t('fleet.figures.label')}>
      <KeyFigure label={t('fleet.figures.cores')} meta={coresMeta}>
        {figures.running}
        <span className='text-muted-foreground'>{` / ${figures.total}`}</span>
      </KeyFigure>
      <KeyFigure label={t('fleet.figures.rates')} meta={t('fleet.figures.ratesMeta', { value: formatRate(combined, locale) })}>
        <UpDown download={download} format={value => formatRate(value, locale)} upload={upload} />
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
        meta={(
          <UpDown download={figures.download} format={value => formatBytes(value, locale)} upload={figures.upload} />
        )}
      >
        {formatBytes(figures.download + figures.upload, locale)}
      </KeyFigure>
    </KeyFigures>
  );
}
