import { useTranslation } from 'react-i18next';
import { trafficDirectionText, trafficValueText } from '@shared/lib/telemetry-format';

/**
 * A download and an upload value, each after its arrow in its direction's
 * colour; a value takes that colour too unless it is zero. A value not known
 * reads 0.
 */
export function UpDown({ download, format, upload }: {
  download: number | null;
  upload: number | null;
  format: (value: number) => string;
}) {
  const { t } = useTranslation();
  return (
    <span className='inline-flex min-w-0 flex-wrap gap-x-3'>
      <span className='whitespace-nowrap'>
        <span aria-hidden='true' className={trafficDirectionText.download}>↓ </span>
        <span className='sr-only'>{t('telemetry.metric.download')}</span>
        <span className={trafficValueText('download', download)}>{format(download ?? 0)}</span>
      </span>
      <span className='whitespace-nowrap'>
        <span aria-hidden='true' className={trafficDirectionText.upload}>↑ </span>
        <span className='sr-only'>{t('telemetry.metric.upload')}</span>
        <span className={trafficValueText('upload', upload)}>{format(upload ?? 0)}</span>
      </span>
    </span>
  );
}
